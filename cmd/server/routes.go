package main

import (
	"context"

	"github.com/gofiber/fiber/v2"
	swagger "github.com/swaggo/fiber-swagger"
	"go.uber.org/zap"

	_ "github.com/cristianortiz/dev-forge/docs/swagger" // generated swagger docs
	authhandler "github.com/cristianortiz/dev-forge/internal/auth/adapters/handler"
	authrepo "github.com/cristianortiz/dev-forge/internal/auth/adapters/repository"
	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	authsvc "github.com/cristianortiz/dev-forge/internal/auth/service"
	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
	"github.com/cristianortiz/dev-forge/internal/shared/middleware"
	templatehandler "github.com/cristianortiz/dev-forge/internal/template/adapters/handler"
	templaterepo "github.com/cristianortiz/dev-forge/internal/template/adapters/repository"
	templatesvc "github.com/cristianortiz/dev-forge/internal/template/service"
)

// convenience aliases used when building role-specific middleware.
const (
	roleAdmin     = domain.RoleAdmin
	roleDeveloper = domain.RoleDeveloper
	roleViewer    = domain.RoleViewer
)

// registerRoutes is the single composition root of the entire API.
// It owns the dependency graph: for each module it instantiates the repository
// (infrastructure layer), injects it into the service (use-case layer), injects
// the service into the HTTP handler (adapter layer), and calls RegisterRoutes so
// the handler mounts its own endpoints on the shared router group.
//
// Pattern for every module:
//
//	repo    := <module>repo.New(db)               // adapters/repository — talks to PostgreSQL
//	svc     := <module>svc.New(repo, log)          // service            — business logic
//	handler := <module>handler.New(svc, log)        // adapters/handler   — HTTP translation
//	handler.RegisterRoutes(api, authMW [, adminMW]) // mount under /api/v1
//
// Shared middleware (authMW, adminMW) is created once here and passed down;
// each handler decides which of its routes require which middleware.
func registerRoutes(app *fiber.App, db *database.DB, cfg *config.Config, log *zap.Logger) error {
	api := app.Group("/api/v1")

	// ── Swagger UI ────────────────────────────────────────────────────────
	// Available at /api/v1/docs/index.html — no auth required.
	// Re-generate with: make swagger
	api.Get("/docs/*", swagger.WrapHandler)

	// ── Shared middleware ─────────────────────────────────────────────────
	// authService is used both as a dependency for the auth module and as the
	// token-validation backend for the Authenticated middleware.
	authService, err := authsvc.New(context.Background(), authrepo.New(db), cfg.Zitadel.Issuer, cfg.Zitadel.KeyPath, log)
	if err != nil {
		return err
	}
	// authMW: validates the Bearer token via Zitadel and injects the User into c.Locals.
	// Applied to every protected route.
	authMW := middleware.Authenticated(authService, log)
	// adminMW: RBAC gate — rejects requests whose User role != admin.
	// Applied only to write/admin endpoints.
	adminMW := middleware.RequireRole(roleAdmin)

	// ── Phase 1: Auth ─────────────────────────────────────────────────────
	// Endpoints: POST /auth/sync, GET /auth/me, GET /auth/users/:id
	authhandler.New(authService, log).RegisterRoutes(api, authMW)

	// ── Phase 1: Templates ────────────────────────────────────────────────
	// Endpoints: GET /templates, GET /templates/:id (all roles)
	//            POST /templates, PUT /templates/:id, DELETE /templates/:id (admin only)
	templatehandler.New(
		templatesvc.New(templaterepo.New(db), log),
		log,
	).RegisterRoutes(api, authMW, adminMW)

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

	return nil
}
