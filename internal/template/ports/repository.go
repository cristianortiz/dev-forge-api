package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

// ListTemplatesFilter defines optional filters for listing templates.
type ListTemplatesFilter struct {
	Language  string // filter by language ("go", "javascript", …); empty = no filter
	Framework string // filter by framework ("fiber", "react", …); empty = no filter
}

// TemplateRepository defines persistence operations for project templates.
type TemplateRepository interface {
	// List returns templates matching the given filters. Only active templates are returned by default.
	List(ctx context.Context, filter ListTemplatesFilter) ([]*domain.ProjectTemplate, error)
	// GetByID returns a template by UUID; returns ErrTemplateNotFound if absent.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error)
	// GetBySlug returns a template by its unique slug; returns ErrTemplateNotFound if absent.
	GetBySlug(ctx context.Context, slug string) (*domain.ProjectTemplate, error)
	// Create persists a new template.
	Create(ctx context.Context, t *domain.ProjectTemplate) error
	// Update persists changes to an existing template.
	Update(ctx context.Context, t *domain.ProjectTemplate) error
	// Deactivate sets is_active = false for the given template.
	Deactivate(ctx context.Context, id uuid.UUID) error
}
