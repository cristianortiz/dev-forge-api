package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestTracer_RecordsOperationWithoutQueryArguments(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
	})

	tracer := newTracer()

	// The parent stands for the HTTP request span.
	ctx, parent := tp.Tracer("test").Start(context.Background(), "GET /users/:id")
	const secret = "s3cr3t-value-that-must-not-leak"
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT id FROM users WHERE zitadel_id = $1",
		Args: []any{secret},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
	parent.End()

	var query *tracetest.SpanStub
	for _, s := range spans.Ended() {
		if s.Name() == "query SELECT" {
			stub := tracetest.SpanStubFromReadOnlySpan(s)
			query = &stub
		}
	}
	if query == nil {
		t.Fatal(`no "query SELECT" span was recorded`)
	}
	if query.Parent.SpanID() != parent.SpanContext().SpanID() {
		t.Error("the query span is not a child of the request span")
	}

	for _, kv := range query.Attributes {
		if kv.Key == "pgx.query.parameters" {
			t.Errorf("query parameters were recorded: %v", kv.Value.Emit())
		}
		if strings.Contains(kv.Value.Emit(), secret) {
			t.Errorf("attribute %s leaks a query argument", kv.Key)
		}
	}
	if !hasAttribute(query.Attributes, "db.query.text", "SELECT id FROM users WHERE zitadel_id = $1") {
		t.Error("expected db.query.text to carry the parameterized statement")
	}
}

func hasAttribute(attrs []attribute.KeyValue, key, want string) bool {
	for _, kv := range attrs {
		if string(kv.Key) == key && kv.Value.Emit() == want {
			return true
		}
	}
	return false
}

func TestTracer_IgnoresQueriesWithoutARecordingParent(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	t.Cleanup(func() { otel.SetTracerProvider(tracenoop.NewTracerProvider()) })

	tracer := newTracer()
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	if n := len(spans.Ended()); n != 0 {
		t.Errorf("startup or background queries produced %d root spans", n)
	}
}
