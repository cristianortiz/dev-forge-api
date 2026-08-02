package usecase

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/app/domain"
	"github.com/cristianortiz/dev-forge/internal/app/ports"
	templateports "github.com/cristianortiz/dev-forge/internal/template/ports"
)

type appService struct {
	repo         ports.ApplicationRepository
	templateRepo templateports.TemplateRepository // para leer defaults
	log          *zap.Logger
}

func New(repo ports.ApplicationRepository, templateRepo templateports.TemplateRepository, log *zap.Logger) ports.ApplicationService {
	return &appService{repo: repo, templateRepo: templateRepo, log: log}
}

func (s *appService) List(ctx context.Context, filter ports.ListFilter) ([]domain.Application, error) {
	return s.repo.List(ctx, filter)
}

func (s *appService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Application, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *appService) Create(ctx context.Context, input ports.CreateAppInput) (*domain.Application, error) {
	if input.Name == "" {
		return nil, domain.ErrInvalidName
	}

	app := &domain.Application{
		ID:          uuid.New(),
		Name:        input.Name,
		Slug:        toSlug(input.Name),
		Description: input.Description,
		OwnerID:     input.OwnerID,
		TemplateID:  input.TemplateID,
		RepoURL:     input.RepoURL,
		Language:    input.Language,
		Framework:   input.Framework,
		IsActive:    true,
	}

	// Aplica defaults del template si se especificó
	if input.TemplateID != nil {
		tmpl, err := s.templateRepo.GetByID(ctx, *input.TemplateID)
		if err == nil {
			if app.Language == "" {
				app.Language = tmpl.Language
			}
			if app.Framework == "" {
				app.Framework = tmpl.Framework
			}
			app.DefaultParams = tmpl.DefaultParams
			app.DefaultScopeConfig = tmpl.DefaultScopeConfig
		}
	}

	// Verifica slug único
	existing, err := s.repo.GetBySlug(ctx, app.Slug)
	if err == nil && existing != nil {
		return nil, domain.ErrSlugTaken
	}

	if err := s.repo.Create(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *appService) Update(ctx context.Context, id uuid.UUID, input ports.UpdateAppInput) (*domain.Application, error) {
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		app.Name = *input.Name
	}
	if input.Description != nil {
		app.Description = *input.Description
	}
	if input.RepoURL != nil {
		app.RepoURL = *input.RepoURL
	}

	if err := s.repo.Update(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *appService) Deactivate(ctx context.Context, id uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.repo.Deactivate(ctx, id)
}

// toSlug convierte "My App Name" → "my-app-name"
func toSlug(name string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, name)
	// colapsa múltiples guiones
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
