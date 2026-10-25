package usecase

import (
	"context"
	"fmt"
	"strings"
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

// List returns active published versions, applying optional language/framework filters.
func (s *TemplateService) List(ctx context.Context, filter ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
	templates, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("template: list: %w", err)
	}
	return templates, nil
}

// GetByID returns a single immutable version by UUID.
func (s *TemplateService) GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error) {
	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template: get by id: %w", err)
	}
	return t, nil
}

// Create publishes a new immutable version after validating its source and defaults.
func (s *TemplateService) Create(ctx context.Context, input ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
	defaultParams := input.DefaultParams
	if len(defaultParams) == 0 {
		defaultParams = []byte("{}")
	}
	defaultScopeConfig := input.DefaultScopeConfig
	if len(defaultScopeConfig) == 0 {
		defaultScopeConfig = []byte("{}")
	}

	t := &domain.ProjectTemplate{
		ID:                 uuid.New(),
		Name:               strings.TrimSpace(input.Name),
		Slug:               input.Slug,
		Version:            input.Version,
		Description:        strings.TrimSpace(input.Description),
		Language:           strings.TrimSpace(input.Language),
		Framework:          strings.TrimSpace(input.Framework),
		SourceRepository:   strings.TrimRight(strings.TrimSpace(input.SourceRepository), "/"),
		SourceCommit:       strings.ToLower(strings.TrimSpace(input.SourceCommit)),
		DefaultParams:      defaultParams,
		DefaultScopeConfig: defaultScopeConfig,
		IsActive:           true,
		CreatedAt:          time.Now().UTC(),
	}
	if err := t.ValidatePublicationMetadata(); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("template: publish: %w", err)
	}

	s.logger.Info("template version published",
		zap.String("id", t.ID.String()),
		zap.String("slug", t.Slug),
		zap.String("version", t.Version),
		zap.String("source_repository", t.SourceRepository),
		zap.String("source_commit", t.SourceCommit),
	)
	return t, nil
}

// Update changes only whether a published version is available for new applications.
func (s *TemplateService) Update(ctx context.Context, id uuid.UUID, input ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
	if input.IsActive == nil {
		return nil, fmt.Errorf("%w: is_active is required; published metadata cannot be edited", domain.ErrInvalidTemplate)
	}

	t, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template: update visibility: %w", err)
	}
	if err := t.SetActive(*input.IsActive); err != nil {
		return nil, err
	}
	if err := s.repo.SetActive(ctx, id, t.IsActive); err != nil {
		return nil, fmt.Errorf("template: update visibility: %w", err)
	}

	s.logger.Info("template version visibility changed",
		zap.String("id", t.ID.String()),
		zap.Bool("is_active", t.IsActive),
	)
	return t, nil
}

// Deactivate hides a published version from future application creation.
func (s *TemplateService) Deactivate(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.SetActive(ctx, id, false); err != nil {
		return fmt.Errorf("template: deactivate: %w", err)
	}
	s.logger.Info("template version deactivated", zap.String("id", id.String()))
	return nil
}
