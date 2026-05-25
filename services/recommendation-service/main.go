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
		e.catalog[item.ItemID] = item
	}
	if len(e.catalog) == 0 {
		fallback := []models.ContentCatalog{
			{ItemID: 1001, Title: "Pro Laptop 15\"", Category: "Computers", CategoryID: 2, Price: 1299.99, ImageURL: "https://picsum.photos/seed/laptop/300/200"},
			{ItemID: 1002, Title: "Wireless Mouse", Category: "Computers", CategoryID: 2, Price: 49.99, ImageURL: "https://picsum.photos/seed/mouse/300/200"},
			{ItemID: 1003, Title: "USB-C Hub", Category: "Computers", CategoryID: 2, Price: 79.99, ImageURL: "https://picsum.photos/seed/hub/300/200"},
			{ItemID: 1004, Title: "Smartphone X", Category: "Phones", CategoryID: 3, Price: 899.99, ImageURL: "https://picsum.photos/seed/phone/300/200"},
			{ItemID: 1005, Title: "Phone Case", Category: "Phones", CategoryID: 3, Price: 24.99, ImageURL: "https://picsum.photos/seed/case/300/200"},
			{ItemID: 1006, Title: "Running Shoes", Category: "Sports", CategoryID: 6, Price: 129.99, ImageURL: "https://picsum.photos/seed/shoes/300/200"},
			{ItemID: 1007, Title: "Yoga Mat", Category: "Sports", CategoryID: 6, Price: 39.99, ImageURL: "https://picsum.photos/seed/yoga/300/200"},
			{ItemID: 1008, Title: "Desk Lamp", Category: "Home", CategoryID: 5, Price: 59.99, ImageURL: "https://picsum.photos/seed/lamp/300/200"},
			{ItemID: 1009, Title: "Coffee Maker", Category: "Home", CategoryID: 5, Price: 89.99, ImageURL: "https://picsum.photos/seed/coffee/300/200"},
			{ItemID: 1010, Title: "Winter Jacket", Category: "Fashion", CategoryID: 4, Price: 199.99, ImageURL: "https://picsum.photos/seed/jacket/300/200"},
			{ItemID: 1011, Title: "Bluetooth Headphones", Category: "Electronics", CategoryID: 1, Price: 149.99, ImageURL: "https://picsum.photos/seed/headphones/300/200"},
			{ItemID: 1012, Title: "4K Monitor", Category: "Computers", CategoryID: 2, Price: 449.99, ImageURL: "https://picsum.photos/seed/monitor/300/200"},
			{ItemID: 1013, Title: "Fitness Tracker", Category: "Sports", CategoryID: 6, Price: 79.99, ImageURL: "https://picsum.photos/seed/tracker/300/200"},
			{ItemID: 1014, Title: "Smart Watch", Category: "Phones", CategoryID: 3, Price: 299.99, ImageURL: "https://picsum.photos/seed/watch/300/200"},
			{ItemID: 1015, Title: "Tablet Pro", Category: "Computers", CategoryID: 2, Price: 599.99, ImageURL: "https://picsum.photos/seed/tablet/300/200"},
		}
		for _, item := range fallback {
			e.catalog[item.ItemID] = item
		}
	}
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

	recs := e.buildRecommendations(ev.UserID)
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

func (e *RecommendationEngine) itemToRec(itemID int64, score float64, reason string) models.Recommendation {
	item, ok := e.catalog[itemID]
	title, category, imageURL := "Item", "General", "https://picsum.photos/seed/item/300/200"
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
	var results []scored
	seen := make(map[int64]bool)
	for itemID := range p.ViewedItems {
		seen[itemID] = true
	}
	for itemID := range p.PurchasedItems {
		seen[itemID] = true
	}

	e.mu.RLock()
	for itemID, views := range e.global.itemViews {
		item, ok := e.catalog[itemID]
		if !ok || seen[itemID] {
			continue
		}
		catBoost := float64(p.CategoryCounts[item.CategoryID])
		if catBoost == 0 {
			catBoost = 0.5
		}
		results = append(results, scored{itemID, float64(views) * catBoost})
	}
	e.mu.RUnlock()

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	var recs []models.Recommendation
	for _, s := range results {
		if len(recs) >= limit {
			break
		}
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

	item, _ := e.catalog[lastViewed]
	var recs []models.Recommendation
	for i, r := range related {
		if i >= limit {
			break
		}
		recs = append(recs, e.itemToRec(r.id, float64(r.count), "Because you viewed "+item.Title))
	}
	if len(recs) == 0 && item.CategoryID > 0 {
		for id, catItem := range e.catalog {
			if id != lastViewed && catItem.CategoryID == item.CategoryID && len(recs) < limit {
				recs = append(recs, e.itemToRec(id, 1.0, "Because you viewed "+item.Title))
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

func (e *RecommendationEngine) getRecommendations(c *gin.Context) {
	userID, _ := strconv.ParseInt(c.Param("userId"), 10, 64)
	c.JSON(200, e.buildRecommendations(userID))
}
