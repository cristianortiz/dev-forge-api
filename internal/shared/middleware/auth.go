package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/auth/ports"
)

// Fiber context local keys.
const (
	LocalsUser   = "auth_user"
	LocalsClaims = "auth_claims"
)

// Authenticated validates the Bearer token and syncs the user to the DB.
// On success it stores *domain.User and *ports.Claims in Fiber locals.
func Authenticated(authSvc ports.AuthService, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rawToken := bearerToken(c)
		if rawToken == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing authorization header")
		}

		claims, err := authSvc.ValidateToken(c.Context(), rawToken)
		if err != nil {
			logger.Debug("token validation failed", zap.Error(err))
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}

		user, err := authSvc.SyncUser(c.Context(), claims)
		if err != nil {
			logger.Error("user sync failed", zap.Error(err))
			return fiber.NewError(fiber.StatusInternalServerError, "internal error")
		}

		c.Locals(LocalsUser, user)
		c.Locals(LocalsClaims, claims)
		return c.Next()
	}
}

// RequireRole returns a middleware that only allows users with one of the given roles.
// Must be placed after Authenticated.
func RequireRole(roles ...domain.Role) fiber.Handler {
	allowed := make(map[domain.Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		user := GetUser(c)
		if user == nil {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		if _, ok := allowed[user.Role]; !ok {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		return c.Next()
	}
}

// GetUser retrieves the authenticated user from the Fiber context.
// Returns nil if Authenticated middleware was not called.
func GetUser(c *fiber.Ctx) *domain.User {
	user, _ := c.Locals(LocalsUser).(*domain.User)
	return user
}

// GetClaims retrieves the JWT claims from the Fiber context.
func GetClaims(c *fiber.Ctx) *ports.Claims {
	claims, _ := c.Locals(LocalsClaims).(*ports.Claims)
	return claims
}

// bearerToken extracts the token from the Authorization header.
// Accepts "Bearer <token>" (standard) and bare "<token>" without spaces
// (Swagger UI apiKey sends the raw value without the Bearer prefix).
// Values containing spaces that don't start with "Bearer " are rejected
// so malformed schemes like "bearer abc123" or "Token abc123" are still blocked.
func bearerToken(c *fiber.Ctx) string {
	header := c.Get(fiber.HeaderAuthorization)
	if header == "" {
		return ""
	}
	if rest, ok := strings.CutPrefix(header, "Bearer "); ok {
		return rest
	}
	// Reject any value that contains a space but does not start with "Bearer ".
	if strings.Contains(header, " ") {
		return ""
	}
	return header
}
