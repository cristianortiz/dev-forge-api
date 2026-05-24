package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByZitadelID(ctx context.Context, zitadelID string) (*domain.User, error)
	Upsert(ctx context.Context, user *domain.User) error
	List(ctx context.Context) ([]*domain.User, error)
}
