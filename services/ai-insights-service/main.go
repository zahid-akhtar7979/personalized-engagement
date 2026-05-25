package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	c.JSON(200, models.SQLQueryResponse{
		Question: req.Question, GeneratedSQL: sql, Rows: rows, RowCount: len(rows),
	})
}

func (s *AIInsightsService) generateSQL(question string) (string, error) {
	if s.cfg.UseMockAI || s.cfg.OpenAIAPIKey == "" {
		return mockSQLFromQuestion(question), nil
	}
	return s.openAISQL(question)
}

func mockSQLFromQuestion(q string) string {
	lower := strings.ToLower(q)
	switch {
	case strings.Contains(lower, "retained") || strings.Contains(lower, "retention"):
		return `SELECT user_id, COUNT(*) as event_count
FROM user_events
GROUP BY user_id
HAVING COUNT(*) > 3
ORDER BY event_count DESC
LIMIT 20`
	case strings.Contains(lower, "conversion") && strings.Contains(lower, "categor"):
		return `SELECT category_id,
  SUM(CASE WHEN event_type = 'PURCHASED' THEN 1 ELSE 0 END)::float /
  NULLIF(SUM(CASE WHEN event_type = 'VIEWED' THEN 1 ELSE 0 END), 0) * 100 as conversion_rate
FROM user_events
GROUP BY category_id
ORDER BY conversion_rate DESC`
	case strings.Contains(lower, "active") && strings.Contains(lower, "week"):
		return `SELECT user_id, COUNT(*) as events
FROM user_events
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY user_id
ORDER BY events DESC
LIMIT 20`
	case strings.Contains(lower, "purchase"):
		return `SELECT user_id, item_id, timestamp
FROM user_events
WHERE event_type = 'PURCHASED'
ORDER BY timestamp DESC
LIMIT 50`
	default:
		return `SELECT event_type, COUNT(*) as count
FROM user_events
GROUP BY event_type
ORDER BY count DESC`
	}
}

func (s *AIInsightsService) openAISQL(question string) (string, error) {
	prompt := fmt.Sprintf(`Convert this business question to a safe PostgreSQL SELECT query.
Only SELECT statements. Tables: users, user_events, content_catalog, analytics_metrics, recommendations.
Question: %s
Return only the SQL.`, question)

	body, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a SQL assistant. Return only SELECT queries."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	})

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return mockSQLFromQuestion(question), nil
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mockSQLFromQuestion(question), nil
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 {
		return mockSQLFromQuestion(question), nil
	}
	sql := strings.TrimSpace(result.Choices[0].Message.Content)
	sql = strings.TrimPrefix(sql, "```sql")
	sql = strings.TrimPrefix(sql, "```")
	sql = strings.TrimSuffix(sql, "```")
	return strings.TrimSpace(sql), nil
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

	cols, _ := rows.Columns()
	var results []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = vals[i]
		}
		results = append(results, row)
	}
	return results, nil
}
