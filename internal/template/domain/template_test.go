package domain

import (
	"errors"
	"testing"
)

func validTemplate() *ProjectTemplate {
	return &ProjectTemplate{
		Name:             "Go Modular Monolith",
		Slug:             "go-modular-monolith",
		Version:          "v1.0.0",
		Language:         "go",
		Framework:        "fiber",
		SourceRepository: "https://github.com/example/go-modular-monolith-v1",
		SourceCommit:     "0123456789abcdef0123456789abcdef01234567",
		DefaultParams:    []byte(`{"PORT":"8080"}`),
	}
}

func TestValidatePublicationMetadata(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ProjectTemplate)
		wantErr bool
	}{
		{name: "valid release"},
		{name: "invalid slug", mutate: func(t *ProjectTemplate) { t.Slug = "Go API" }, wantErr: true},
		{name: "invalid version", mutate: func(t *ProjectTemplate) { t.Version = "1.0" }, wantErr: true},
		{name: "GitHub source required", mutate: func(t *ProjectTemplate) { t.SourceRepository = "https://example.com/repo" }, wantErr: true},
		{name: "GitHub source must not include a port", mutate: func(t *ProjectTemplate) {
			t.SourceRepository = "https://github.com:8443/example/repo"
		}, wantErr: true},
		{name: "one repository only", mutate: func(t *ProjectTemplate) { t.SourceRepository += "/subdirectory" }, wantErr: true},
		{name: "full commit required", mutate: func(t *ProjectTemplate) { t.SourceCommit = "abc123" }, wantErr: true},
		{name: "defaults must be object", mutate: func(t *ProjectTemplate) { t.DefaultParams = []byte(`[]`) }, wantErr: true},
		{name: "defaults must be valid JSON", mutate: func(t *ProjectTemplate) { t.DefaultParams = []byte(`{`) }, wantErr: true},
		{name: "secret keys forbidden", mutate: func(t *ProjectTemplate) {
			t.DefaultParams = []byte(`{"db":{"API_Key":"value"}}`)
		}, wantErr: true},
		{name: "empty defaults are allowed", mutate: func(t *ProjectTemplate) {
			t.DefaultParams = nil
			t.DefaultScopeConfig = nil
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := validTemplate()
			if tt.mutate != nil {
				tt.mutate(template)
			}
			err := template.ValidatePublicationMetadata()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePublicationMetadata() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidTemplate) {
				t.Fatalf("error = %v, want ErrInvalidTemplate", err)
			}
		})
	}
}

func TestSetActiveOnlyChangesVisibility(t *testing.T) {
	template := validTemplate()
	template.IsActive = true
	template.Name = "Published name"

	if err := template.SetActive(false); err != nil {
		t.Fatalf("SetActive(false): %v", err)
	}
	if template.IsActive {
		t.Fatal("template should be inactive")
	}
	if template.Name != "Published name" || template.SourceCommit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatal("published metadata changed while changing visibility")
	}

	if err := template.SetActive(true); err != nil {
		t.Fatalf("SetActive(true): %v", err)
	}
	if !template.IsActive {
		t.Fatal("template should be active")
	}
}

func TestSetActiveCannotActivateInvalidMetadata(t *testing.T) {
	template := &ProjectTemplate{Slug: "prototype", Version: "v1.0.0"}
	if err := template.SetActive(true); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("SetActive(true) error = %v, want ErrInvalidTemplate", err)
	}
}
