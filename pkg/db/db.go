package db

import (
	"personalized-engagement/pkg/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.ContentCatalog{},
		&models.Category{},
		&models.UserEventRecord{},
		&models.RecommendationRecord{},
		&models.AnalyticsMetricRecord{},
	)
}
