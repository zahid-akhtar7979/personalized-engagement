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
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.refreshInsightsFromDB()
	go svc.consumeAnalytics(ctx)
	go svc.runRetentionAgent(ctx)
	go svc.periodicDBInsights(ctx)

	if svc.cfg.UseMockAI || svc.cfg.OpenAIAPIKey == "" {
		log.Warn("Ask PEP: mock SQL mode (set PEP_OPENAI_API_KEY and PEP_USE_MOCK_AI=false for OpenAI)")
	} else {
		log.Info("Ask PEP: OpenAI SQL generation enabled", zap.String("model", svc.cfg.OpenAIModel))
	}

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/ai/status", svc.aiStatus)
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
	if metrics.TotalUsers == 0 && metrics.Views == 0 {
		return
	}

	if s.prevRetention > 0 && metrics.RetentionRate < s.prevRetention-5 {
		s.addAlertUnique(models.RetentionAlert{
			AlertType:      "RETENTION_DROP",
			Category:       "Platform-wide",
			DropPercentage: s.prevRetention - metrics.RetentionRate,
			Suggestion:     "Increase personalized recommendations and re-engagement campaigns",
			Timestamp:      time.Now().UTC(),
		})
	}

	if metrics.Views > 100 && metrics.ConversionRate < 2 {
		s.addAlertUnique(models.RetentionAlert{
			AlertType:      "LOW_CONVERSION",
			Category:       "Platform-wide",
			DropPercentage: 2 - metrics.ConversionRate,
			Suggestion:     fmt.Sprintf("Conversion is %.2f%% — optimize cart recommendations and checkout flow", metrics.ConversionRate),
			Timestamp:      time.Now().UTC(),
		})
	}

	if metrics.CTR < 5 && metrics.Views > 100 {
		s.addAlertUnique(models.RetentionAlert{
			AlertType:      "LOW_CTR",
			Category:       "Recommendations",
			DropPercentage: 5 - metrics.CTR,
			Suggestion:     fmt.Sprintf("Recommendation CTR is %.1f%% — test new ranking rules and placement", metrics.CTR),
			Timestamp:      time.Now().UTC(),
		})
	}

	s.prevRetention = metrics.RetentionRate
}

func (s *AIInsightsService) refreshInsightsFromDB() {
	type row struct {
		CategoryName   string  `gorm:"column:category_name"`
		Views          int64   `gorm:"column:views"`
		ConversionRate float64 `gorm:"column:conversion_rate"`
	}

	var rows []row
	err := s.db.Raw(`
		SELECT
			COALESCE('Category-' || e.category_id::TEXT, 'Unknown') AS category_name,
			COUNT(*) FILTER (WHERE e.event_type = 'VIEWED') AS views,
			ROUND(
				100.0 * COUNT(*) FILTER (WHERE e.event_type = 'PURCHASED') /
				NULLIF(COUNT(*) FILTER (WHERE e.event_type = 'VIEWED'), 0),
				2
			) AS conversion_rate
		FROM user_events e
		WHERE e.category_id IS NOT NULL
		GROUP BY e.category_id
		HAVING COUNT(*) FILTER (WHERE e.event_type = 'VIEWED') >= 50
		ORDER BY conversion_rate ASC NULLS LAST
		LIMIT 8
	`).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return
	}

	s.alertsMu.Lock()
	s.alerts = s.alerts[:0]
	s.alertsMu.Unlock()

	for _, r := range rows {
		if r.ConversionRate >= 2 {
			continue
		}
		s.addAlertUnique(models.RetentionAlert{
			AlertType:      "LOW_CONVERSION",
			Category:       r.CategoryName,
			DropPercentage: 2 - r.ConversionRate,
			Suggestion: fmt.Sprintf(
				"%s conversion is %.2f%% (%d views) — add targeted bundles and cart prompts",
				r.CategoryName, r.ConversionRate, r.Views,
			),
			Timestamp: time.Now().UTC(),
		})
	}

	// Highlight top-performing category
	var top row
	_ = s.db.Raw(`
		SELECT
			COALESCE('Category-' || e.category_id::TEXT, 'Unknown') AS category_name,
			COUNT(*) FILTER (WHERE e.event_type = 'VIEWED') AS views,
			ROUND(
				100.0 * COUNT(*) FILTER (WHERE e.event_type = 'PURCHASED') /
				NULLIF(COUNT(*) FILTER (WHERE e.event_type = 'VIEWED'), 0),
				2
			) AS conversion_rate
		FROM user_events e
		WHERE e.category_id IS NOT NULL
		GROUP BY e.category_id
		HAVING COUNT(*) FILTER (WHERE e.event_type = 'VIEWED') >= 50
		ORDER BY conversion_rate DESC NULLS LAST
		LIMIT 1
	`).Scan(&top).Error
	if top.Views > 0 && top.ConversionRate > 0 {
		s.addAlertUnique(models.RetentionAlert{
			AlertType:      "HIGH_PERFORMER",
			Category:       top.CategoryName,
			DropPercentage: top.ConversionRate,
			Suggestion: fmt.Sprintf(
				"%s leads with %.2f%% conversion — replicate its recommendation strategy in weaker categories",
				top.CategoryName, top.ConversionRate,
			),
			Timestamp: time.Now().UTC(),
		})
	}
}

func (s *AIInsightsService) periodicDBInsights(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshInsightsFromDB()
		}
	}
}

func (s *AIInsightsService) addAlertUnique(alert models.RetentionAlert) {
	s.alertsMu.Lock()
	defer s.alertsMu.Unlock()
	for _, existing := range s.alerts {
		if existing.AlertType == alert.AlertType && existing.Category == alert.Category {
			return
		}
	}
	s.alerts = append([]models.RetentionAlert{alert}, s.alerts...)
	if len(s.alerts) > 50 {
		s.alerts = s.alerts[:50]
	}
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

func (s *AIInsightsService) getAlerts(c *gin.Context) {
	s.alertsMu.RLock()
	if len(s.alerts) == 0 {
		s.alertsMu.RUnlock()
		s.refreshInsightsFromDB()
		s.alertsMu.RLock()
	}
	out := make([]models.RetentionAlert, len(s.alerts))
	copy(out, s.alerts)
	s.alertsMu.RUnlock()
	c.JSON(200, out)
}

func (s *AIInsightsService) sqlAssistant(c *gin.Context) {
	var req models.SQLQueryRequest
	if err := c.BindJSON(&req); err != nil || req.Question == "" {
		c.JSON(400, gin.H{"error": "question required"})
		return
	}

	resp, err := s.runNaturalLanguageQuery(strings.TrimSpace(req.Question))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, models.SQLQueryResponse{
		Question:     req.Question,
		GeneratedSQL: resp.GeneratedSQL,
		Rows:         resp.Data,
		RowCount:     resp.RowCount,
	})
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
