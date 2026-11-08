package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
	"github.com/cristianortiz/dev-forge/internal/shared/logger"
	"github.com/cristianortiz/dev-forge/internal/shared/server"
	"github.com/cristianortiz/dev-forge/internal/shared/telemetry"
)

func main() {
	os.Exit(run())
}

// run owns the process lifecycle and returns the exit code, so every deferred
// cleanup runs (log.Fatal would skip them). Shutdown order, last to first:
// stop accepting requests and drain in-flight ones, close the database, flush
// telemetry (last, so it captures everything above) and sync the logger.
func run() int {
	// ── config ────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		return 1
	}

	// ── logger ────────────────────────────────────────────────────────────
	log, err := logger.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to build logger:", err)
		return 1
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("dev-forge starting",
		zap.String("env", cfg.Environment),
		zap.String("version", cfg.OTEL.ServiceVersion),
	)

	// ── telemetry ─────────────────────────────────────────────────────────
	// Before the database and the HTTP server: both read the global providers
	// when they are created. An invalid OTEL_* configuration stops the start.
	shutdownTelemetry, err := telemetry.Setup(ctx, cfg, log)
	if err != nil {
		log.Error("failed to initialize telemetry", zap.Error(err))
		return 1
	}
	if cfg.OTEL.Enabled {
		// Keep Setup's own diagnostics on stdout to avoid exporting exporter errors.
		log = logger.WithOTEL(log)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), cfg.OTEL.ShutdownTimeout)
		defer cancel()
		if err := shutdownTelemetry(flushCtx); err != nil {
			log.Warn("telemetry shutdown incomplete", zap.Error(err))
		}
	}()

	if err := serve(ctx, cfg, log); err != nil {
		log.Error("dev-forge terminated with error", zap.Error(err))
		return 1
	}

	log.Info("dev-forge stopped")
	return 0
}

// serve connects the dependencies, runs the HTTP server and returns when ctx is
// cancelled (shutdown signal) or the server fails.
func serve(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	// ── database ──────────────────────────────────────────────────────────
	db, err := database.New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	// ── HTTP server ───────────────────────────────────────────────────────
	srv := server.New(cfg, log)
	server.RegisterHealthRoutes(srv.App)

	if err := registerRoutes(srv.App, db, cfg, log); err != nil {
		return fmt.Errorf("register routes: %w", err)
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Listen() }()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	// ── graceful shutdown ─────────────────────────────────────────────────
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}
	return nil
}
