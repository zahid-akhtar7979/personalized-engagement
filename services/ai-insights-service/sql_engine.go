package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"personalized-engagement/pkg/models"
)

const postgresSchema = `
PostgreSQL schema (use only these tables and columns):

TABLE users (
  id BIGINT PRIMARY KEY,
  username VARCHAR(255) UNIQUE NOT NULL,
  email VARCHAR(255),
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

TABLE categories (
  id BIGINT PRIMARY KEY,
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

func (s *AIInsightsService) aiStatus(c *gin.Context) {
	c.JSON(200, gin.H{
		"llmEnabled":  !s.cfg.UseMockAI && s.cfg.OpenAIAPIKey != "",
		"model":       s.cfg.OpenAIModel,
		"useMockAI":   s.cfg.UseMockAI,
		"hasApiKey":   s.cfg.OpenAIAPIKey != "",
	})
}

func (s *AIInsightsService) aiQuery(c *gin.Context) {
	var req models.AIQueryRequest
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Question) == "" {
		c.JSON(400, gin.H{"error": "question required"})
		return
	}

	resp, err := s.runNaturalLanguageQuery(strings.TrimSpace(req.Question))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, resp)
}

func (s *AIInsightsService) runNaturalLanguageQuery(question string) (models.AIQueryResponse, error) {
	sql, source, err := s.generateSQLWithSchema(question)
	if err != nil {
		return models.AIQueryResponse{}, err
	}

	data, execErr := s.executeSafeSQL(sql)
	if execErr != nil {
		return models.AIQueryResponse{
			GeneratedSQL: sql,
			Summary:      fmt.Sprintf("Query failed: %s", execErr.Error()),
			Data:         []map[string]interface{}{{"error": execErr.Error()}},
			RowCount:     0,
			Source:       source,
		}, nil
	}
	if data == nil {
		data = emptyRows()
	}

	summary := s.generateSummary(question, sql, data, source)
	return models.AIQueryResponse{
		GeneratedSQL: sql,
		Summary:      summary,
		Data:         data,
		RowCount:     len(data),
		Source:       source,
	}, nil
}

func (s *AIInsightsService) generateSQLWithSchema(question string) (sql string, source string, err error) {
	if s.cfg.UseMockAI || s.cfg.OpenAIAPIKey == "" {
		return mockSQLFromQuestion(question), "mock", nil
	}

	sql, err = s.openAISQLWithSchema(question)
	if err != nil {
		return "", "", fmt.Errorf("OpenAI SQL generation failed: %w", err)
	}
	return sql, "openai", nil
}

func (s *AIInsightsService) openAISQLWithSchema(question string) (string, error) {
	system := `You are a PostgreSQL SQL assistant for an e-commerce personalization platform (Retailrocket-style event data).

Rules:
- Generate ONLY one valid PostgreSQL SELECT statement.
- Never use INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE, CREATE, GRANT, or semicolons with multiple statements.
- Use only tables and columns from the schema below.
- Prefer LIMIT 20–50 on large result sets.
- Use event_type values exactly: VIEWED, ADD_TO_CART, PURCHASED.
- Return ONLY the raw SQL query with no markdown, explanation, or code fences.`

	user := fmt.Sprintf("%s\n\nBusiness question: %s", postgresSchema, question)

	sql, err := s.chatCompletion(system, user, 0)
	if err != nil {
		return "", err
	}
	sql = cleanSQL(sql)
	if sql == "" {
		return "", fmt.Errorf("empty SQL from model")
	}
	return sql, nil
}

func (s *AIInsightsService) generateSummary(question, sql string, data []map[string]interface{}, source string) string {
	if source == "mock" || s.cfg.OpenAIAPIKey == "" {
		return mockSummaryFromQuestion(question, len(data))
	}

	system := `You summarize database query results in one short sentence (under 20 words).
Be conversational. Do not mention SQL or the database.
Example: "Here are the top 20 users by event count."`

	preview, _ := json.Marshal(truncateRows(data, 5))
	user := fmt.Sprintf("Question: %s\nRows returned: %d\nSample JSON rows: %s", question, len(data), string(preview))

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
	// Take first statement only if model returned multiple lines with comments
	lines := strings.Split(sql, "\n")
	var parts []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		parts = append(parts, t)
	}
	if len(parts) > 0 {
		sql = strings.Join(parts, " ")
	}
	return strings.TrimSpace(strings.TrimSuffix(sql, ";"))
}

func (s *AIInsightsService) chatCompletion(system, user string, temperature float64) (string, error) {
	model := s.cfg.OpenAIModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model": model,
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

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &errBody)
		msg := errBody.Error.Message
		if msg == "" {
			msg = string(data)
		}
		return "", fmt.Errorf("OpenAI API %d: %s", resp.StatusCode, msg)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}
	return result.Choices[0].Message.Content, nil
}
