package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"personalized-engagement/pkg/config"
	"personalized-engagement/pkg/kafka"
	"personalized-engagement/pkg/logger"
	"personalized-engagement/pkg/models"
	redisclient "personalized-engagement/pkg/redis"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[int64]map[*websocket.Conn]bool)}
}

func (h *Hub) Register(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]bool)
	}
	h.clients[userID][conn] = true
}

func (h *Hub) Unregister(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, userID)
		}
	}
}

func (h *Hub) Broadcast(userID int64, payload interface{}) {
	data, _ := json.Marshal(payload)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients[userID] {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}
}

type Orchestrator struct {
	log      *zap.Logger
	kafka    *kafka.Client
	redis    *redisclient.Client
	hub      *Hub
	feed     []models.ActivityFeedItem
	feedMu   sync.RWMutex
	dashWriter *kafka.Writer
}

func main() {
	cfg := config.Load("engagement-orchestrator")
	log := logger.New(cfg.ServiceName)

	kc := kafka.NewClient(cfg.KafkaBrokers, log)
	redis := redisclient.New(cfg.RedisAddr)
	if err := redis.Ping(context.Background()); err != nil {
		log.Warn("redis ping", zap.Error(err))
	}

	orch := &Orchestrator{
		log:        log,
		kafka:      kc,
		redis:      redis,
		hub:        NewHub(),
		dashWriter: kc.NewWriter(kafka.TopicDashboardEvents),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go orch.consumeRecommendations(ctx)
	go orch.consumeAnalytics(ctx)
	go orch.consumeUserEvents(ctx)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/dashboard/:userId", orch.getDashboard)
	r.GET("/api/activity", orch.getActivityFeed)
	r.GET("/ws/dashboard/:userId", orch.wsDashboard)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		log.Info("engagement-orchestrator listening", zap.String("port", cfg.Port))
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
	_ = orch.dashWriter.Close()
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

func (o *Orchestrator) addFeed(message, feedType string, userID int64) {
	item := models.ActivityFeedItem{
		ID: uuid.New().String(), Message: message, UserID: userID,
		Timestamp: time.Now().UTC(), Type: feedType,
	}
	o.feedMu.Lock()
	o.feed = append([]models.ActivityFeedItem{item}, o.feed...)
	if len(o.feed) > 100 {
		o.feed = o.feed[:100]
	}
	o.feedMu.Unlock()
}

func (o *Orchestrator) consumeRecommendations(ctx context.Context) {
	reader := o.kafka.NewReader(kafka.TopicRecommendationEvents, "engagement-orchestrator-recs")
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

		var rec models.RecommendationEvent
		if err := json.Unmarshal(msg.Value, &rec); err != nil {
			continue
		}

		dashboard := models.DashboardPayload{
			UserID: rec.UserID, Recommendations: rec.Recommendations,
			UpdatedAt: time.Now().UTC(),
		}

		_ = o.redis.SetDashboard(ctx, rec.UserID, dashboard, 5*time.Minute)

		wsPayload := map[string]interface{}{
			"type": "dashboard_update",
			"data": dashboard,
		}
		o.hub.Broadcast(rec.UserID, wsPayload)

		_ = kafka.PublishWithRetry(ctx, o.dashWriter, fmt.Sprintf("%d", rec.UserID), dashboard, o.log, 3)

		o.addFeed(fmt.Sprintf("User %d: recommendations refreshed", rec.UserID), "recommendation", rec.UserID)
		o.addFeed(fmt.Sprintf("User %d: dashboard updated", rec.UserID), "dashboard", rec.UserID)
	}
}

func (o *Orchestrator) consumeAnalytics(ctx context.Context) {
	reader := o.kafka.NewReader(kafka.TopicAnalyticsEvents, "engagement-orchestrator-analytics")
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

		var metrics models.AnalyticsMetrics
		if err := json.Unmarshal(msg.Value, &metrics); err != nil {
			continue
		}

		_ = o.redis.SetJSON(ctx, "analytics:global", metrics, 5*time.Minute)

		wsPayload := map[string]interface{}{
			"type": "analytics_update",
			"data": metrics,
		}
		// broadcast to all connected users
		o.hub.mu.RLock()
		for userID := range o.hub.clients {
			o.hub.Broadcast(userID, wsPayload)
		}
		o.hub.mu.RUnlock()

		o.addFeed("Analytics metrics refreshed", "analytics", 0)
	}
}

func (o *Orchestrator) consumeUserEvents(ctx context.Context) {
	reader := o.kafka.NewReader(kafka.TopicUserEvents, "engagement-orchestrator-events")
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

		action := "viewed"
		switch ev.EventType {
		case models.EventTypeAddToCart:
			action = "added to cart"
		case models.EventTypePurchased:
			action = "purchased"
		}
		o.addFeed(fmt.Sprintf("User %d %s item %d", ev.UserID, action, ev.ItemID), "event", ev.UserID)

		wsPayload := map[string]interface{}{
			"type": "activity",
			"data": map[string]interface{}{
				"userId": ev.UserID, "itemId": ev.ItemID,
				"eventType": ev.EventType, "message": fmt.Sprintf("User %d %s item %d", ev.UserID, action, ev.ItemID),
			},
		}
		o.hub.Broadcast(ev.UserID, wsPayload)
	}
}

func (o *Orchestrator) getDashboard(c *gin.Context) {
	userID, _ := strconvParseInt(c.Param("userId"))
	var dashboard models.DashboardPayload
	if err := o.redis.GetDashboard(c.Request.Context(), userID, &dashboard); err != nil {
		c.JSON(200, models.DashboardPayload{
			UserID: userID,
			Recommendations: models.PersonalizedRecs{},
			UpdatedAt: time.Now().UTC(),
		})
		return
	}
	c.JSON(200, dashboard)
}

func (o *Orchestrator) getActivityFeed(c *gin.Context) {
	o.feedMu.RLock()
	defer o.feedMu.RUnlock()
	c.JSON(200, o.feed)
}

func (o *Orchestrator) wsDashboard(c *gin.Context) {
	userID, err := strconvParseInt(c.Param("userId"))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid userId"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	o.hub.Register(userID, conn)
	defer o.hub.Unregister(userID, conn)

	// send current dashboard on connect
	var dashboard models.DashboardPayload
	if err := o.redis.GetDashboard(c.Request.Context(), userID, &dashboard); err == nil {
		_ = conn.WriteJSON(map[string]interface{}{"type": "dashboard_update", "data": dashboard})
	}

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func strconvParseInt(s string) (int64, error) {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}
