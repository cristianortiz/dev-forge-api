package main

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	authhandler "github.com/cristianortiz/dev-forge/internal/auth/adapters/handler"
	authrepo "github.com/cristianortiz/dev-forge/internal/auth/adapters/repository"
	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	authsvc "github.com/cristianortiz/dev-forge/internal/auth/service"
	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
	"github.com/cristianortiz/dev-forge/internal/shared/middleware"
)

// convenience aliases used when building role-specific middleware.
const (
	roleAdmin     = domain.RoleAdmin
	roleDeveloper = domain.RoleDeveloper
	roleViewer    = domain.RoleViewer
)

// registerRoutes wires all modules and mounts their routes under /api/v1.
// This is the single composition root for the entire API surface.
// Add each new module here as phases progress.
func registerRoutes(app *fiber.App, db *database.DB, cfg *config.Config, log *zap.Logger) error {
	api := app.Group("/api/v1")

	// ── auth middleware (shared across all protected routes) ──────────────
	authService, err := authsvc.New(context.Background(), authrepo.New(db), cfg.Zitadel.Issuer, cfg.Zitadel.KeyPath, log)
	if err != nil {
		return err
	}
	authMW := middleware.Authenticated(authService, log)
	adminMW := middleware.RequireRole(roleAdmin)

	// ── Phase 1: Auth ─────────────────────────────────────────────────────
	authhandler.New(authService, log).RegisterRoutes(api, authMW)

	// ── Phase 1: Templates (TODO) ─────────────────────────────────────────
	// templatehandler.New(...).RegisterRoutes(api, authMW, adminMW)

	// ── Phase 1: Applications (TODO) ──────────────────────────────────────
	// apphandler.New(...).RegisterRoutes(api, authMW)

	// ── Phase 1: Git (TODO) ───────────────────────────────────────────────
	// githandler.New(...).RegisterRoutes(api, authMW)

	// ── Phase 1: Clusters — admin only (TODO) ─────────────────────────────
	// clusterhandler.New(...).RegisterRoutes(api, authMW, adminMW)

	// ── Phase 2: Builds / Releases / Scopes / Deploy (TODO) ──────────────
	// ── Phase 3: Parameters / Services (TODO) ────────────────────────────
	// ── Phase 4: Observability (TODO) ────────────────────────────────────
	// ── Phase 5: Approvals / Audit / Notifications (TODO) ────────────────

	_ = adminMW // remove once first admin route is added
	return nil
}
