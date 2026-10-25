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
	// Create persists a new template.
	Create(ctx context.Context, t *domain.ProjectTemplate) error
	// SetActive changes only catalog visibility.
	SetActive(ctx context.Context, id uuid.UUID, active bool) error
}
