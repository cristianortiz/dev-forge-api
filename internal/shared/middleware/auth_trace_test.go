package middleware

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

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/auth/ports"
)

// Token validation and user sync both call out (Zitadel, PostgreSQL); they must
// run under the request's span so those calls appear inside the request trace.
func TestAuthenticated_PassesTheRequestSpanToTheAuthService(t *testing.T) {
	var validateCtx, syncCtx context.Context
	svc := &mockAuthService{
		validateToken: func(ctx context.Context, _ string) (*ports.Claims, error) {
			validateCtx = ctx
			return &ports.Claims{ZitadelID: "z1"}, nil
		},
		syncUser: func(ctx context.Context, _ *ports.Claims) (*domain.User, error) {
			syncCtx = ctx
			return &domain.User{ID: uuid.New(), Role: domain.RoleViewer}, nil
		},
	}

	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	app := fiber.New()
	app.Use(fiberotel.New(fiberotel.WithTracerProvider(tp)))
	app.Get("/", Authenticated(svc, zap.NewNop()), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected one server span, got %d", len(ended))
	}
	want := ended[0].SpanContext()
	for name, ctx := range map[string]context.Context{"ValidateToken": validateCtx, "SyncUser": syncCtx} {
		if ctx == nil {
			t.Fatalf("%s was not called", name)
		}
		if sc := trace.SpanContextFromContext(ctx); sc.TraceID() != want.TraceID() || sc.SpanID() != want.SpanID() {
			t.Errorf("%s context carries span %v, want the request span %v", name, sc, want)
		}
	}
}
