package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// TemplateService implements ports.TemplateService.
type TemplateService struct {
	repo   ports.TemplateRepository
	logger *zap.Logger
}

// New creates a TemplateService.
func New(repo ports.TemplateRepository, logger *zap.Logger) *TemplateService {
	return &TemplateService{repo: repo, logger: logger}
}

// List returns templates, applying optional language/framework filters.
func (s *TemplateService) List(ctx context.Context, filter ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
	templates, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("template: list: %w", err)
	}
	return templates, nil
}

// GetByID returns a single template; returns ErrTemplateNotFound if absent.
func (s *TemplateService) GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template: get by id: %w", err)
	}
	return t, nil
}

// Create validates and persists a new project template.
func (s *TemplateService) Create(ctx context.Context, input ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {

	// Apply safe defaults for JSONB fields.
	if len(input.DefaultParams) == 0 {
		input.DefaultParams = []byte("{}")
	}
	if len(input.DefaultScopeConfig) == 0 {
		input.DefaultScopeConfig = []byte("{}")
	}

	t := &domain.ProjectTemplate{
		ID:                 uuid.New(),
		Name:               input.Name,
		Slug:               input.Slug,
		Description:        input.Description,
		Language:           input.Language,
		Framework:          input.Framework,
		DockerfileTemplate: input.DockerfileTemplate,
		DefaultParams:      input.DefaultParams,
		DefaultScopeConfig: input.DefaultScopeConfig,
		RepoTemplateURL:    input.RepoTemplateURL,
		IsActive:           true,
		CreatedAt:          time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("template: create: %w", err)
	}

	s.logger.Info("template created",
		zap.String("id", t.ID.String()),
		zap.String("slug", t.Slug),
	)
	return t, nil
}

// Update applies a partial update to an existing template.
func (s *TemplateService) Update(ctx context.Context, id uuid.UUID, input ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template: update: %w", err)
	}

	if input.Name != "" {
		t.Name = input.Name
	}
	if input.Description != "" {
		t.Description = input.Description
	}
	if input.Language != "" {
		t.Language = input.Language
	}
	if input.Framework != "" {
		t.Framework = input.Framework
	}
	if input.DockerfileTemplate != "" {
		t.DockerfileTemplate = input.DockerfileTemplate
	}
	if len(input.DefaultParams) > 0 {
		t.DefaultParams = input.DefaultParams
	}
	if len(input.DefaultScopeConfig) > 0 {
		t.DefaultScopeConfig = input.DefaultScopeConfig
	}
	if input.RepoTemplateURL != "" {
		t.RepoTemplateURL = input.RepoTemplateURL
	}
	if input.IsActive != nil {
		t.IsActive = *input.IsActive
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, fmt.Errorf("template: update: %w", err)
	}

	s.logger.Info("template updated", zap.String("id", t.ID.String()))
	return t, nil
}

// Deactivate soft-deletes a template by setting is_active = false.
func (s *TemplateService) Deactivate(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Deactivate(ctx, id); err != nil {
		return fmt.Errorf("template: deactivate: %w", err)
	}
	s.logger.Info("template deactivated", zap.String("id", id.String()))
	return nil
}
