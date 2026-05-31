package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

type AIInsightsService struct {
	log           *zap.Logger
	db            *gormdb.DB
	redis         *redisclient.Client
	kafka         *kafka.Client
	alertWriter   *kafka.Writer
	cfg           *config.Config
	alerts        []models.RetentionAlert
	alertsMu      sync.RWMutex
	prevRetention float64
	categoryPerf  map[string]float64
}

func main() {
	cfg := config.Load("ai-insights-service")
	log := logger.New(cfg.ServiceName)

	db, err := pdb.Connect(cfg.PostgresDSN)
	if err != nil {
		log.Fatal("db connect", zap.Error(err))
	}

	kc := kafka.NewClient(cfg.KafkaBrokers, log)
	svc := &AIInsightsService{
		log:          log,
		db:           db,
		redis:        redisclient.New(cfg.RedisAddr),
		kafka:        kc,
		alertWriter:  kc.NewWriter(kafka.TopicAnalyticsEvents),
		cfg:          cfg,
		categoryPerf: make(map[string]float64),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.consumeAnalytics(ctx)
	go svc.runRetentionAgent(ctx)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/ai/alerts", svc.getAlerts)
	r.POST("/api/ai/sql", svc.sqlAssistant)
	r.POST("/api/ai/query", svc.aiQuery)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		log.Info("ai-insights-service listening", zap.String("port", cfg.Port))
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
	_ = svc.alertWriter.Close()
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func (s *AIInsightsService) consumeAnalytics(ctx context.Context) {
	reader := s.kafka.NewReader(kafka.TopicAnalyticsEvents, "ai-insights-service")
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
		s.analyzeRetention(metrics)
	}
}

func (s *AIInsightsService) analyzeRetention(metrics models.AnalyticsMetrics) {
	if s.prevRetention > 0 && metrics.RetentionRate < s.prevRetention-5 {
		alert := models.RetentionAlert{
			AlertType:      "RETENTION_DROP",
			Category:       "Platform-wide",
			DropPercentage: s.prevRetention - metrics.RetentionRate,
			Suggestion:     "Increase personalized recommendations and re-engagement campaigns",
			Timestamp:      time.Now().UTC(),
		}
		s.addAlert(alert)
		_ = kafka.PublishWithRetry(context.Background(), s.alertWriter, "retention-alert", alert, s.log, 3)
	}

	if metrics.ConversionRate < 2 {
		alert := models.RetentionAlert{
			AlertType:      "LOW_CONVERSION",
			Category:       "General",
			DropPercentage: metrics.ConversionRate,
			Suggestion:     "Optimize cart recommendations and checkout flow",
			Timestamp:      time.Now().UTC(),
		}
		s.addAlert(alert)
	}

	// Category performance simulation
	categories := []string{"Electronics", "Computers", "Phones", "Fashion", "Home", "Sports"}
	for _, cat := range categories {
		score := metrics.EngagementScore * (0.5 + float64(len(cat)%5)*0.1)
		prev := s.categoryPerf[cat]
		s.categoryPerf[cat] = score
		if prev > 0 && score < prev*0.85 {
			alert := models.RetentionAlert{
				AlertType:      "RETENTION_DROP",
				Category:       cat,
				DropPercentage: 12.5,
				Suggestion:     fmt.Sprintf("Increase %s recommendations", strings.ToLower(cat)),
				Timestamp:      time.Now().UTC(),
			}
			s.addAlert(alert)
		}
	}

	s.prevRetention = metrics.RetentionRate
}

func (s *AIInsightsService) runRetentionAgent(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var metrics models.AnalyticsMetrics
			if err := s.redis.GetJSON(ctx, "analytics:global", &metrics); err != nil {
				continue
			}
			s.analyzeRetention(metrics)
		}
	}
}

func (s *AIInsightsService) addAlert(alert models.RetentionAlert) {
	s.alertsMu.Lock()
	s.alerts = append([]models.RetentionAlert{alert}, s.alerts...)
	if len(s.alerts) > 50 {
		s.alerts = s.alerts[:50]
	}
	s.alertsMu.Unlock()
}

func (s *AIInsightsService) getAlerts(c *gin.Context) {
	s.alertsMu.RLock()
	defer s.alertsMu.RUnlock()
	c.JSON(200, s.alerts)
}

func (s *AIInsightsService) sqlAssistant(c *gin.Context) {
	var req models.SQLQueryRequest
	if err := c.BindJSON(&req); err != nil || req.Question == "" {
		c.JSON(400, gin.H{"error": "question required"})
		return
	}

	sql, err := s.generateSQL(req.Question)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	rows, err := s.executeSafeSQL(sql)
	if err != nil {
		c.JSON(200, models.SQLQueryResponse{
			Question: req.Question, GeneratedSQL: sql,
			Rows: []map[string]interface{}{{"error": err.Error()}}, RowCount: 0,
		})
		return
	}

	if rows == nil {
		rows = emptyRows()
	}
	c.JSON(200, models.SQLQueryResponse{
		Question: req.Question, GeneratedSQL: sql, Rows: rows, RowCount: len(rows),
	})
}

func (s *AIInsightsService) generateSQL(question string) (string, error) {
	return s.generateSQLWithSchema(question)
}

func mockSQLFromQuestion(q string) string {
	lower := strings.ToLower(q)
	switch {
	case strings.Contains(lower, "retained") || strings.Contains(lower, "retention"):
		return `SELECT user_id, COUNT(*) AS event_count
FROM user_events
GROUP BY user_id
HAVING COUNT(*) > 3
ORDER BY event_count DESC
LIMIT 20`
	case strings.Contains(lower, "conversion") && strings.Contains(lower, "categor"):
		return `SELECT e.category_id,
  COALESCE(c.name, 'Category-' || e.category_id::TEXT) AS category_name,
  ROUND(
    SUM(CASE WHEN e.event_type = 'PURCHASED' THEN 1 ELSE 0 END)::numeric /
    NULLIF(SUM(CASE WHEN e.event_type = 'VIEWED' THEN 1 ELSE 0 END), 0) * 100,
    2
  ) AS conversion_rate
FROM user_events e
LEFT JOIN categories c ON c.id = e.category_id
WHERE e.category_id IS NOT NULL
GROUP BY e.category_id, c.name
ORDER BY conversion_rate DESC NULLS LAST
LIMIT 20`
	case strings.Contains(lower, "active") && strings.Contains(lower, "week"):
		return `SELECT user_id, COUNT(*) AS events_last_7_days
FROM user_events
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY user_id
ORDER BY events_last_7_days DESC
LIMIT 20`
	case strings.Contains(lower, "purchase"):
		return `SELECT e.user_id, e.item_id, c.title, e.timestamp
FROM user_events e
LEFT JOIN content_catalog c ON c.item_id = e.item_id
WHERE e.event_type = 'PURCHASED'
ORDER BY e.timestamp DESC
LIMIT 50`
	case strings.Contains(lower, "catalog") || strings.Contains(lower, "product") || strings.Contains(lower, "item"):
		return `SELECT item_id, title, category, price
FROM content_catalog
ORDER BY price DESC
LIMIT 20`
	case strings.Contains(lower, "roi") || strings.Contains(lower, "analytics"):
		return `SELECT retention_rate, ctr, conversion_rate, engagement_score, roi_percentage, active_users, recorded_at
FROM analytics_metrics
ORDER BY recorded_at DESC
LIMIT 20`
	default:
		return `SELECT user_id, COUNT(*) AS total_events
FROM user_events
GROUP BY user_id
ORDER BY total_events DESC
LIMIT 20`
	}
}

func (s *AIInsightsService) executeSafeSQL(sql string) ([]map[string]interface{}, error) {
	normalized := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(normalized, "SELECT") {
		return nil, fmt.Errorf("only SELECT queries allowed")
	}
	forbidden := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE", "CREATE", "GRANT"}
	for _, f := range forbidden {
		if strings.Contains(normalized, f) {
			return nil, fmt.Errorf("forbidden SQL operation: %s", f)
		}
	}

	rows, err := s.db.Raw(sql).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return emptyRows(), err
	}

	results := emptyRows()
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return results, fmt.Errorf("scan row: %w", err)
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = normalizeDBValue(vals[i])
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return results, err
	}
	return results, nil
}
