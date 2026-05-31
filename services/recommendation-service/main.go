package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"personalized-engagement/pkg/config"
	pdb "personalized-engagement/pkg/db"
	"personalized-engagement/pkg/kafka"
	"personalized-engagement/pkg/logger"
	"personalized-engagement/pkg/models"
	gormdb "gorm.io/gorm"
)

type UserProfile struct {
	ViewedItems    map[int64]int
	CartItems      map[int64]bool
	PurchasedItems map[int64]bool
	CategoryCounts map[int64]int
}

type RecommendationEngine struct {
	log      *zap.Logger
	db       *gormdb.DB
	kafka    *kafka.Client
	writer   *kafka.Writer
	catalog  map[int64]models.ContentCatalog
	mu       sync.RWMutex
	profiles map[int64]*UserProfile
	global   struct {
		itemViews   map[int64]int
		categoryHot map[int64]int
		coView      map[int64]map[int64]int
	}
}

func main() {
	cfg := config.Load("recommendation-service")
	log := logger.New(cfg.ServiceName)

	db, err := pdb.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatal("db connect", zap.Error(err))
	}
	kc := kafka.NewClient(cfg.KafkaBrokers, log)
	engine := &RecommendationEngine{
		log:      log,
		db:       db,
		kafka:    kc,
		writer:   kc.NewWriter(kafka.TopicRecommendationEvents),
		catalog:  make(map[int64]models.ContentCatalog),
		profiles: make(map[int64]*UserProfile),
	}
	engine.global.itemViews = make(map[int64]int)
	engine.global.categoryHot = make(map[int64]int)
	engine.global.coView = make(map[int64]map[int64]int)

	engine.loadCatalog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go engine.consumeEvents(ctx)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/recommendations/:userId", engine.getRecommendations)
	r.GET("/api/users/sample", engine.getSampleUsers)
	r.GET("/api/users/active", engine.getActiveReplayUsers)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		log.Info("recommendation-service listening", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = engine.writer.Close()
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func (e *RecommendationEngine) loadCatalog() {
	var items []models.ContentCatalog
	e.db.Find(&items)
	for _, item := range items {
		if item.ImageURL == "" {
			item.ImageURL = fmt.Sprintf("https://picsum.photos/seed/%d/300/200", item.ItemID)
		}
		e.catalog[item.ItemID] = item
	}
	e.log.Info("catalog loaded from postgres", zap.Int("items", len(e.catalog)))
}

func (e *RecommendationEngine) getProfile(userID int64) *UserProfile {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.profiles[userID]; ok {
		return p
	}
	p := &UserProfile{
		ViewedItems:    make(map[int64]int),
		CartItems:      make(map[int64]bool),
		PurchasedItems: make(map[int64]bool),
		CategoryCounts: make(map[int64]int),
	}
	e.profiles[userID] = p
	return p
}

func (e *RecommendationEngine) consumeEvents(ctx context.Context) {
	reader := e.kafka.NewReader(kafka.TopicUserEvents, "recommendation-service")
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.log.Warn("read message", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		var ev models.UserEvent
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			continue
		}
		e.processEvent(ctx, ev)
	}
}

func (e *RecommendationEngine) processEvent(ctx context.Context, ev models.UserEvent) {
	p := e.getProfile(ev.UserID)

	e.mu.Lock()
	switch ev.EventType {
	case models.EventTypeViewed:
		p.ViewedItems[ev.ItemID]++
		p.CategoryCounts[ev.CategoryID]++
		e.global.itemViews[ev.ItemID]++
		e.global.categoryHot[ev.CategoryID]++
		e.trackCoView(ev.ItemID, p.ViewedItems)
	case models.EventTypeAddToCart:
		p.CartItems[ev.ItemID] = true
		p.CategoryCounts[ev.CategoryID] += 2
	case models.EventTypePurchased:
		p.PurchasedItems[ev.ItemID] = true
		p.CategoryCounts[ev.CategoryID] += 3
	}
	e.mu.Unlock()

	_ = e.db.Create(&models.UserEventRecord{
		EventID: ev.EventID, UserID: ev.UserID, ItemID: ev.ItemID,
		CategoryID: ev.CategoryID, EventType: ev.EventType, Timestamp: ev.Timestamp,
	}).Error

	recs := models.NormalizeRecs(e.buildRecommendations(ev.UserID))
	recEvent := models.RecommendationEvent{
		UserID: ev.UserID, Recommendations: recs, Timestamp: time.Now().UTC(),
	}
	_ = kafka.PublishWithRetry(ctx, e.writer, fmt.Sprintf("%d", ev.UserID), recEvent, e.log, 3)
}

func (e *RecommendationEngine) trackCoView(itemID int64, viewed map[int64]int) {
	if e.global.coView[itemID] == nil {
		e.global.coView[itemID] = make(map[int64]int)
	}
	for other := range viewed {
		if other != itemID {
			e.global.coView[itemID][other]++
		}
	}
}

func (e *RecommendationEngine) buildRecommendations(userID int64) models.PersonalizedRecs {
	e.mu.RLock()
	p := e.profiles[userID]
	e.mu.RUnlock()
	if p == nil {
		p = &UserProfile{
			ViewedItems: make(map[int64]int), CartItems: make(map[int64]bool),
			CategoryCounts: make(map[int64]int),
		}
	}
	return models.PersonalizedRecs{
		RecommendedForYou:   e.trendingInPreferredCategories(p, 6),
		TrendingNow:         e.globalTrending(6),
		BecauseYouViewed:    e.becauseYouViewed(p, 6),
		CartRecommendations: e.cartBased(p, 6),
	}
}

func (e *RecommendationEngine) ensureCatalogItem(itemID int64) {
	e.mu.RLock()
	_, ok := e.catalog[itemID]
	e.mu.RUnlock()
	if ok {
		return
	}
	var item models.ContentCatalog
	if err := e.db.Where("item_id = ?", itemID).First(&item).Error; err != nil {
		return
	}
	if item.ImageURL == "" {
		item.ImageURL = fmt.Sprintf("https://picsum.photos/seed/%d/300/200", item.ItemID)
	}
	e.mu.Lock()
	e.catalog[itemID] = item
	e.mu.Unlock()
}

func (e *RecommendationEngine) itemToRec(itemID int64, score float64, reason string) models.Recommendation {
	e.ensureCatalogItem(itemID)
	item, ok := e.catalog[itemID]
	title, category, imageURL := fmt.Sprintf("Product-%d", itemID), "General", fmt.Sprintf("https://picsum.photos/seed/%d/300/200", itemID)
	if ok {
		title, category = item.Title, item.Category
		if item.ImageURL != "" {
			imageURL = item.ImageURL
		}
	}
	return models.Recommendation{ItemID: itemID, Title: title, Category: category, Score: score, Reason: reason, ImageURL: imageURL}
}

func (e *RecommendationEngine) trendingInPreferredCategories(p *UserProfile, limit int) []models.Recommendation {
	type scored struct {
		itemID int64
		score  float64
	}
	seen := make(map[int64]bool)
	for itemID := range p.ViewedItems {
		seen[itemID] = true
	}
	for itemID := range p.PurchasedItems {
		seen[itemID] = true
	}

	topCats := make(map[int64]int)
	for catID, count := range p.CategoryCounts {
		if count > 0 {
			topCats[catID] = count
		}
	}

	var results []scored
	e.mu.RLock()
	for itemID, views := range e.global.itemViews {
		if seen[itemID] {
			continue
		}
		catID := itemID%6 + 1
		if item, ok := e.catalog[itemID]; ok {
			catID = item.CategoryID
		}
		catBoost := float64(topCats[catID])
		if catBoost == 0 {
			continue
		}
		results = append(results, scored{itemID, float64(views) * catBoost})
	}
	e.mu.RUnlock()

	// Fallback: suggest unseen catalog items in the user's top categories.
	if len(results) < limit {
		type catKV struct {
			id, count int
		}
		var cats []catKV
		for id, c := range topCats {
			cats = append(cats, catKV{int(id), c})
		}
		sort.Slice(cats, func(i, j int) bool { return cats[i].count > cats[j].count })

		e.mu.RLock()
		for _, cat := range cats {
			for itemID, item := range e.catalog {
				if seen[itemID] || item.CategoryID != int64(cat.id) {
					continue
				}
				results = append(results, scored{itemID, float64(cat.count)})
				if len(results) >= limit*3 {
					break
				}
			}
		}
		e.mu.RUnlock()
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	var recs []models.Recommendation
	for _, s := range results {
		if len(recs) >= limit {
			break
		}
		if seen[s.itemID] {
			continue
		}
		seen[s.itemID] = true
		recs = append(recs, e.itemToRec(s.itemID, s.score, "Trending in your preferred categories"))
	}
	return recs
}

func (e *RecommendationEngine) globalTrending(limit int) []models.Recommendation {
	type kv struct {
		id, count int64
	}
	var items []kv
	e.mu.RLock()
	for id, c := range e.global.itemViews {
		items = append(items, kv{id, int64(c)})
	}
	e.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].count > items[j].count })

	var recs []models.Recommendation
	for i, kv := range items {
		if i >= limit {
			break
		}
		recs = append(recs, e.itemToRec(kv.id, float64(kv.count), "Trending now across platform"))
	}
	return recs
}

func (e *RecommendationEngine) becauseYouViewed(p *UserProfile, limit int) []models.Recommendation {
	var lastViewed int64
	maxViews := 0
	for itemID, count := range p.ViewedItems {
		if count > maxViews {
			maxViews = count
			lastViewed = itemID
		}
	}
	if lastViewed == 0 {
		return nil
	}

	e.mu.RLock()
	co := e.global.coView[lastViewed]
	e.mu.RUnlock()

	type kv struct {
		id, count int64
	}
	var related []kv
	for id, c := range co {
		if !p.PurchasedItems[id] {
			related = append(related, kv{id, int64(c)})
		}
	}
	sort.Slice(related, func(i, j int) bool { return related[i].count > related[j].count })

	e.ensureCatalogItem(lastViewed)
	item, _ := e.catalog[lastViewed]
	viewedTitle := item.Title
	if viewedTitle == "" {
		viewedTitle = fmt.Sprintf("Product-%d", lastViewed)
	}
	var recs []models.Recommendation
	for i, r := range related {
		if i >= limit {
			break
		}
		recs = append(recs, e.itemToRec(r.id, float64(r.count), "Because you viewed "+viewedTitle))
	}
	if len(recs) == 0 && item.CategoryID > 0 {
		for id, catItem := range e.catalog {
			if id != lastViewed && catItem.CategoryID == item.CategoryID && len(recs) < limit {
				recs = append(recs, e.itemToRec(id, 1.0, "Because you viewed "+viewedTitle))
			}
		}
	}
	return recs
}

func (e *RecommendationEngine) cartBased(p *UserProfile, limit int) []models.Recommendation {
	if len(p.CartItems) == 0 {
		return nil
	}
	var cartItem int64
	for id := range p.CartItems {
		cartItem = id
		break
	}
	item, ok := e.catalog[cartItem]
	if !ok {
		return nil
	}
	var recs []models.Recommendation
	for id, catItem := range e.catalog {
		if id == cartItem {
			continue
		}
		if catItem.CategoryID == item.CategoryID {
			recs = append(recs, e.itemToRec(id, 2.0, "Frequently bought with cart items"))
			if len(recs) >= limit {
				break
			}
		}
	}
	return recs
}

func (e *RecommendationEngine) catalogCategoryID(itemID int64) int64 {
	e.ensureCatalogItem(itemID)
	if item, ok := e.catalog[itemID]; ok {
		return item.CategoryID
	}
	return itemID%6 + 1
}

func (e *RecommendationEngine) hydrateProfileFromDB(userID int64) {
	var records []models.UserEventRecord
	if err := e.db.Where("user_id = ?", userID).Order("timestamp asc").Limit(500).Find(&records).Error; err != nil || len(records) == 0 {
		return
	}
	p := e.getProfile(userID)
	e.mu.Lock()
	for _, rec := range records {
		switch rec.EventType {
		case models.EventTypeViewed:
			p.ViewedItems[rec.ItemID]++
			p.CategoryCounts[rec.CategoryID]++
			e.trackCoView(rec.ItemID, p.ViewedItems)
		case models.EventTypeAddToCart:
			p.CartItems[rec.ItemID] = true
			p.CategoryCounts[rec.CategoryID] += 2
		case models.EventTypePurchased:
			p.PurchasedItems[rec.ItemID] = true
			p.CategoryCounts[rec.CategoryID] += 3
		}
	}
	e.mu.Unlock()
}

func (e *RecommendationEngine) hydrateProfileFromDBIfNeeded(userID int64) {
	e.mu.RLock()
	p := e.profiles[userID]
	needsHydrate := p == nil || len(p.ViewedItems) == 0
	e.mu.RUnlock()
	if needsHydrate {
		e.hydrateProfileFromDB(userID)
	}
}

func (e *RecommendationEngine) getRecommendations(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	e.hydrateProfileFromDBIfNeeded(userID)
	c.JSON(200, models.NormalizeRecs(e.buildRecommendations(userID)))
}

func (e *RecommendationEngine) getActiveReplayUsers(c *gin.Context) {
	type row struct {
		UserID     int64 `gorm:"column:user_id"`
		EventCount int64 `gorm:"column:event_count"`
	}
	var rows []row
	e.db.Raw(`
		SELECT user_id, COUNT(*) AS event_count
		FROM user_events
		GROUP BY user_id
		ORDER BY MAX(timestamp) DESC
		LIMIT 30
	`).Scan(&rows)

	e.mu.RLock()
	for uid := range e.profiles {
		found := false
		for _, r := range rows {
			if r.UserID == uid {
				found = true
				break
			}
		}
		if !found {
			rows = append([]row{{UserID: uid, EventCount: 1}}, rows...)
		}
	}
	e.mu.RUnlock()

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"userId": r.UserID, "eventCount": r.EventCount,
			"username": fmt.Sprintf("user_%d", r.UserID),
		})
	}
	c.JSON(200, out)
}

func (e *RecommendationEngine) getSampleUsers(c *gin.Context) {
	type row struct {
		UserID     int64  `gorm:"column:user_id"`
		EventCount int64  `gorm:"column:event_count"`
		Username   string `gorm:"column:username"`
	}
	var rows []row
	e.db.Raw(`
		SELECT u.id AS user_id, COUNT(e.id) AS event_count, u.username
		FROM users u
		INNER JOIN user_events e ON e.user_id = u.id
		GROUP BY u.id, u.username
		ORDER BY event_count DESC
		LIMIT 20
	`).Scan(&rows)
	if len(rows) == 0 {
		c.JSON(200, []row{})
		return
	}
	out := make([]gin.H, len(rows))
	for i, r := range rows {
		out[i] = gin.H{"userId": r.UserID, "eventCount": r.EventCount, "username": r.Username}
	}
	c.JSON(200, out)
}
