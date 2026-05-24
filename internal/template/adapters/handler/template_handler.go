package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

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
//	PUT    /templates/:id    — update (admin only)
//	DELETE /templates/:id    — deactivate (admin only)
func (h *TemplateHandler) RegisterRoutes(router fiber.Router, authMW, adminMW fiber.Handler) {
	g := router.Group("/templates")

	g.Get("/", authMW, h.list)
	g.Get("/:id", authMW, h.getByID)

	g.Post("/", authMW, adminMW, middleware.ValidateBody[createTemplateRequest](), h.create)
	g.Put("/:id", authMW, adminMW, middleware.ValidateBody[updateTemplateRequest](), h.update)
	g.Delete("/:id", authMW, adminMW, h.deactivate)
}

// list godoc
// @Summary      List project templates
// @Tags         templates
// @Security     BearerAuth
// @Produce      json
// @Param        language   query  string  false  "Filter by language (go, javascript, python…)"
// @Param        framework  query  string  false  "Filter by framework (fiber, react, fastapi…)"
// @Success      200  {array}   templateResponse
// @Failure      500  {object}  map[string]string
// @Router       /templates [get]
func (h *TemplateHandler) list(c *fiber.Ctx) error {
	filter := ports.ListTemplatesFilter{
		Language:  c.Query("language"),
		Framework: c.Query("framework"),
	}
	templates, err := h.svc.List(c.Context(), filter)
	if err != nil {
		h.logger.Error("list templates", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list templates")
	}
	result := make([]templateResponse, 0, len(templates))
	for _, t := range templates {
		result = append(result, toResponse(t))
	}
	return c.JSON(result)
}

// getByID godoc
// @Summary      Get a project template by ID
// @Tags         templates
// @Security     BearerAuth
// @Produce      json
// @Param        id   path  string  true  "Template UUID"
// @Success      200  {object}  templateResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /templates/{id} [get]
func (h *TemplateHandler) getByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	t, err := h.svc.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		h.logger.Error("get template", zap.String("id", c.Params("id")), zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to get template")
	}
	return c.JSON(toResponse(t))
}

// create godoc
// @Summary      Create a project template (admin only)
// @Tags         templates
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  createTemplateRequest  true  "Template data"
// @Success      201  {object}  templateResponse
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Failure      422  {object}  map[string]string
// @Router       /templates [post]
func (h *TemplateHandler) create(c *fiber.Ctx) error {
	req := middleware.GetBody[createTemplateRequest](c)
	t, err := h.svc.Create(c.Context(), req.toInput())
	if err != nil {
		if errors.Is(err, domain.ErrSlugAlreadyExists) {
			return fiber.NewError(fiber.StatusConflict, "slug already exists")
		}
		h.logger.Error("create template", zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create template")
	}
	return c.Status(fiber.StatusCreated).JSON(toResponse(t))
}

// update godoc
// @Summary      Update a project template (admin only)
// @Tags         templates
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string                true  "Template UUID"
// @Param        body  body  updateTemplateRequest true  "Fields to update"
// @Success      200  {object}  templateResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      422  {object}  map[string]string
// @Router       /templates/{id} [put]
func (h *TemplateHandler) update(c *fiber.Ctx) error {
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
		h.logger.Error("update template", zap.String("id", c.Params("id")), zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update template")
	}
	return c.JSON(toResponse(t))
}

// deactivate godoc
// @Summary      Deactivate a project template (admin only)
// @Tags         templates
// @Security     BearerAuth
// @Param        id   path  string  true  "Template UUID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /templates/{id} [delete]
func (h *TemplateHandler) deactivate(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid template id")
	}
	if err := h.svc.Deactivate(c.Context(), id); err != nil {
		if errors.Is(err, domain.ErrTemplateNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		h.logger.Error("deactivate template", zap.String("id", c.Params("id")), zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "failed to deactivate template")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
