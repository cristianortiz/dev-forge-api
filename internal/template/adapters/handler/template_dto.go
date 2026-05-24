package handler

import (
	"encoding/json"
	"time"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// ── Request types ─────────────────────────────────────────────────────────────

// createTemplateRequest is the JSON body for POST /templates.
type createTemplateRequest struct {
	Name               string          `json:"name"                validate:"required"`
	Slug               string          `json:"slug"                validate:"required"`
	Description        string          `json:"description"`
	Language           string          `json:"language"            validate:"required"`
	Framework          string          `json:"framework"`
	DockerfileTemplate string          `json:"dockerfile_template" validate:"required"`
	DefaultParams      json.RawMessage `json:"default_params"       swaggertype:"object"`
	DefaultScopeConfig json.RawMessage `json:"default_scope_config" swaggertype:"object"`
	RepoTemplateURL    string          `json:"repo_template_url"    validate:"omitempty,url"`
}

func (r createTemplateRequest) toInput() ports.CreateTemplateInput {
	return ports.CreateTemplateInput{
		Name:               r.Name,
		Slug:               r.Slug,
		Description:        r.Description,
		Language:           r.Language,
		Framework:          r.Framework,
		DockerfileTemplate: r.DockerfileTemplate,
		DefaultParams:      []byte(r.DefaultParams),
		DefaultScopeConfig: []byte(r.DefaultScopeConfig),
		RepoTemplateURL:    r.RepoTemplateURL,
	}
}

// updateTemplateRequest is the JSON body for PUT /templates/:id.
type updateTemplateRequest struct {
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Language           string          `json:"language"`
	Framework          string          `json:"framework"`
	DockerfileTemplate string          `json:"dockerfile_template"`
	DefaultParams      json.RawMessage `json:"default_params"       swaggertype:"object"`
	DefaultScopeConfig json.RawMessage `json:"default_scope_config" swaggertype:"object"`
	RepoTemplateURL    string          `json:"repo_template_url" validate:"omitempty,url"`
	IsActive           *bool           `json:"is_active"`
}

func (r updateTemplateRequest) toInput() ports.UpdateTemplateInput {
	return ports.UpdateTemplateInput{
		Name:               r.Name,
		Description:        r.Description,
		Language:           r.Language,
		Framework:          r.Framework,
		DockerfileTemplate: r.DockerfileTemplate,
		DefaultParams:      []byte(r.DefaultParams),
		DefaultScopeConfig: []byte(r.DefaultScopeConfig),
		RepoTemplateURL:    r.RepoTemplateURL,
		IsActive:           r.IsActive,
	}
}

// ── Response type ─────────────────────────────────────────────────────────────

// templateResponse is the JSON representation of a ProjectTemplate.
type templateResponse struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Description        string          `json:"description"`
	Language           string          `json:"language"`
	Framework          string          `json:"framework"`
	DockerfileTemplate string          `json:"dockerfile_template"`
	DefaultParams      json.RawMessage `json:"default_params"       swaggertype:"object"`
	DefaultScopeConfig json.RawMessage `json:"default_scope_config" swaggertype:"object"`
	RepoTemplateURL    string          `json:"repo_template_url"`
	IsActive           bool            `json:"is_active"`
	CreatedAt          string          `json:"created_at"`
}

func toResponse(t *domain.ProjectTemplate) templateResponse {
	return templateResponse{
		ID:                 t.ID.String(),
		Name:               t.Name,
		Slug:               t.Slug,
		Description:        t.Description,
		Language:           t.Language,
		Framework:          t.Framework,
		DockerfileTemplate: t.DockerfileTemplate,
		DefaultParams:      json.RawMessage(t.DefaultParams),
		DefaultScopeConfig: json.RawMessage(t.DefaultScopeConfig),
		RepoTemplateURL:    t.RepoTemplateURL,
		IsActive:           t.IsActive,
		CreatedAt:          t.CreatedAt.Format(time.RFC3339),
	}
}
