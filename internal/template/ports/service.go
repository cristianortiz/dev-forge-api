package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

// CreateTemplateInput holds the metadata for publishing a new immutable version.
type CreateTemplateInput struct {
	Name               string
	Slug               string
	Version            string
	Description        string
	Language           string
	Framework          string
	DefaultParams      []byte // raw JSONB; defaults to "{}" if empty
	DefaultScopeConfig []byte // raw JSONB; defaults to "{}" if empty
	SourceRepository   string
	SourceCommit       string
}

// UpdateTemplateInput can only change catalog visibility of a published version.
type UpdateTemplateInput struct {
	IsActive *bool
}

// TemplateService defines use cases for managing project templates.
type TemplateService interface {
	// List returns all active templates, optionally filtered by language or framework.
	List(ctx context.Context, filter ListTemplatesFilter) ([]*domain.ProjectTemplate, error)
	// GetByID returns a single template by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error)
	// Create publishes a new immutable blueprint version. Enforcing admin-only access is the caller's responsibility.
	Create(ctx context.Context, input CreateTemplateInput) (*domain.ProjectTemplate, error)
	// Update changes only whether a published version is visible for new applications.
	Update(ctx context.Context, id uuid.UUID, input UpdateTemplateInput) (*domain.ProjectTemplate, error)
	// Deactivate soft-deletes a template (sets is_active = false). Enforcing admin-only access is the caller's responsibility.
	Deactivate(ctx context.Context, id uuid.UUID) error
}
