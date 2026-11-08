package handler

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.uber.org/zap"

	sharedlogger "github.com/cristianortiz/dev-forge/internal/shared/logger"
	"github.com/cristianortiz/dev-forge/internal/shared/middleware"
	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// TemplateHandler handles HTTP requests for project templates.
type TemplateHandler struct {
	svc    ports.TemplateService
	logger *zap.Logger
}

// New creates a TemplateHandler.
func New(svc ports.TemplateService, logger *zap.Logger) *TemplateHandler {
	return &TemplateHandler{svc: svc, logger: logger}
}

// RegisterRoutes mounts template routes under the given router.
//
//	GET    /templates        — list (all authenticated roles)
//	GET    /templates/:id    — get by ID (all authenticated roles)
//	POST   /templates        — create (admin only)
//	PATCH  /templates/:id    — change catalog visibility (admin only)
//	DELETE /templates/:id    — deactivate (admin only)
func (h *TemplateHandler) RegisterRoutes(router fiber.Router, authMW, adminMW fiber.Handler) {
	g := router.Group("/templates")

	g.Get("/", authMW, h.list)
	g.Get("/:id", authMW, h.getByID)

	g.Post("/", authMW, adminMW, middleware.ValidateBody[createTemplateRequest](), h.create)
	g.Patch("/:id", authMW, adminMW, middleware.ValidateBody[updateTemplateRequest](), h.update)
	g.Delete("/:id", authMW, adminMW, h.deactivate)
}

func (h *TemplateHandler) list(c fiber.Ctx) error {
	filter := ports.ListTemplatesFilter{
		Language:  c.Query("language"),
		Framework: c.Query("framework"),
	}
	templates, err := h.svc.List(c.Context(), filter)
	if err != nil {
		sharedlogger.WithContext(h.logger, c.Context()).Error("list templates", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list templates")
	}
	result := make([]templateResponse, 0, len(templates))
	for _, t := range templates {
		result = append(result, toResponse(t))
	}
	return c.JSON(result)
}

func (h *TemplateHandler) getByID(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	t, err := h.svc.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		sharedlogger.WithContext(h.logger, c.Context()).Error("get template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to get template")
	}
	return c.JSON(toResponse(t))
}

func (h *TemplateHandler) create(c fiber.Ctx) error {
	req := middleware.GetBody[createTemplateRequest](c)
	t, err := h.svc.Create(c.Context(), req.toInput())
	if err != nil {
		if errors.Is(err, domain.ErrTemplateVersionExists) {
			return fiber.NewError(fiber.StatusConflict, "template version already exists")
		}
		if errors.Is(err, domain.ErrInvalidTemplate) {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		sharedlogger.WithContext(h.logger, c.Context()).Error("create template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create template")
	}
	return c.Status(fiber.StatusCreated).JSON(toResponse(t))
}

func (h *TemplateHandler) update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	req := middleware.GetBody[updateTemplateRequest](c)
	t, err := h.svc.Update(c.Context(), id, req.toInput())
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		if errors.Is(err, domain.ErrInvalidTemplate) {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		sharedlogger.WithContext(h.logger, c.Context()).Error("update template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update template")
	}
	return c.JSON(toResponse(t))
}

func (h *TemplateHandler) deactivate(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	if err := h.svc.Deactivate(c.Context(), id); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		sharedlogger.WithContext(h.logger, c.Context()).Error("deactivate template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to deactivate template")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// fiber:context-methods migrated
