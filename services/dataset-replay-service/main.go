package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"personalized-engagement/pkg/config"
	"personalized-engagement/pkg/kafka"
	"personalized-engagement/pkg/logger"
	"personalized-engagement/pkg/models"
)

const defaultBatchSize = 100

type ReplayService struct {
	log       *zap.Logger
	kafka     *kafka.Client
	writer    *kafka.Writer
	csvPath   string
	header    map[string]int
	mu        sync.Mutex
	state     models.ReplayState
	cancel    context.CancelFunc
	speed     float64
	batchSize int
}

func main() {
	cfg := config.Load("dataset-replay-service")
	log := logger.New(cfg.ServiceName)

	topics := []string{kafka.TopicUserEvents, kafka.TopicRecommendationEvents, kafka.TopicDashboardEvents, kafka.TopicAnalyticsEvents}
	if err := kafka.EnsureTopics(cfg.KafkaBrokers, topics); err != nil {
		log.Warn("topic creation (may already exist)", zap.Error(err))
	}

	kc := kafka.NewClient(cfg.KafkaBrokers, log)
	svc := &ReplayService{
		log:       log,
		kafka:     kc,
		writer:    kc.NewWriter(kafka.TopicUserEvents),
		csvPath:   cfg.EventsCSVPath,
		state:     models.ReplayState{Speed: 1.0, BatchSize: defaultBatchSize},
		speed:     1.0,
		batchSize: defaultBatchSize,
	}

	if err := svc.initCSV(); err != nil {
		log.Fatal("init csv", zap.Error(err))
	}

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/replay/status", svc.getStatus)
	r.POST("/api/replay/start", svc.startReplay)
	r.POST("/api/replay/next-batch", svc.nextBatch)
	r.POST("/api/replay/pause", svc.pauseReplay)
	r.POST("/api/replay/resume", svc.resumeReplay)
	r.POST("/api/replay/reset", svc.resetReplay)
	r.PUT("/api/replay/speed", svc.setSpeed)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		log.Info("dataset-replay-service listening", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	svc.stopReplay()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = svc.writer.Close()
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// initCSV counts rows without loading the full file into memory.
func (s *ReplayService) initCSV() error {
	f, err := os.Open(s.csvPath)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	headerRow, err := reader.Read()
	if err != nil {
		return err
	}
	header := make(map[string]int)
	for j, col := range headerRow {
		header[strings.ToLower(strings.TrimSpace(col))] = j
	}

	var total int64
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total++
	}

	s.header = header
	s.state.Total = total
	s.state.BatchSize = s.batchSize
	s.state.HasMore = total > 0
	s.log.Info("csv indexed", zap.Int64("total", total), zap.String("path", s.csvPath))
	return nil
}

func (s *ReplayService) readBatch(offset, limit int) ([]models.UserEvent, error) {
	f, err := os.Open(s.csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	if _, err := reader.Read(); err != nil { // skip header
		return nil, err
	}

	for i := 0; i < offset; i++ {
		if _, err := reader.Read(); err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, err
		}
	}

	var events []models.UserEvent
	for len(events) < limit {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return events, err
		}
		if len(row) < 4 {
			continue
		}
		events = append(events, s.rowToEvent(row))
	}
	return events, nil
}

func (s *ReplayService) rowToEvent(row []string) models.UserEvent {
	tsStr := colVal(row, s.header, "timestamp", 0)
	ts, _ := time.Parse("2006-01-02 15:04:05", tsStr)
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339, tsStr)
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	userID, _ := strconv.ParseInt(colVal(row, s.header, "visitorid", 1), 10, 64)
	itemID, _ := strconv.ParseInt(colVal(row, s.header, "itemid", 3), 10, 64)
	catID, _ := strconv.ParseInt(colVal(row, s.header, "categoryid", -1), 10, 64)
	if catID == 0 {
		catID = itemID%6 + 1
	}
	return models.UserEvent{
		EventID:    uuid.New().String(),
		UserID:     userID,
		ItemID:     itemID,
		CategoryID: catID,
		EventType:  mapEventType(colVal(row, s.header, "event", 2)),
		Timestamp:  ts,
	}
}

func colVal(row []string, header map[string]int, name string, fallback int) string {
	if idx, ok := header[name]; ok && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	if fallback >= 0 && fallback < len(row) {
		return strings.TrimSpace(row[fallback])
	}
	return ""
}

func mapEventType(e string) string {
	switch strings.ToLower(strings.TrimSpace(e)) {
	case "view":
		return models.EventTypeViewed
	case "addtocart":
		return models.EventTypeAddToCart
	case "transaction":
		return models.EventTypePurchased
	default:
		return models.EventTypeViewed
	}
}

func (s *ReplayService) getStatus(c *gin.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.HasMore = s.state.Processed < s.state.Total
	s.state.BatchSize = s.batchSize
	c.JSON(200, s.state)
}

func (s *ReplayService) startReplay(c *gin.Context) {
	s.mu.Lock()
	if s.state.Running {
		s.mu.Unlock()
		c.JSON(409, gin.H{"error": "replay already running"})
		return
	}
	// Start always replays the first batch from the beginning
	s.state.Processed = 0
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state.Running = true
	s.state.Paused = false
	s.mu.Unlock()

	go s.runBatch(ctx, 0)
	c.JSON(200, gin.H{"message": fmt.Sprintf("replaying first %d events", s.batchSize)})
}

func (s *ReplayService) nextBatch(c *gin.Context) {
	s.mu.Lock()
	if s.state.Running {
		s.mu.Unlock()
		c.JSON(409, gin.H{"error": "replay already running"})
		return
	}
	if s.state.Processed >= s.state.Total {
		s.mu.Unlock()
		c.JSON(400, gin.H{"error": "all events replayed", "state": s.state})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state.Running = true
	s.state.Paused = false
	start := int(s.state.Processed)
	s.mu.Unlock()

	go s.runBatch(ctx, start)
	c.JSON(200, gin.H{"message": fmt.Sprintf("replaying next %d events", s.batchSize)})
}

func (s *ReplayService) pauseReplay(c *gin.Context) {
	s.mu.Lock()
	s.state.Paused = true
	s.mu.Unlock()
	c.JSON(200, gin.H{"message": "replay paused"})
}

func (s *ReplayService) resumeReplay(c *gin.Context) {
	s.mu.Lock()
	if s.state.Running {
		s.state.Paused = false
		s.mu.Unlock()
		c.JSON(200, gin.H{"message": "replay resumed"})
		return
	}
	start := int(s.state.Processed)
	if start >= int(s.state.Total) {
		s.mu.Unlock()
		c.JSON(400, gin.H{"error": "nothing to resume"})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.state.Running = true
	s.state.Paused = false
	s.mu.Unlock()
	go s.runBatch(ctx, start)
	c.JSON(200, gin.H{"message": "replay resumed"})
}

func (s *ReplayService) resetReplay(c *gin.Context) {
	s.stopReplay()
	s.mu.Lock()
	s.state.Processed = 0
	s.state.HasMore = s.state.Total > 0
	s.mu.Unlock()
	c.JSON(200, gin.H{"message": "replay reset to start"})
}

func (s *ReplayService) setSpeed(c *gin.Context) {
	var req struct {
		Speed float64 `json:"speed"`
	}
	if err := c.BindJSON(&req); err != nil || req.Speed <= 0 {
		c.JSON(400, gin.H{"error": "invalid speed"})
		return
	}
	s.mu.Lock()
	s.speed = req.Speed
	s.state.Speed = req.Speed
	s.mu.Unlock()
	c.JSON(200, s.state)
}

func (s *ReplayService) stopReplay() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.state.Running = false
	s.mu.Unlock()
}

func (s *ReplayService) runBatch(ctx context.Context, startOffset int) {
	s.mu.Lock()
	batchSize := s.batchSize
	s.mu.Unlock()

	events, err := s.readBatch(startOffset, batchSize)
	if err != nil {
		s.log.Error("read batch", zap.Error(err))
		s.mu.Lock()
		s.state.Running = false
		s.mu.Unlock()
		return
	}

	for i, ev := range events {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.state.Running = false
			s.mu.Unlock()
			return
		default:
		}

		for {
			s.mu.Lock()
			paused := s.state.Paused
			s.mu.Unlock()
			if !paused {
				break
			}
			time.Sleep(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				s.mu.Lock()
				s.state.Running = false
				s.mu.Unlock()
				return
			default:
			}
		}

		key := fmt.Sprintf("%d", ev.UserID)
		if err := kafka.PublishWithRetry(ctx, s.writer, key, ev, s.log, 3); err != nil {
			s.log.Error("publish event", zap.Error(err))
		}

		s.mu.Lock()
		s.state.Processed = int64(startOffset + i + 1)
		s.state.LastBatchEnd = s.state.Processed
		s.state.HasMore = s.state.Processed < s.state.Total
		speed := s.speed
		s.mu.Unlock()

		time.Sleep(time.Duration(float64(500)/speed) * time.Millisecond)
	}

	s.mu.Lock()
	s.state.Running = false
	s.state.HasMore = s.state.Processed < s.state.Total
	processed := s.state.Processed
	total := s.state.Total
	s.mu.Unlock()
	s.log.Info("batch completed",
		zap.Int("published", len(events)),
		zap.Int64("processed", processed),
		zap.Int64("total", total),
	)
}
