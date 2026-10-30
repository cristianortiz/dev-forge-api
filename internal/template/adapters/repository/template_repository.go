package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cristianortiz/dev-forge/internal/shared/database"
	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// uniqueViolation is the PostgreSQL error code for unique constraint violations.
const uniqueViolation = "23505"

// TemplateRepository implements ports.TemplateRepository using PostgreSQL via pgx.
type TemplateRepository struct {
	db *database.DB
}

// New creates a TemplateRepository.
func New(db *database.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

// List returns active templates, optionally filtered by language and/or framework.
func (r *TemplateRepository) List(ctx context.Context, filter ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
	q := `SELECT id, name, slug, version, description, language, framework,
	             source_repository, source_commit,
	             default_params, default_scope_config, is_active, created_at
	      FROM project_templates WHERE is_active = TRUE`

	args := []any{}
	idx := 1
	if filter.Language != "" {
		q += fmt.Sprintf(" AND language = $%d", idx)
		args = append(args, filter.Language)
		idx++
	}
	if filter.Framework != "" {
		q += fmt.Sprintf(" AND framework = $%d", idx)
		args = append(args, filter.Framework)
	}
	q += " ORDER BY name ASC"

	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []*domain.ProjectTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// GetByID returns the template with the given UUID; returns ErrTemplateNotFound if absent.
func (r *TemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error) {
	const q = `SELECT id, name, slug, version, description, language, framework,
	                  source_repository, source_commit,
	                  default_params, default_scope_config, is_active, created_at
	           FROM project_templates WHERE id = $1`
	t, err := scanTemplate(r.db.Pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTemplateNotFound
	}
	return t, err
}

// Create persists a new immutable version; returns ErrTemplateVersionExists on unique constraint violation.
func (r *TemplateRepository) Create(ctx context.Context, t *domain.ProjectTemplate) error {
	const q = `INSERT INTO project_templates
	           (id, name, slug, version, description, language, framework,
	            source_repository, source_commit, default_params, default_scope_config, is_active, created_at)
	           VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := r.db.Pool.Exec(ctx, q,
		t.ID, t.Name, t.Slug, t.Version, t.Description, t.Language, t.Framework,
		t.SourceRepository, t.SourceCommit, t.DefaultParams, t.DefaultScopeConfig, t.IsActive, t.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation &&
			pgErr.ConstraintName == "project_templates_slug_version_key" {
			return domain.ErrTemplateVersionExists
		}
		return fmt.Errorf("create template: %w", err)
	}
	return nil
}

// SetActive changes only the visibility of an immutable version.
func (r *TemplateRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	const q = `UPDATE project_templates SET is_active = $2 WHERE id = $1`
	tag, err := r.db.Pool.Exec(ctx, q, id, active)
	if err != nil {
		return fmt.Errorf("set template active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTemplateNotFound
	}
	return nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTemplate(row rowScanner) (*domain.ProjectTemplate, error) {
	t := &domain.ProjectTemplate{}
	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.Version, &t.Description, &t.Language, &t.Framework,
		&t.SourceRepository, &t.SourceCommit, &t.DefaultParams, &t.DefaultScopeConfig,
		&t.IsActive, &t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}
