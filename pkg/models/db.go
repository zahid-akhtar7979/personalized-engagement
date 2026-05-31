package models

import "time"

type User struct {
	ID        int64     `gorm:"primaryKey"`
	Username  string    `gorm:"uniqueIndex"`
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ContentCatalog struct {
	ID         int64   `gorm:"primaryKey;column:id"`
	ItemID     int64   `gorm:"uniqueIndex;column:item_id"`
	Title      string
	CategoryID int64
	Category   string
	Price      float64
	ImageURL   string
}

func (ContentCatalog) TableName() string { return "content_catalog" }

type Category struct {
	ID       int64 `gorm:"primaryKey"`
	Name     string
	ParentID *int64
}

type UserEventRecord struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	EventID    string    `gorm:"uniqueIndex"`
	UserID     int64     `gorm:"index"`
	ItemID     int64
	CategoryID int64
	EventType  string    `gorm:"index"`
	Timestamp  time.Time `gorm:"index"`
}

func (UserEventRecord) TableName() string { return "user_events" }

type RecommendationRecord struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	UserID    int64     `gorm:"index"`
	ItemID    int64
	Score     float64
	Reason    string
	Section   string
	CreatedAt time.Time
}

func (RecommendationRecord) TableName() string { return "recommendations" }

type AnalyticsMetricRecord struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	RetentionRate   float64
	CTR             float64
	ConversionRate  float64
	EngagementScore float64
	ROIPercentage   float64
	ActiveUsers     int64
	RecordedAt      time.Time `gorm:"index"`
}

func (AnalyticsMetricRecord) TableName() string { return "analytics_metrics" }
