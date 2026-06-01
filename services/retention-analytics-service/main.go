package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
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
	redisclient "personalized-engagement/pkg/redis"
	gormdb "gorm.io/gorm"
)

const (
	avgOrderValue = 75.0 // modeled average order value (USD); Retailrocket has no price in events

	// ROI assumes only a fraction of observed GMV is attributed to the personalization platform,
	// and platform cost scales with event volume (infra + processing).
	personalizationAttribution = 0.08 // 8% of gross merchandise value credited to PEP
	platformBaseCostUSD        = 75_000.0
	platformCostPerEventUSD    = 0.015
)

type AnalyticsEngine struct {
	log          *zap.Logger
	db           *gormdb.DB
	kafka        *kafka.Client
	writer       *kafka.Writer
	redis        *redisclient.Client
	mu           sync.RWMutex
	refreshMu    sync.Mutex
	lastRefresh  time.Time
	state        struct {
		usersSeen       map[int64]bool
		returningUsers  map[int64]bool
		sessionCount    map[int64]int
		views           int64
		purchases       int64
		addToCart       int64
		recClicks       int64
		recImpressions  int64
		revenueGain     float64
		categoryViews   map[int64]int64
		categoryPurch   map[int64]int64
	}
	metrics models.AnalyticsMetrics
}

func main() {
	cfg := config.Load("retention-analytics-service")
	log := logger.New(cfg.ServiceName)

	db, err := pdb.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatal("db connect", zap.Error(err))
	}
	kc := kafka.NewClient(cfg.KafkaBrokers, log)
	redis := redisclient.New(cfg.RedisAddr)

	engine := &AnalyticsEngine{
		log:    log,
		db:     db,
		kafka:  kc,
		writer: kc.NewWriter(kafka.TopicAnalyticsEvents),
		redis:  redis,
	}
	engine.state.usersSeen = make(map[int64]bool)
	engine.state.returningUsers = make(map[int64]bool)
	engine.state.sessionCount = make(map[int64]int)
	engine.state.categoryViews = make(map[int64]int64)
	engine.state.categoryPurch = make(map[int64]int64)
	engine.refreshFromDB()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go engine.consumeUserEvents(ctx)
	go engine.periodicPublish(ctx)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/analytics", engine.getAnalytics)
	r.GET("/api/analytics/history", engine.getHistory)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		log.Info("retention-analytics-service listening", zap.String("port", cfg.Port))
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

func (a *AnalyticsEngine) consumeUserEvents(ctx context.Context) {
	reader := a.kafka.NewReader(kafka.TopicUserEvents, "retention-analytics-service")
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			continue
		}

		var ev models.UserEvent
		if err := json.Unmarshal(msg.Value, &ev); err != nil {
			continue
		}
		a.processEvent(ev)
	}
}

func (a *AnalyticsEngine) processEvent(ev models.UserEvent) {
	_ = ev
	a.scheduleRefreshFromDB()
}

func (a *AnalyticsEngine) scheduleRefreshFromDB() {
	a.refreshMu.Lock()
	if time.Since(a.lastRefresh) < 2*time.Second {
		a.refreshMu.Unlock()
		return
	}
	a.lastRefresh = time.Now()
	a.refreshMu.Unlock()
	go a.refreshFromDB()
}

// refreshFromDB aggregates real metrics from user_events (Retailrocket import + live replay).
func (a *AnalyticsEngine) refreshFromDB() models.AnalyticsMetrics {
	type counts struct {
		TotalUsers int64 `gorm:"column:total_users"`
		Views      int64 `gorm:"column:views"`
		Purchases  int64 `gorm:"column:purchases"`
		AddToCart  int64 `gorm:"column:add_to_cart"`
	}
	var c counts
	err := a.db.Raw(`
		SELECT
			COUNT(DISTINCT user_id) AS total_users,
			COUNT(*) FILTER (WHERE event_type = 'VIEWED') AS views,
			COUNT(*) FILTER (WHERE event_type = 'PURCHASED') AS purchases,
			COUNT(*) FILTER (WHERE event_type = 'ADD_TO_CART') AS add_to_cart
		FROM user_events
	`).Scan(&c).Error
	if err != nil {
		a.log.Warn("analytics db refresh", zap.Error(err))
	}

	var returningUsers int64
	_ = a.db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT user_id FROM user_events GROUP BY user_id HAVING COUNT(*) > 1
		) t
	`).Scan(&returningUsers).Error

	views := c.Views
	purchases := c.Purchases
	addToCart := c.AddToCart
	totalUsers := c.TotalUsers
	returning := returningUsers

	retentionRate := 0.0
	if totalUsers > 0 {
		retentionRate = (float64(returning) / float64(totalUsers)) * 100
	}

	impressions := views
	clicks := addToCart + purchases
	ctr := 0.0
	if impressions > 0 {
		ctr = (float64(clicks) / float64(impressions)) * 100
		if ctr > 100 {
			ctr = 100
		}
	}

	conversionRate := 0.0
	if views > 0 {
		conversionRate = (float64(purchases) / float64(views)) * 100
	}

	// Engagement index 0–100: composite of retention, CTR, and purchase conversion (not a raw event count).
	ctrComponent := ctr
	if ctrComponent > 15 {
		ctrComponent = 15
	}
	ctrComponent = (ctrComponent / 15) * 100

	convComponent := conversionRate
	if convComponent > 5 {
		convComponent = 5
	}
	convComponent = (convComponent / 5) * 100

	engagementScore := retentionRate*0.4 + ctrComponent*0.3 + convComponent*0.3
	if engagementScore > 100 {
		engagementScore = 100
	}

	totalEvents := views + addToCart + purchases
	grossRevenue := float64(purchases) * avgOrderValue
	revenueGain := grossRevenue * personalizationAttribution // attributed incremental revenue
	systemCost := platformBaseCostUSD + float64(totalEvents)*platformCostPerEventUSD
	roi := 0.0
	if systemCost > 0 {
		roi = ((revenueGain - systemCost) / systemCost) * 100
	}

	metrics := models.AnalyticsMetrics{
		RetentionRate:   retentionRate,
		CTR:             ctr,
		ConversionRate:  conversionRate,
		EngagementScore: engagementScore,
		ROIPercentage:   roi,
		ActiveUsers:     totalUsers,
		TotalUsers:      totalUsers,
		ReturningUsers:  returning,
		RecClicks:       clicks,
		RecImpressions:  impressions,
		Purchases:       purchases,
		Views:           views,
		RevenueGain:     revenueGain,
		SystemCost:      systemCost,
		UpdatedAt:       time.Now().UTC(),
	}

	a.mu.Lock()
	a.metrics = metrics
	a.mu.Unlock()
	return metrics
}

func (a *AnalyticsEngine) periodicPublish(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastHistory time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := a.refreshFromDB()
			_ = a.redis.SetJSON(ctx, "analytics:global", metrics, 5*time.Minute)
			_ = kafka.PublishWithRetry(ctx, a.writer, "global", metrics, a.log, 3)

			if metrics.Views > 0 && time.Since(lastHistory) >= 60*time.Second {
				_ = a.db.Create(&models.AnalyticsMetricRecord{
					RetentionRate: metrics.RetentionRate, CTR: metrics.CTR,
					ConversionRate: metrics.ConversionRate, EngagementScore: metrics.EngagementScore,
					ROIPercentage: metrics.ROIPercentage, ActiveUsers: metrics.ActiveUsers,
					RecordedAt: metrics.UpdatedAt,
				}).Error
				lastHistory = time.Now()
			}
		}
	}
}

func (a *AnalyticsEngine) getAnalytics(c *gin.Context) {
	a.mu.RLock()
	metrics := a.metrics
	a.mu.RUnlock()
	if metrics.Views == 0 && metrics.TotalUsers == 0 {
		metrics = a.refreshFromDB()
	}
	c.JSON(200, metrics)
}

func (a *AnalyticsEngine) getHistory(c *gin.Context) {
	var records []models.AnalyticsMetricRecord
	a.db.Order("recorded_at desc").Limit(50).Find(&records)

	out := make([]gin.H, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		out = append(out, gin.H{
			"retentionRate":   r.RetentionRate,
			"ctr":             r.CTR,
			"conversionRate":  r.ConversionRate,
			"engagementScore": r.EngagementScore,
			"roiPercentage":   r.ROIPercentage,
			"activeUsers":     r.ActiveUsers,
			"recordedAt":      r.RecordedAt,
		})
	}
	if len(out) == 0 {
		m := a.refreshFromDB()
		out = append(out, gin.H{
			"retentionRate": m.RetentionRate, "ctr": m.CTR,
			"conversionRate": m.ConversionRate, "engagementScore": m.EngagementScore,
			"roiPercentage": m.ROIPercentage, "activeUsers": m.ActiveUsers,
			"recordedAt": m.UpdatedAt,
		})
	}
	c.JSON(200, out)
}
