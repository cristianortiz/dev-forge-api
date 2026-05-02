package logger

import (
	"fmt"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap.Logger from config.
// In development (debug=true) it uses a human-friendly console encoder;
// in production it uses structured JSON.
func New(cfg *config.Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.LogLevel())
	if err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config

	if cfg.Debug {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapCfg = zap.NewProductionConfig()
		zapCfg.Encoding = "json"
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)

	logger, err := zapCfg.Build(
		zap.Fields(
			zap.String("service", cfg.OTEL.ServiceName),
			zap.String("version", cfg.OTEL.ServiceVersion),
			zap.String("env", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %w", err)
	}

	return logger, nil
}
