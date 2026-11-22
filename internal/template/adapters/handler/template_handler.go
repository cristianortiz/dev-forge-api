package handler

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/shared/middleware"
	sharedserver "github.com/cristianortiz/dev-forge/internal/shared/server"
	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

const (
	errorCodeTemplateListFailed     = "template_list_failed"
	errorCodeTemplateNotFound       = "template_not_found"
	errorCodeTemplateReadFailed     = "template_read_failed"
	errorCodeInvalidTemplateID      = "invalid_template_id"
	errorCodeTemplateVersionExists  = "template_version_exists"
	errorCodeInvalidTemplate        = "invalid_template"
	errorCodeTemplateCreateFailed   = "template_create_failed"
	errorCodeTemplateUpdateFailed   = "template_update_failed"
	errorCodeTemplateDeactivateFail = "template_deactivate_failed"
)

// TemplateHandler handles HTTP requests for project templates.
type TemplateHandler struct {
	svc ports.TemplateService
}

// New creates a TemplateHandler.
func New(svc ports.TemplateService) *TemplateHandler {
	return &TemplateHandler{svc: svc}
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
		sharedserver.SetErrorDetails(c, errorCodeTemplateListFailed, err)
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
		sharedserver.SetErrorCode(c, errorCodeInvalidTemplateID)
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	t, err := h.svc.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			sharedserver.SetErrorCode(c, errorCodeTemplateNotFound)
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		sharedserver.SetErrorDetails(c, errorCodeTemplateReadFailed, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to get template")
	}
	return c.JSON(toResponse(t))
}

func (h *TemplateHandler) create(c fiber.Ctx) error {
	req := middleware.GetBody[createTemplateRequest](c)
	t, err := h.svc.Create(c.Context(), req.toInput())
	if err != nil {
		if errors.Is(err, domain.ErrTemplateVersionExists) {
			sharedserver.SetErrorCode(c, errorCodeTemplateVersionExists)
			return fiber.NewError(fiber.StatusConflict, "template version already exists")
		}
		if errors.Is(err, domain.ErrInvalidTemplate) {
			sharedserver.SetErrorCode(c, errorCodeInvalidTemplate)
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		sharedserver.SetErrorDetails(c, errorCodeTemplateCreateFailed, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create template")
	}
	return c.Status(fiber.StatusCreated).JSON(toResponse(t))
}

func (h *TemplateHandler) update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		sharedserver.SetErrorCode(c, errorCodeInvalidTemplateID)
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	req := middleware.GetBody[updateTemplateRequest](c)
	t, err := h.svc.Update(c.Context(), id, req.toInput())
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			sharedserver.SetErrorCode(c, errorCodeTemplateNotFound)
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		if errors.Is(err, domain.ErrInvalidTemplate) {
			sharedserver.SetErrorCode(c, errorCodeInvalidTemplate)
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		sharedserver.SetErrorDetails(c, errorCodeTemplateUpdateFailed, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update template")
	}
	return c.JSON(toResponse(t))
}

func (h *TemplateHandler) deactivate(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		sharedserver.SetErrorCode(c, errorCodeInvalidTemplateID)
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	if err := h.svc.Deactivate(c.Context(), id); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			sharedserver.SetErrorCode(c, errorCodeTemplateNotFound)
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		sharedserver.SetErrorDetails(c, errorCodeTemplateDeactivateFail, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to deactivate template")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// fiber:context-methods migrated
