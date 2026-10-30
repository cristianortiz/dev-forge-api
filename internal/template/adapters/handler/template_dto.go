package handler

import (
	"encoding/json"
	"time"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

type createTemplateRequest struct {
	Name               string          `json:"name" validate:"required"`
	Slug               string          `json:"slug" validate:"required"`
	Version            string          `json:"version" validate:"required"`
	Description        string          `json:"description"`
	Language           string          `json:"language" validate:"required"`
	Framework          string          `json:"framework" validate:"required"`
	SourceRepository   string          `json:"source_repository" validate:"required,url"`
	SourceCommit       string          `json:"source_commit" validate:"required,len=40"`
	DefaultParams      json.RawMessage `json:"default_params"`
	DefaultScopeConfig json.RawMessage `json:"default_scope_config"`
}

func (r createTemplateRequest) toInput() ports.CreateTemplateInput {
	return ports.CreateTemplateInput{
		Name:               r.Name,
		Slug:               r.Slug,
		Version:            r.Version,
		Description:        r.Description,
		Language:           r.Language,
		Framework:          r.Framework,
		SourceRepository:   r.SourceRepository,
		SourceCommit:       r.SourceCommit,
		DefaultParams:      []byte(r.DefaultParams),
		DefaultScopeConfig: []byte(r.DefaultScopeConfig),
	}
}

type updateTemplateRequest struct {
	IsActive *bool `json:"is_active" validate:"required"`
}

func (r updateTemplateRequest) toInput() ports.UpdateTemplateInput {
	return ports.UpdateTemplateInput{IsActive: r.IsActive}
}

type templateResponse struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Version            string          `json:"version"`
	Description        string          `json:"description"`
	Language           string          `json:"language"`
	Framework          string          `json:"framework"`
	SourceRepository   string          `json:"source_repository"`
	SourceCommit       string          `json:"source_commit"`
	DefaultParams      json.RawMessage `json:"default_params"`
	DefaultScopeConfig json.RawMessage `json:"default_scope_config"`
	IsActive           bool            `json:"is_active"`
	CreatedAt          string          `json:"created_at"`
}

func toResponse(t *domain.ProjectTemplate) templateResponse {
	return templateResponse{
		ID:                 t.ID.String(),
		Name:               t.Name,
		Slug:               t.Slug,
		Version:            t.Version,
		Description:        t.Description,
		Language:           t.Language,
		Framework:          t.Framework,
		SourceRepository:   t.SourceRepository,
		SourceCommit:       t.SourceCommit,
		DefaultParams:      json.RawMessage(t.DefaultParams),
		DefaultScopeConfig: json.RawMessage(t.DefaultScopeConfig),
		IsActive:           t.IsActive,
		CreatedAt:          t.CreatedAt.Format(time.RFC3339),
	}
}
