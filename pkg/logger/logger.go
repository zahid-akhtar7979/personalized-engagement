package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(service string) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return l.With(zap.String("service", service))
}
