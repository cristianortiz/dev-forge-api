package ports

import (
	"context"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
)

// Claims holds the parsed fields from a Zitadel JWT.
type Claims struct {
	ZitadelID string
	Email     string
	Name      string
	Roles     []string // from urn:zitadel:iam:org:project:roles
}

// AuthService defines authentication and user-sync operations.
type AuthService interface {
	// ValidateToken parses and validates a raw JWT, returning its claims.
	ValidateToken(ctx context.Context, rawToken string) (*Claims, error)
	// SyncUser ensures the user from the claims exists in the DB.
	// Creates the user on first login; returns the existing user on subsequent calls.
	SyncUser(ctx context.Context, claims *Claims) (*domain.User, error)
	// GetMe validates the token and returns the corresponding user.
	GetMe(ctx context.Context, rawToken string) (*domain.User, error)
	// GetUserByID returns a user by their internal UUID (admin use).
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
}
