package logger

import (
	"context"
	"fmt"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
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
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return contextFieldFilter{Core: core}
		}),
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

// WithOTEL adds the OpenTelemetry bridge to the existing stdout core. Call it
// after telemetry.Setup so the bridge uses the installed global LoggerProvider.
func WithOTEL(logger *zap.Logger) *zap.Logger {
	return logger.WithOptions(zap.WrapCore(func(stdout zapcore.Core) zapcore.Core {
		return zapcore.NewTee(
			contextFieldFilter{Core: stdout},
			otelzap.NewCore(
				"github.com/cristianortiz/dev-forge/internal/shared/logger",
				otelzap.WithLoggerProvider(otel.GetLoggerProvider()),
			),
		)
	}))
}

// WithContext passes the request context to otelzap and adds trace identifiers
// to stdout. Unsampled spans are cleared so logs do not point to absent traces.
func WithContext(logger *zap.Logger, ctx context.Context) *zap.Logger {
	if ctx == nil {
		return logger
	}

	spanContext := trace.SpanContextFromContext(ctx)
	fields := []zap.Field{zap.Any("context", ctx)}
	if spanContext.IsValid() && spanContext.IsSampled() {
		fields = append(fields,
			zap.String("trace_id", spanContext.TraceID().String()),
			zap.String("span_id", spanContext.SpanID().String()),
		)
	} else if spanContext.IsValid() {
		ctx = trace.ContextWithSpanContext(ctx, trace.SpanContext{})
		fields[0] = zap.Any("context", ctx)
	}

	return logger.With(fields...)
}

// contextFieldFilter keeps otelzap's context field out of console output; the
// trace and span identifiers are added separately by WithContext.
type contextFieldFilter struct {
	zapcore.Core
}

func (c contextFieldFilter) With(fields []zap.Field) zapcore.Core {
	return contextFieldFilter{Core: c.Core.With(withoutContext(fields))}
}

func (c contextFieldFilter) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c contextFieldFilter) Write(entry zapcore.Entry, fields []zap.Field) error {
	return c.Core.Write(entry, withoutContext(fields))
}

func withoutContext(fields []zap.Field) []zap.Field {
	filtered := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		if field.Key != "context" {
			filtered = append(filtered, field)
		}
	}
	if len(filtered) == len(fields) {
		return fields
	}
	return filtered
}
