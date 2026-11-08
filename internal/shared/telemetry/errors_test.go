package telemetry

import (
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func attributeKey(k string) attribute.Key { return attribute.Key(k) }

func TestRateLimitedErrorHandler(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	h := newErrorHandler(zap.New(core), time.Minute)
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return clock }

	h.Handle(errors.New("export failed"))
	if logs.Len() != 1 {
		t.Fatalf("first error should be logged, got %d entries", logs.Len())
	}

	for range 50 {
		clock = clock.Add(time.Second)
		h.Handle(errors.New("export failed"))
	}
	if logs.Len() != 1 {
		t.Fatalf("errors within the interval must be suppressed, got %d entries", logs.Len())
	}

	clock = clock.Add(time.Minute)
	h.Handle(errors.New("export failed"))
	if logs.Len() != 2 {
		t.Fatalf("an error after the interval should be logged, got %d entries", logs.Len())
	}
	got := logs.All()[1].ContextMap()["suppressed_since_last_log"]
	if got != int64(50) {
		t.Errorf("suppressed_since_last_log = %v, want 50", got)
	}
}
