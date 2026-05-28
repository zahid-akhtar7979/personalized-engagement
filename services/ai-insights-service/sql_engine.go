package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"personalized-engagement/pkg/models"
)

const postgresSchema = `
PostgreSQL schema (use only these tables and columns):

TABLE users (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(255) UNIQUE NOT NULL,
  email VARCHAR(255),
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

TABLE categories (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  parent_id BIGINT REFERENCES categories(id)
);

TABLE content_catalog (
  id BIGSERIAL PRIMARY KEY,
  item_id BIGINT UNIQUE NOT NULL,
  title VARCHAR(500) NOT NULL,
  category_id BIGINT,
  category VARCHAR(255),
  price DECIMAL(10,2),
  image_url VARCHAR(500)
);

TABLE user_events (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(64) UNIQUE NOT NULL,
  user_id BIGINT NOT NULL,
  item_id BIGINT,
  category_id BIGINT,
  event_type VARCHAR(32) NOT NULL,
  timestamp TIMESTAMPTZ NOT NULL
);
-- event_type values: VIEWED, ADD_TO_CART, PURCHASED

TABLE recommendations (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  item_id BIGINT NOT NULL,
  score DECIMAL(8,4),
  reason VARCHAR(255),
  section VARCHAR(64),
  created_at TIMESTAMPTZ
);

TABLE analytics_metrics (
  id BIGSERIAL PRIMARY KEY,
  retention_rate DECIMAL(8,4),
  ctr DECIMAL(8,4),
  conversion_rate DECIMAL(8,4),
  engagement_score DECIMAL(12,2),
  roi_percentage DECIMAL(8,4),
  active_users BIGINT,
  recorded_at TIMESTAMPTZ
);
`

func (s *AIInsightsService) aiQuery(c *gin.Context) {
	var req models.AIQueryRequest
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Question) == "" {
		c.JSON(400, gin.H{"error": "question required"})
		return
	}

	question := strings.TrimSpace(req.Question)
	sql, err := s.generateSQLWithSchema(question)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	data, execErr := s.executeSafeSQL(sql)
	if execErr != nil {
		c.JSON(200, models.AIQueryResponse{
			GeneratedSQL: sql,
			Summary:      fmt.Sprintf("Query failed: %s", execErr.Error()),
			Data:         []map[string]interface{}{{"error": execErr.Error()}},
			RowCount:     0,
		})
		return
	}
	if data == nil {
		data = emptyRows()
	}

	summary := s.generateSummary(question, sql, data)
	c.JSON(200, models.AIQueryResponse{
		GeneratedSQL: sql,
		Summary:      summary,
		Data:         data,
		RowCount:     len(data),
	})
}

func (s *AIInsightsService) generateSQLWithSchema(question string) (string, error) {
	if s.cfg.UseMockAI || s.cfg.OpenAIAPIKey == "" {
		return mockSQLFromQuestion(question), nil
	}
	return s.openAISQLWithSchema(question)
}

func (s *AIInsightsService) openAISQLWithSchema(question string) (string, error) {
	system := `You are a PostgreSQL SQL assistant for an e-commerce analytics platform.
Rules:
- Generate ONLY a single SELECT statement.
- Never use INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE, CREATE, or GRANT.
- Use only tables and columns from the provided schema.
- Return ONLY the raw SQL query with no markdown, explanation, or code fences.`

	user := fmt.Sprintf("%s\n\nBusiness question: %s", postgresSchema, question)

	sql, err := s.chatCompletion(system, user, 0)
	if err != nil || sql == "" {
		return mockSQLFromQuestion(question), nil
	}
	return cleanSQL(sql), nil
}

func (s *AIInsightsService) generateSummary(question, sql string, data []map[string]interface{}) string {
	if s.cfg.UseMockAI || s.cfg.OpenAIAPIKey == "" {
		return mockSummaryFromQuestion(question, len(data))
	}

	system := `You summarize database query results in one short spoken sentence (under 15 words).
Examples: "Here are the top retained users." or "Found 12 purchases this week."
Do not mention SQL. Be conversational.`

	preview, _ := json.Marshal(truncateRows(data, 5))
	user := fmt.Sprintf("Question: %s\nRows returned: %d\nSample data: %s", question, len(data), string(preview))

	summary, err := s.chatCompletion(system, user, 0.3)
	if err != nil || summary == "" {
		return mockSummaryFromQuestion(question, len(data))
	}
	return strings.TrimSpace(summary)
}

func mockSummaryFromQuestion(q string, rowCount int) string {
	lower := strings.ToLower(q)
	switch {
	case strings.Contains(lower, "retained") || strings.Contains(lower, "retention"):
		if rowCount == 0 {
			return "No retained users found yet. Try running dataset replay first."
		}
		return "Here are the top retained users."
	case strings.Contains(lower, "conversion") && strings.Contains(lower, "categor"):
		return "Here are the highest conversion categories."
	case strings.Contains(lower, "active") && strings.Contains(lower, "week"):
		return "Here are the most active users this week."
	case strings.Contains(lower, "purchase"):
		return "Here are the recent purchases."
	case strings.Contains(lower, "roi"):
		return "Here is the ROI analytics history."
	default:
		if rowCount == 0 {
			return "No results found for that question."
		}
		return fmt.Sprintf("Here are your results. %d rows returned.", rowCount)
	}
}

func truncateRows(data []map[string]interface{}, n int) []map[string]interface{} {
	if len(data) <= n {
		return data
	}
	return data[:n]
}

func cleanSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	sql = strings.TrimPrefix(sql, "```sql")
	sql = strings.TrimPrefix(sql, "```SQL")
	sql = strings.TrimPrefix(sql, "```")
	sql = strings.TrimSuffix(sql, "```")
	return strings.TrimSpace(sql)
}

func (s *AIInsightsService) chatCompletion(system, user string, temperature float64) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": temperature,
	})

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("%s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no completion")
	}
	return result.Choices[0].Message.Content, nil
}
