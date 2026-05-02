package server

import (
	"fmt"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"go.uber.org/zap"
)

// Server wraps a Fiber app with its dependencies.
type Server struct {
	App    *fiber.App
	cfg    *config.Config
	logger *zap.Logger
}

// New creates a Fiber HTTP server with base middleware configured.
func New(cfg *config.Config, logger *zap.Logger) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "dev-forge",
		ErrorHandler: defaultErrorHandler(logger),
	})

	// ── base middleware ────────────────────────────────────────────────────
	app.Use(requestid.New())
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.Debug}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}))
	app.Use(zapLogger(logger))

	return &Server{App: app, cfg: cfg, logger: logger}
}

// Listen starts the HTTP server.
func (s *Server) Listen() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.logger.Info("starting HTTP server", zap.String("addr", addr))
	return s.App.Listen(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	s.logger.Info("shutting down HTTP server")
	return s.App.Shutdown()
}
