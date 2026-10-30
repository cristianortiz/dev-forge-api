package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
)

func TestCreateRequestToInput(t *testing.T) {
	req := createTemplateRequest{
		Name:               "Go Modular Monolith",
		Slug:               "go-modular-monolith",
		Version:            "v1.0.0",
		Description:        "Curated Go API scaffold",
		Language:           "go",
		Framework:          "fiber",
		SourceRepository:   "https://github.com/example/go-modular-monolith-v1",
		SourceCommit:       "0123456789abcdef0123456789abcdef01234567",
		DefaultParams:      json.RawMessage(`{"PORT":"8080"}`),
		DefaultScopeConfig: json.RawMessage(`{"replicas":1}`),
	}

	input := req.toInput()
	if input.Name != req.Name || input.Slug != req.Slug || input.Version != req.Version {
		t.Fatalf("identity fields were not mapped: %+v", input)
	}
	if input.SourceRepository != req.SourceRepository || input.SourceCommit != req.SourceCommit {
		t.Fatalf("source fields were not mapped: %+v", input)
	}
	if string(input.DefaultParams) != string(req.DefaultParams) ||
		string(input.DefaultScopeConfig) != string(req.DefaultScopeConfig) {
		t.Fatalf("JSON defaults were not mapped: %+v", input)
	}
}

func TestUpdateRequestToInputOnlyChangesVisibility(t *testing.T) {
	active := false
	input := (updateTemplateRequest{IsActive: &active}).toInput()
	if input.IsActive == nil || *input.IsActive {
		t.Fatalf("IsActive = %v, want pointer to false", input.IsActive)
	}
}

func TestToResponseIncludesImmutableSourceAndVersion(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	template := &domain.ProjectTemplate{
		ID:                 id,
		Name:               "Go Modular Monolith",
		Slug:               "go-modular-monolith",
		Version:            "v1.0.0",
		Description:        "Curated Go API scaffold",
		Language:           "go",
		Framework:          "fiber",
		SourceRepository:   "https://github.com/example/go-modular-monolith-v1",
		SourceCommit:       "0123456789abcdef0123456789abcdef01234567",
		DefaultParams:      []byte(`{"PORT":"8080"}`),
		DefaultScopeConfig: []byte(`{"replicas":1}`),
		IsActive:           true,
		CreatedAt:          createdAt,
	}

	response := toResponse(template)
	if response.ID != id.String() || response.Version != template.Version ||
		response.SourceRepository != template.SourceRepository || response.SourceCommit != template.SourceCommit {
		t.Fatalf("response does not preserve version identity: %+v", response)
	}
	if response.CreatedAt != "2026-01-15T10:30:00Z" {
		t.Fatalf("CreatedAt = %q", response.CreatedAt)
	}

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if string(result["default_params"]) != `{"PORT":"8080"}` {
		t.Fatalf("default_params = %s, want JSON object", result["default_params"])
	}
	if _, exists := result["dockerfile_template"]; exists {
		t.Fatal("response must not expose Dockerfile source")
	}
}
