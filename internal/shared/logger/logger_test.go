package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
)

type capturedLog struct {
	embedded.Logger
	mu     sync.Mutex
	ctx    context.Context
	record log.Record
}

func (l *capturedLog) Enabled(context.Context, log.EnabledParameters) bool {
	return true
}

func (l *capturedLog) Emit(ctx context.Context, record log.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ctx = ctx
	l.record = record.Clone()
}

type capturedProvider struct {
	embedded.LoggerProvider
	logger *capturedLog
}

func (p *capturedProvider) Logger(string, ...log.LoggerOption) log.Logger {
	return p.logger
}

func TestNewDebugLoggerKeepsErrorWithoutStackTrace(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = write
	t.Cleanup(func() {
		os.Stderr = originalStderr
		_ = write.Close()
		_ = read.Close()
	})

	logger, err := New(&config.Config{Debug: true, Environment: "development"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	logger.Error("handler error", zap.Error(errors.New("business validation failed")))
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = originalStderr

	output, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "handler error") ||
		!strings.Contains(string(output), "business validation failed") {
		t.Errorf("error message was not retained: %s", output)
	}
	if strings.Contains(string(output), "goroutine ") {
		t.Errorf("logger printed a Go stack trace: %s", output)
	}
}

func TestWithContextCorrelatesOTLPAndStdout(t *testing.T) {
	provider := &capturedProvider{logger: &capturedLog{}}
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() { otel.SetLoggerProvider(noop.NewLoggerProvider()) })

	var stdout bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&stdout),
		zapcore.DebugLevel,
	)
	logger := WithOTEL(zap.New(core))

	wantTraceID := trace.TraceID{1, 2, 3}
	wantSpanID := trace.SpanID{4, 5, 6}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    wantTraceID,
		SpanID:     wantSpanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	WithContext(logger, ctx).Error("request failed", zap.String("reason", "database"))

	provider.logger.mu.Lock()
	gotContext := trace.SpanContextFromContext(provider.logger.ctx)
	var hasContextAttribute bool
	provider.logger.record.WalkAttributes(func(kv attribute.KeyValue) bool {
		hasContextAttribute = hasContextAttribute || kv.Key == "context"
		return true
	})
	provider.logger.mu.Unlock()
	if gotContext.TraceID() != wantTraceID || gotContext.SpanID() != wantSpanID {
		t.Fatalf("OTLP context = %v, want trace %s span %s", gotContext, wantTraceID, wantSpanID)
	}
	if hasContextAttribute {
		t.Error("context was exported as a log attribute instead of trace context")
	}

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &entry); err != nil {
		t.Fatalf("decode stdout JSON: %v", err)
	}
	if entry["trace_id"] != wantTraceID.String() || entry["span_id"] != wantSpanID.String() {
		t.Errorf("stdout correlation IDs = %v/%v, want %s/%s", entry["trace_id"], entry["span_id"], wantTraceID, wantSpanID)
	}
	if _, ok := entry["context"]; ok {
		t.Error("stdout contains the internal context field")
	}
}

func TestWithContextOmitsUnsampledIDs(t *testing.T) {
	provider := &capturedProvider{logger: &capturedLog{}}
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() { otel.SetLoggerProvider(noop.NewLoggerProvider()) })

	var stdout bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&stdout),
		zapcore.DebugLevel,
	)
	logger := WithOTEL(zap.New(core))
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3},
		SpanID:  trace.SpanID{4, 5, 6},
	})
	WithContext(logger, trace.ContextWithSpanContext(context.Background(), spanContext)).Info("unsampled")

	if got := trace.SpanContextFromContext(provider.logger.ctx); got.IsValid() {
		t.Errorf("OTLP log retained unsampled trace context %v", got)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &entry); err != nil {
		t.Fatalf("decode stdout JSON: %v", err)
	}
	for _, key := range []string{"trace_id", "span_id"} {
		if _, ok := entry[key]; ok {
			t.Errorf("stdout contains %s for an unsampled span", key)
		}
	}
}

func TestWithContextWithoutSpanOmitsCorrelationIDs(t *testing.T) {
	provider := &capturedProvider{logger: &capturedLog{}}
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() { otel.SetLoggerProvider(noop.NewLoggerProvider()) })

	var stdout bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&stdout),
		zapcore.DebugLevel,
	)
	WithContext(WithOTEL(zap.New(core)), context.Background()).Info("without span")

	if got := trace.SpanContextFromContext(provider.logger.ctx); got.IsValid() {
		t.Errorf("OTLP log unexpectedly has trace context %v", got)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &entry); err != nil {
		t.Fatalf("decode stdout JSON: %v", err)
	}
	for _, key := range []string{"trace_id", "span_id"} {
		if _, ok := entry[key]; ok {
			t.Errorf("stdout contains %s without a span", key)
		}
	}
}
