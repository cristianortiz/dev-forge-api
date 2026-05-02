package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
	"github.com/cristianortiz/dev-forge/internal/shared/logger"
	"github.com/cristianortiz/dev-forge/internal/shared/server"
	"go.uber.org/zap"
)

func main() {
	// ── config ────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// ── logger ────────────────────────────────────────────────────────────
	log, err := logger.New(cfg)
	if err != nil {
		panic("failed to build logger: " + err.Error())
	}
	defer log.Sync() //nolint:errcheck

	log.Info("dev-forge starting",
		zap.String("env", cfg.Environment),
		zap.String("version", cfg.OTEL.ServiceVersion),
	)

	// ── database ──────────────────────────────────────────────────────────
	ctx := context.Background()

	db, err := database.New(ctx, cfg, log)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// ── HTTP server ───────────────────────────────────────────────────────
	srv := server.New(cfg, log)
	server.RegisterHealthRoutes(srv.App)

	// TODO: register module routes here (Phase 1+)
	// auth.RegisterRoutes(srv.App, ...)

	// ── graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Listen(); err != nil {
			log.Error("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutdown signal received")

	if err := srv.Shutdown(); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}

	log.Info("dev-forge stopped")
}
