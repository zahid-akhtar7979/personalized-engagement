package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Port            string
	PostgresDSN     string
	RedisAddr       string
	KafkaBrokers    []string
	OpenAIAPIKey    string
	OpenAIModel     string
	UseMockAI       bool
	EventsCSVPath   string
	ServiceName     string
}

func Load(serviceName string) *Config {
	viper.SetEnvPrefix("PEP")
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("POSTGRES_DSN", "host=postgres user=pep password=pep123 dbname=pep port=5432 sslmode=disable")
	viper.SetDefault("REDIS_ADDR", "redis:6379")
	viper.SetDefault("KAFKA_BROKERS", "kafka:9092")
	viper.SetDefault("USE_MOCK_AI", "true")
	viper.SetDefault("OPENAI_MODEL", "gpt-4o-mini")
	viper.SetDefault("EVENTS_CSV_PATH", "/data/events.csv")

	brokers := strings.Split(viper.GetString("KAFKA_BROKERS"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	apiKey := strings.TrimSpace(viper.GetString("OPENAI_API_KEY"))
	useMock := viper.GetBool("USE_MOCK_AI")
	if apiKey == "" {
		useMock = true
	}

	return &Config{
		Port:          viper.GetString("PORT"),
		PostgresDSN:   viper.GetString("POSTGRES_DSN"),
		RedisAddr:     viper.GetString("REDIS_ADDR"),
		KafkaBrokers:  brokers,
		OpenAIAPIKey:  apiKey,
		OpenAIModel:   viper.GetString("OPENAI_MODEL"),
		UseMockAI:     useMock,
		EventsCSVPath: viper.GetString("EVENTS_CSV_PATH"),
		ServiceName:   serviceName,
	}
}
