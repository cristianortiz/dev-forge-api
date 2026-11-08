package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	fiberotel "github.com/gofiber/contrib/v3/otel"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// Every handler must hand the use case the request's context: that is how the
// repository's database spans become children of the HTTP span. c.RequestCtx()
// does not carry the span, so a regression there would only show up as
// disconnected traces in production.
func TestHandlers_PassTheRequestSpanToTheUseCase(t *testing.T) {
	tmpl := sampleTemplate()
	id := tmpl.ID.String()

	tests := []struct {
		name, method, path string
		body               any
	}{
		{"list", "GET", "/templates", nil},
		{"get by id", "GET", "/templates/" + id, nil},
		{"create", "POST", "/templates", validCreatePayload()},
		{"update", "PATCH", "/templates/" + id, map[string]any{"is_active": false}},
		{"deactivate", "DELETE", "/templates/" + id, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got context.Context
			svc := &mockTemplateService{
				listFn: func(ctx context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
					got = ctx
					return nil, nil
				},
				getByIDFn: func(ctx context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
					got = ctx
					return tmpl, nil
				},
				createFn: func(ctx context.Context, _ ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
					got = ctx
					return tmpl, nil
				},
				updateFn: func(ctx context.Context, _ uuid.UUID, _ ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
					got = ctx
					return tmpl, nil
				},
				deactivateFn: func(ctx context.Context, _ uuid.UUID) error {
					got = ctx
					return nil
				},
			}

			spans := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
			app := fiber.New()
			app.Use(fiberotel.New(fiberotel.WithTracerProvider(tp)))
			New(svc, zap.NewNop()).RegisterRoutes(app, passThrough, passThrough)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.body != nil {
				req = httptest.NewRequest(tt.method, tt.path, jsonBody(t, tt.body))
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode >= 400 {
				t.Fatalf("status = %d, want success", resp.StatusCode)
			}
			if got == nil {
				t.Fatal("the use case was not called")
			}

			ended := spans.Ended()
			if len(ended) != 1 {
				t.Fatalf("expected one server span, got %d", len(ended))
			}
			want := ended[0].SpanContext()
			if sc := trace.SpanContextFromContext(got); sc.TraceID() != want.TraceID() || sc.SpanID() != want.SpanID() {
				t.Errorf("use case context carries span %v, want the request span %v", sc, want)
			}
		})
	}
}
