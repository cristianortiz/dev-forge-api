package server

import (
	"context"
	"fmt"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	fiberotel "github.com/gofiber/contrib/v3/otel"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	requestIDHeader = "X-Request-Id"
	// traceIDHeader returns the trace ID to clients so a support request can
	// quote it. It is the only trace information sent back.
	traceIDHeader = "X-Trace-Id"
)

// Server wraps a Fiber app with its dependencies.
type Server struct {
	App    *fiber.App
	cfg    *config.Config
	logger *zap.Logger
}

// New creates a Fiber HTTP server with base middleware configured.
//
// When telemetry is enabled the global tracer and meter providers must already
// be installed (see telemetry.Setup): the OTEL middleware reads them here.
func New(cfg *config.Config, logger *zap.Logger) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "dev-forge",
		ErrorHandler: defaultErrorHandler(logger),
	})

	// ── base middleware ────────────────────────────────────────────────────
	// Order matters:
	//   - the outer recover is a final guard for middleware panics.
	//   - accessLogger is outside OTEL so it sees the final status and trace ID.
	app.Use(requestid.New())
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.Debug}))
	app.Use(accessLogger(logger))
	if cfg.OTEL.Enabled {
		app.Use(otelMiddleware())
		// Preserve the span context for accessLogger, which runs outside OTEL.
		app.Use(captureTraceContext)
	}
	// Recover handler panics inside OTEL so its span and metrics record the 500.
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.Debug}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		// Browsers only expose these to scripts when listed here.
		ExposeHeaders: []string{requestIDHeader, traceIDHeader},
	}))

	return &Server{App: app, cfg: cfg, logger: logger}
}

// otelMiddleware traces every request and records the HTTP server metrics.
//
//   - Probes are skipped so they do not distort request rates or fill Tempo.
//   - The API faces clients we do not control, so an incoming trace context only
//     links to the new trace instead of continuing it; callers cannot force
//     sampling decisions or join our traces.
//   - Responses carry X-Trace-Id but no traceparent or baggage.
//   - The client address is not recorded on spans.
func otelMiddleware() fiber.Handler {
	return fiberotel.New(
		fiberotel.WithNext(isProbe),
		fiberotel.WithPublicEndpoint(),
		fiberotel.WithTraceResponseHeader(traceIDHeader),
		fiberotel.WithResponsePropagators(propagation.NewCompositeTextMapPropagator()),
		fiberotel.WithClientIP(false),
	)
}

func captureTraceContext(c fiber.Ctx) error {
	if spanContext := trace.SpanContextFromContext(c.Context()); spanContext.IsValid() {
		c.Locals(traceContextLocal, spanContext)
	}
	return c.Next()
}

func isProbe(c fiber.Ctx) bool {
	switch c.Path() {
	case "/health", "/ready":
		return true
	}
	return false
}

// Listen starts the HTTP server.
func (s *Server) Listen() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.logger.Info("starting HTTP server", zap.String("addr", addr))
	return s.App.Listen(addr)
}

// Shutdown stops accepting connections and waits for in-flight requests until
// ctx expires.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.App.ShutdownWithContext(ctx)
}
