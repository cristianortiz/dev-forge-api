package ports

import (
	"context"

	"github.com/cristianortiz/dev-forge/internal/app/domain"
	"github.com/google/uuid"
)

type CreateAppInput struct {
	Name        string
	Description string
	OwnerID     uuid.UUID
	TemplateID  *uuid.UUID // opcional — aplica defaults del template
	RepoURL     string
	Language    string
	Framework   string
}

type UpdateAppInput struct {
	Name        *string
	Description *string
	RepoURL     *string
}

type ListFilter struct {
	OwnerID   *uuid.UUID
	Language  *string
	Framework *string
}

type ApplicationRepository interface {
	List(ctx context.Context, filter ListFilter) ([]domain.Application, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Application, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Application, error)
	Create(ctx context.Context, app *domain.Application) error
	Update(ctx context.Context, app *domain.Application) error
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type ApplicationService interface {
	List(ctx context.Context, filter ListFilter) ([]domain.Application, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Application, error)
	Create(ctx context.Context, input CreateAppInput) (*domain.Application, error)
	Update(ctx context.Context, id uuid.UUID, input UpdateAppInput) (*domain.Application, error)
	Deactivate(ctx context.Context, id uuid.UUID) error
}
