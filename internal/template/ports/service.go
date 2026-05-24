package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

// CreateTemplateInput holds the data required to create a new project template.
type CreateTemplateInput struct {
	Name               string
	Slug               string
	Description        string
	Language           string
	Framework          string
	DockerfileTemplate string
	DefaultParams      []byte // raw JSONB; defaults to "{}" if empty
	DefaultScopeConfig []byte // raw JSONB; defaults to "{}" if empty
	RepoTemplateURL    string
}

// UpdateTemplateInput holds the fields that may be updated on a template.
// Only non-zero / non-nil fields are applied (partial update semantics).
type UpdateTemplateInput struct {
	Name               string
	Description        string
	Language           string
	Framework          string
	DockerfileTemplate string
	DefaultParams      []byte // raw JSONB
	DefaultScopeConfig []byte // raw JSONB
	RepoTemplateURL    string
	IsActive           *bool // pointer to distinguish unset from explicit false
}

// TemplateService defines use cases for managing project templates.
type TemplateService interface {
	// List returns all active templates, optionally filtered by language or framework.
	List(ctx context.Context, filter ListTemplatesFilter) ([]*domain.ProjectTemplate, error)
	// GetByID returns a single template by its UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error)
	// Create creates a new project template. Enforcing admin-only access is the caller's responsibility.
	Create(ctx context.Context, input CreateTemplateInput) (*domain.ProjectTemplate, error)
	// Update applies a partial update to an existing template. Enforcing admin-only access is the caller's responsibility.
	Update(ctx context.Context, id uuid.UUID, input UpdateTemplateInput) (*domain.ProjectTemplate, error)
	// Deactivate soft-deletes a template (sets is_active = false). Enforcing admin-only access is the caller's responsibility.
	Deactivate(ctx context.Context, id uuid.UUID) error
}
