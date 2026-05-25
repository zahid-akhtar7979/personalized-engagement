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

const systemCost = 5000.0
const avgOrderValue = 75.0

type AnalyticsEngine struct {
	log    *zap.Logger
	db     *gormdb.DB
	kafka  *kafka.Client
	writer *kafka.Writer
	redis  *redisclient.Client
	mu     sync.RWMutex
	state  struct {
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
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state.sessionCount[ev.UserID] > 0 {
		a.state.returningUsers[ev.UserID] = true
	}
	a.state.sessionCount[ev.UserID]++
	a.state.usersSeen[ev.UserID] = true

	switch ev.EventType {
	case models.EventTypeViewed:
		a.state.views++
		a.state.categoryViews[ev.CategoryID]++
		a.state.recImpressions++
	case models.EventTypeAddToCart:
		a.state.addToCart++
	case models.EventTypePurchased:
		a.state.purchases++
		a.state.revenueGain += avgOrderValue
		a.state.categoryPurch[ev.CategoryID]++
	}
}

func (a *AnalyticsEngine) recalculate() models.AnalyticsMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	totalUsers := int64(len(a.state.usersSeen))
	returning := int64(len(a.state.returningUsers))
	active := int64(0)
	for _, count := range a.state.sessionCount {
		if count > 0 {
			active++
		}
	}

	retentionRate := 0.0
	if totalUsers > 0 {
		retentionRate = (float64(returning) / float64(totalUsers)) * 100
	}

	ctr := 0.0
	if a.state.recImpressions > 0 {
		ctr = (float64(a.state.recClicks) / float64(a.state.recImpressions)) * 100
	}

	conversionRate := 0.0
	if a.state.views > 0 {
		conversionRate = (float64(a.state.purchases) / float64(a.state.views)) * 100
	}

	engagementScore := float64(a.state.views) + 3*float64(a.state.addToCart) + 5*float64(a.state.purchases)

	roi := 0.0
	if systemCost > 0 {
		roi = ((a.state.revenueGain - systemCost) / systemCost) * 100
	}

	return models.AnalyticsMetrics{
		RetentionRate:   retentionRate,
		CTR:             ctr,
		ConversionRate:  conversionRate,
		EngagementScore: engagementScore,
		ROIPercentage:   roi,
		ActiveUsers:     active,
		TotalUsers:      totalUsers,
		ReturningUsers:  returning,
		RecClicks:       a.state.recClicks,
		RecImpressions:  a.state.recImpressions,
		Purchases:       a.state.purchases,
		Views:           a.state.views,
		RevenueGain:     a.state.revenueGain,
		SystemCost:      systemCost,
		UpdatedAt:       time.Now().UTC(),
	}
}

func (a *AnalyticsEngine) periodicPublish(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := a.recalculate()
			a.mu.Lock()
			a.metrics = metrics
			a.mu.Unlock()

			_ = a.redis.SetJSON(ctx, "analytics:global", metrics, 5*time.Minute)
			_ = kafka.PublishWithRetry(ctx, a.writer, "global", metrics, a.log, 3)

			_ = a.db.Create(&models.AnalyticsMetricRecord{
				RetentionRate: metrics.RetentionRate, CTR: metrics.CTR,
				ConversionRate: metrics.ConversionRate, EngagementScore: metrics.EngagementScore,
				ROIPercentage: metrics.ROIPercentage, ActiveUsers: metrics.ActiveUsers,
				RecordedAt: metrics.UpdatedAt,
			}).Error
		}
	}
}

func (a *AnalyticsEngine) getAnalytics(c *gin.Context) {
	a.mu.RLock()
	metrics := a.metrics
	a.mu.RUnlock()
	if metrics.UpdatedAt.IsZero() {
		metrics = a.recalculate()
	}
	c.JSON(200, metrics)
}

func (a *AnalyticsEngine) getHistory(c *gin.Context) {
	var records []models.AnalyticsMetricRecord
	a.db.Order("recorded_at desc").Limit(50).Find(&records)
	c.JSON(200, records)
}
