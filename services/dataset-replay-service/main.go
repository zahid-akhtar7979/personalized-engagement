package main

import (
	"context"
	"encoding/csv"
	"fmt"
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

type ReplayService struct {
	log       *zap.Logger
	kafka     *kafka.Client
	writer    *kafka.Writer
	csvPath   string
	mu        sync.Mutex
	state     models.ReplayState
	events    []models.UserEvent
	cancel    context.CancelFunc
	speed     float64
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
		log:     log,
		kafka:   kc,
		writer:  kc.NewWriter(kafka.TopicUserEvents),
		csvPath: cfg.EventsCSVPath,
		state:   models.ReplayState{Speed: 1.0},
		speed:   1.0,
	}

	if err := svc.loadEvents(); err != nil {
		log.Fatal("load events", zap.Error(err))
	}

	r := gin.Default()
	r.Use(corsMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/replay/status", svc.getStatus)
	r.POST("/api/replay/start", svc.startReplay)
	r.POST("/api/replay/pause", svc.pauseReplay)
	r.POST("/api/replay/resume", svc.resumeReplay)
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

func (s *ReplayService) loadEvents() error {
	f, err := os.Open(s.csvPath)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	header := make(map[string]int)
	var events []models.UserEvent
	for i, row := range records {
		if i == 0 {
			for j, col := range row {
				header[strings.ToLower(strings.TrimSpace(col))] = j
			}
			continue
		}
		if len(row) < 4 {
			continue
		}
		tsStr := colVal(row, header, "timestamp", 0)
		ts, _ := time.Parse("2006-01-02 15:04:05", tsStr)
		if ts.IsZero() {
			ts, _ = time.Parse(time.RFC3339, tsStr)
		}
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		userID, _ := strconv.ParseInt(colVal(row, header, "visitorid", 1), 10, 64)
		itemID, _ := strconv.ParseInt(colVal(row, header, "itemid", 3), 10, 64)
		catID, _ := strconv.ParseInt(colVal(row, header, "categoryid", -1), 10, 64)
		if catID == 0 {
			catID = itemID%6 + 1
		}
		evType := colVal(row, header, "event", 2)
		events = append(events, models.UserEvent{
			EventID:    uuid.New().String(),
			UserID:     userID,
			ItemID:     itemID,
			CategoryID: catID,
			EventType:  mapEventType(evType),
			Timestamp:  ts,
		})
	}
	s.events = events
	s.state.Total = int64(len(events))
	return nil
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
	switch e {
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
	c.JSON(200, s.state)
}

func (s *ReplayService) startReplay(c *gin.Context) {
	s.mu.Lock()
	if s.state.Running && !s.state.Paused {
		s.mu.Unlock()
		c.JSON(200, s.state)
		return
	}
	s.state.Running = true
	s.state.Paused = false
	if s.state.Processed >= s.state.Total {
		s.state.Processed = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go s.runReplay(ctx)
	c.JSON(200, gin.H{"message": "replay started", "state": s.state})
}

func (s *ReplayService) pauseReplay(c *gin.Context) {
	s.mu.Lock()
	s.state.Paused = true
	s.mu.Unlock()
	c.JSON(200, gin.H{"message": "replay paused"})
}

func (s *ReplayService) resumeReplay(c *gin.Context) {
	s.mu.Lock()
	if !s.state.Running {
		s.state.Running = true
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.mu.Unlock()
		go s.runReplay(ctx)
	} else {
		s.state.Paused = false
		s.mu.Unlock()
	}
	c.JSON(200, gin.H{"message": "replay resumed"})
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
	}
	s.state.Running = false
	s.mu.Unlock()
}

func (s *ReplayService) runReplay(ctx context.Context) {
	s.mu.Lock()
	startIdx := int(s.state.Processed)
	s.mu.Unlock()

	for i := startIdx; i < len(s.events); i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		for {
			s.mu.Lock()
			paused := s.state.Paused
			speed := s.speed
			s.mu.Unlock()
			if !paused {
				break
			}
			time.Sleep(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			default:
			}
			_ = speed
		}

		ev := s.events[i]
		key := fmt.Sprintf("%d", ev.UserID)
		if err := kafka.PublishWithRetry(ctx, s.writer, key, ev, s.log, 3); err != nil {
			s.log.Error("publish event", zap.Error(err))
		}

		s.mu.Lock()
		s.state.Processed = int64(i + 1)
		s.mu.Unlock()

		s.mu.Lock()
		delay := time.Duration(float64(500) / s.speed) * time.Millisecond
		s.mu.Unlock()
		time.Sleep(delay)
	}

	s.mu.Lock()
	s.state.Running = false
	s.mu.Unlock()
	s.log.Info("replay completed")
}
