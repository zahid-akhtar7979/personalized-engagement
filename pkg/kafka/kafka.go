package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type Writer = kafka.Writer
type Reader = kafka.Reader

const (
	TopicUserEvents           = "user-events"
	TopicRecommendationEvents = "recommendation-events"
	TopicDashboardEvents      = "dashboard-events"
	TopicAnalyticsEvents      = "analytics-events"
)

type Client struct {
	brokers []string
	log     *zap.Logger
}

func NewClient(brokers []string, log *zap.Logger) *Client {
	return &Client{brokers: brokers, log: log}
}

func (c *Client) NewWriter(topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(c.brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
}

func (c *Client) NewReader(topic, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        c.brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
}

func PublishJSON(ctx context.Context, w *kafka.Writer, key string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: data,
	})
}

func PublishWithRetry(ctx context.Context, w *kafka.Writer, key string, v interface{}, log *zap.Logger, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := PublishJSON(ctx, w, key, v); err != nil {
			lastErr = err
			log.Warn("kafka publish retry", zap.Int("attempt", i+1), zap.Error(err))
			time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("kafka publish failed after %d retries: %w", maxRetries, lastErr)
}

func EnsureTopics(brokers []string, topics []string) error {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfigs := make([]kafka.TopicConfig, len(topics))
	for i, t := range topics {
		topicConfigs[i] = kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     4,
			ReplicationFactor: 1,
		}
	}
	return controllerConn.CreateTopics(topicConfigs...)
}
