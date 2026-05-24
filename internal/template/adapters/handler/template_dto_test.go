package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

// ── createTemplateRequest.toInput() ──────────────────────────────────────────

func TestCreateRequest_ToInput_FieldMapping(t *testing.T) {
	req := createTemplateRequest{
		Name:               "Go REST API",
		Slug:               "go-rest-api",
		Description:        "A Go REST API template",
		Language:           "go",
		Framework:          "fiber",
		DockerfileTemplate: "FROM golang:1.26-alpine",
		DefaultParams:      json.RawMessage(`{"PORT":"8080"}`),
		DefaultScopeConfig: json.RawMessage(`{"replicas":2}`),
		RepoTemplateURL:    "https://github.com/example/go-rest-api",
	}

	input := req.toInput()

	if input.Name != req.Name {
		t.Errorf("Name = %q, want %q", input.Name, req.Name)
	}
	if input.Slug != req.Slug {
		t.Errorf("Slug = %q, want %q", input.Slug, req.Slug)
	}
	if input.Description != req.Description {
		t.Errorf("Description = %q, want %q", input.Description, req.Description)
	}
	if input.Language != req.Language {
		t.Errorf("Language = %q, want %q", input.Language, req.Language)
	}
	if input.Framework != req.Framework {
		t.Errorf("Framework = %q, want %q", input.Framework, req.Framework)
	}
	if input.DockerfileTemplate != req.DockerfileTemplate {
		t.Errorf("DockerfileTemplate = %q, want %q", input.DockerfileTemplate, req.DockerfileTemplate)
	}
	if input.RepoTemplateURL != req.RepoTemplateURL {
		t.Errorf("RepoTemplateURL = %q, want %q", input.RepoTemplateURL, req.RepoTemplateURL)
	}
}

func TestCreateRequest_ToInput_JSONBConversion(t *testing.T) {
	req := createTemplateRequest{
		DefaultParams:      json.RawMessage(`{"PORT":"8080"}`),
		DefaultScopeConfig: json.RawMessage(`{"replicas":2}`),
	}

	input := req.toInput()

	if string(input.DefaultParams) != `{"PORT":"8080"}` {
		t.Errorf("DefaultParams = %s, want {\"PORT\":\"8080\"}", input.DefaultParams)
	}
	if string(input.DefaultScopeConfig) != `{"replicas":2}` {
		t.Errorf("DefaultScopeConfig = %s, want {\"replicas\":2}", input.DefaultScopeConfig)
	}
}

func TestCreateRequest_ToInput_NilJSONB(t *testing.T) {
	req := createTemplateRequest{} // DefaultParams and DefaultScopeConfig unset (nil)

	input := req.toInput()

	if input.DefaultParams != nil {
		t.Errorf("expected nil DefaultParams, got %s", input.DefaultParams)
	}
	if input.DefaultScopeConfig != nil {
		t.Errorf("expected nil DefaultScopeConfig, got %s", input.DefaultScopeConfig)
	}
}

// ── updateTemplateRequest.toInput() ──────────────────────────────────────────

func TestUpdateRequest_ToInput_FieldMapping(t *testing.T) {
	active := true
	req := updateTemplateRequest{
		Name:               "Updated Name",
		Description:        "Updated description",
		Language:           "go",
		Framework:          "fiber",
		DockerfileTemplate: "FROM golang:1.26",
		DefaultParams:      json.RawMessage(`{"PORT":"9090"}`),
		DefaultScopeConfig: json.RawMessage(`{"replicas":3}`),
		RepoTemplateURL:    "https://github.com/example/updated",
		IsActive:           &active,
	}

	input := req.toInput()

	if input.Name != req.Name {
		t.Errorf("Name = %q, want %q", input.Name, req.Name)
	}
	if input.Description != req.Description {
		t.Errorf("Description = %q, want %q", input.Description, req.Description)
	}
	if input.IsActive == nil || *input.IsActive != true {
		t.Error("IsActive should be true")
	}
	if string(input.DefaultParams) != `{"PORT":"9090"}` {
		t.Errorf("DefaultParams = %s", input.DefaultParams)
	}
}

func TestUpdateRequest_ToInput_NilIsActive(t *testing.T) {
	req := updateTemplateRequest{} // IsActive not set

	input := req.toInput()

	if input.IsActive != nil {
		t.Errorf("expected nil IsActive, got %v", *input.IsActive)
	}
}

func TestUpdateRequest_ToInput_IsActiveFalse(t *testing.T) {
	active := false
	req := updateTemplateRequest{IsActive: &active}

	input := req.toInput()

	if input.IsActive == nil {
		t.Fatal("IsActive should not be nil")
	}
	if *input.IsActive != false {
		t.Error("IsActive should be false")
	}
}

// ── toResponse() ─────────────────────────────────────────────────────────────

func TestToResponse_FieldMapping(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	tmpl := &domain.ProjectTemplate{
		ID:                 id,
		Name:               "Go REST API",
		Slug:               "go-rest-api",
		Description:        "A Go REST API template",
		Language:           "go",
		Framework:          "fiber",
		DockerfileTemplate: "FROM golang:1.26-alpine",
		DefaultParams:      []byte(`{"PORT":"8080"}`),
		DefaultScopeConfig: []byte(`{"replicas":1}`),
		RepoTemplateURL:    "https://github.com/example/go-rest-api",
		IsActive:           true,
		CreatedAt:          createdAt,
	}

	resp := toResponse(tmpl)

	if resp.ID != id.String() {
		t.Errorf("ID = %q, want %q", resp.ID, id.String())
	}
	if resp.Name != tmpl.Name {
		t.Errorf("Name = %q, want %q", resp.Name, tmpl.Name)
	}
	if resp.Slug != tmpl.Slug {
		t.Errorf("Slug = %q, want %q", resp.Slug, tmpl.Slug)
	}
	if resp.Language != tmpl.Language {
		t.Errorf("Language = %q, want %q", resp.Language, tmpl.Language)
	}
	if resp.Framework != tmpl.Framework {
		t.Errorf("Framework = %q, want %q", resp.Framework, tmpl.Framework)
	}
	if resp.IsActive != tmpl.IsActive {
		t.Errorf("IsActive = %v, want %v", resp.IsActive, tmpl.IsActive)
	}
}

func TestToResponse_JSONBRenderedAsRawJSON(t *testing.T) {
	tmpl := &domain.ProjectTemplate{
		ID:                 uuid.New(),
		DefaultParams:      []byte(`{"PORT":"8080"}`),
		DefaultScopeConfig: []byte(`{"replicas":1}`),
		CreatedAt:          time.Now(),
	}

	resp := toResponse(tmpl)

	if string(resp.DefaultParams) != `{"PORT":"8080"}` {
		t.Errorf("DefaultParams = %s", resp.DefaultParams)
	}
	if string(resp.DefaultScopeConfig) != `{"replicas":1}` {
		t.Errorf("DefaultScopeConfig = %s", resp.DefaultScopeConfig)
	}
}

func TestToResponse_CreatedAtRFC3339(t *testing.T) {
	createdAt := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	tmpl := &domain.ProjectTemplate{
		ID:        uuid.New(),
		CreatedAt: createdAt,
	}

	resp := toResponse(tmpl)

	want := "2026-05-23T12:00:00Z"
	if resp.CreatedAt != want {
		t.Errorf("CreatedAt = %q, want %q", resp.CreatedAt, want)
	}
}

func TestToResponse_JSONBMarshalledAsObject(t *testing.T) {
	tmpl := &domain.ProjectTemplate{
		ID:            uuid.New(),
		DefaultParams: []byte(`{"PORT":"8080"}`),
		CreatedAt:     time.Now(),
	}

	resp := toResponse(tmpl)

	// Simulate JSON serialization — JSONB fields should appear as objects, not base64.
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if string(out["default_params"]) != `{"PORT":"8080"}` {
		t.Errorf("default_params in JSON = %s, want {\"PORT\":\"8080\"}", out["default_params"])
	}
}
