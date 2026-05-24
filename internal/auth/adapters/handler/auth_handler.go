package handler

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/auth/ports"
	"github.com/cristianortiz/dev-forge/internal/shared/middleware"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	authSvc ports.AuthService
	logger  *zap.Logger
}

// New creates an AuthHandler.
func New(authSvc ports.AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, logger: logger}
}

// RegisterRoutes mounts the auth routes under the given router.
// authMW is the Authenticated middleware from shared/middleware.
func (h *AuthHandler) RegisterRoutes(router fiber.Router, authMW fiber.Handler) {
	auth := router.Group("/auth")
	auth.Get("/me", authMW, h.getMe)
}

// getMe godoc
// @Summary      Get authenticated user profile
// @Description  Returns the profile of the currently authenticated user.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  userResponse
// @Failure      401  {object}  map[string]string
// @Router       /auth/me [get]
func (h *AuthHandler) getMe(c *fiber.Ctx) error {
	user := middleware.GetUser(c)
	return c.JSON(userResponse{
		ID:    user.ID.String(),
		Email: user.Email,
		Name:  user.Name,
		Role:  string(user.Role),
	})
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}
