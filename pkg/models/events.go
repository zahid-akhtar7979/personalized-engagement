package models

import "time"

const (
	EventTypeViewed      = "VIEWED"
	EventTypeAddToCart   = "ADD_TO_CART"
	EventTypePurchased   = "PURCHASED"
	EventTypeRecClick    = "REC_CLICK"
	EventTypeRecImpress  = "REC_IMPRESSION"
)

type UserEvent struct {
	EventID    string    `json:"eventId"`
	UserID     int64     `json:"userId"`
	ItemID     int64     `json:"itemId"`
	CategoryID int64     `json:"categoryId"`
	EventType  string    `json:"eventType"`
	Timestamp  time.Time `json:"timestamp"`
}

type RecommendationEvent struct {
	UserID          int64              `json:"userId"`
	Recommendations PersonalizedRecs   `json:"recommendations"`
	Timestamp       time.Time          `json:"timestamp"`
}

type PersonalizedRecs struct {
	RecommendedForYou  []Recommendation `json:"recommendedForYou"`
	TrendingNow        []Recommendation `json:"trendingNow"`
	BecauseYouViewed   []Recommendation `json:"becauseYouViewed"`
	CartRecommendations []Recommendation `json:"cartRecommendations"`
}

type Recommendation struct {
	ItemID      int64   `json:"itemId"`
	Title       string  `json:"title"`
	Category    string  `json:"category"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
	ImageURL    string  `json:"imageUrl,omitempty"`
}

type DashboardPayload struct {
	UserID          int64              `json:"userId"`
	Recommendations PersonalizedRecs   `json:"recommendations"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

type AnalyticsMetrics struct {
	RetentionRate    float64   `json:"retentionRate"`
	CTR              float64   `json:"ctr"`
	ConversionRate   float64   `json:"conversionRate"`
	EngagementScore  float64   `json:"engagementScore"`
	ROIPercentage    float64   `json:"roiPercentage"`
	ActiveUsers      int64     `json:"activeUsers"`
	TotalUsers       int64     `json:"totalUsers"`
	ReturningUsers   int64     `json:"returningUsers"`
	RecClicks        int64     `json:"recClicks"`
	RecImpressions   int64     `json:"recImpressions"`
	Purchases        int64     `json:"purchases"`
	Views            int64     `json:"views"`
	RevenueGain      float64   `json:"revenueGain"`
	SystemCost       float64   `json:"systemCost"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type ActivityFeedItem struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	UserID    int64     `json:"userId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
}

type RetentionAlert struct {
	AlertType      string  `json:"alertType"`
	Category       string  `json:"category"`
	DropPercentage float64 `json:"dropPercentage"`
	Suggestion     string  `json:"suggestion"`
	Timestamp      time.Time `json:"timestamp"`
}

type SQLQueryRequest struct {
	Question string `json:"question"`
}

type SQLQueryResponse struct {
	Question   string        `json:"question"`
	GeneratedSQL string      `json:"generatedSql"`
	Rows       []map[string]interface{} `json:"rows"`
	RowCount   int           `json:"rowCount"`
}

type AIQueryRequest struct {
	Question string `json:"question"`
}

type AIQueryResponse struct {
	GeneratedSQL string                   `json:"generatedSql"`
	Summary      string                   `json:"summary"`
	Data         []map[string]interface{} `json:"data"`
	RowCount     int                      `json:"rowCount"`
	Source       string                   `json:"source,omitempty"` // "openai" | "mock"
}

type ReplayState struct {
	Running      bool    `json:"running"`
	Paused       bool    `json:"paused"`
	Speed        float64 `json:"speed"`
	Processed    int64   `json:"processed"`
	Total        int64   `json:"total"`
	BatchSize    int     `json:"batchSize"`
	LastBatchEnd int64   `json:"lastBatchEnd"`
	HasMore      bool    `json:"hasMore"`
}
