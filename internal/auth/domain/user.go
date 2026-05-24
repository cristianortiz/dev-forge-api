package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Role represents the user's access level in the platform.
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
)

// IsValid reports whether r is a known role.
func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleDeveloper, RoleViewer:
		return true
	}
	return false
}

// Domain errors.
var (
	ErrUserNotFound = errors.New("user not found")
)

// User represents a dev-forge user, sourced from Zitadel via JWT.
// No password is stored; authentication is fully delegated to Zitadel.
type User struct {
	ID        uuid.UUID
	ZitadelID string // sub claim from Zitadel JWT
	Email     string
	Name      string
	Role      Role
	CreatedAt time.Time
}
