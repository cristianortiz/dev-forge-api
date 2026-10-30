package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

const (
	blueprintSlug    = "go-modular-monolith"
	blueprintVersion = "v1.0.0"
)

func main() {
	if err := seed(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

func seed() error {
	sourceRepository := strings.TrimSpace(os.Getenv("BLUEPRINT_SOURCE_REPOSITORY"))
	sourceCommit := strings.TrimSpace(os.Getenv("BLUEPRINT_SOURCE_COMMIT"))
	if sourceRepository == "" || sourceCommit == "" {
		return errors.New("BLUEPRINT_SOURCE_REPOSITORY and BLUEPRINT_SOURCE_COMMIT must identify a real, validated GitHub snapshot")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	template := &domain.ProjectTemplate{
		ID:                 uuid.New(),
		Name:               "Go Modular Monolith",
		Slug:               blueprintSlug,
		Version:            blueprintVersion,
		Description:        "Curated Go API scaffold with a modular monolith architecture.",
		Language:           "go",
		Framework:          "fiber",
		SourceRepository:   strings.TrimRight(sourceRepository, "/"),
		SourceCommit:       strings.ToLower(sourceCommit),
		DefaultParams:      []byte(`{"PORT":"8080","ENV":"production","LOG_LEVEL":"info"}`),
		DefaultScopeConfig: []byte(`{"replicas":1,"cpu":"500m","memory":"512Mi","health_check_path":"/health"}`),
		IsActive:           true,
	}
	if err := template.ValidatePublicationMetadata(); err != nil {
		return fmt.Errorf("validate blueprint snapshot: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.Database.URL())
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	const insert = `
INSERT INTO project_templates
  (id, name, slug, version, description, language, framework,
   source_repository, source_commit, default_params, default_scope_config, is_active)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12)
ON CONFLICT (slug, version) DO NOTHING
RETURNING id`

	err = pool.QueryRow(ctx, insert,
		template.ID, template.Name, template.Slug, template.Version, template.Description,
		template.Language, template.Framework, template.SourceRepository, template.SourceCommit,
		template.DefaultParams, template.DefaultScopeConfig, template.IsActive,
	).Scan(&template.ID)
	if err == nil {
		fmt.Printf("published %s@%s from %s (%s)\n",
			template.Slug, template.Version, template.SourceRepository, template.SourceCommit)
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("insert blueprint version: %w", err)
	}

	var existingID uuid.UUID
	var existingRepository, existingCommit string
	const existing = `SELECT id, source_repository, source_commit
	                  FROM project_templates WHERE slug = $1 AND version = $2`
	if err := pool.QueryRow(ctx, existing, template.Slug, template.Version).
		Scan(&existingID, &existingRepository, &existingCommit); err != nil {
		return fmt.Errorf("read existing blueprint version: %w", err)
	}
	if existingRepository != template.SourceRepository || !strings.EqualFold(existingCommit, template.SourceCommit) {
		return fmt.Errorf("%s@%s already exists with a different source; publish a new version instead of mutating it",
			template.Slug, template.Version)
	}
	fmt.Printf("already present %s@%s (%s)\n", template.Slug, template.Version, existingID)
	return nil
}
