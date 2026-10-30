package openapi

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSpecDescribesAllCurrentOperations(t *testing.T) {
	var document struct {
		OpenAPI string                          `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(Spec, &document); err != nil {
		t.Fatalf("parse embedded OpenAPI contract: %v", err)
	}
	if document.OpenAPI != "3.2.0" {
		t.Fatalf("OpenAPI version = %q, want 3.2.0", document.OpenAPI)
	}

	expected := map[string]struct{}{
		"GET /auth/me":           {},
		"GET /templates":         {},
		"POST /templates":        {},
		"GET /templates/{id}":    {},
		"PATCH /templates/{id}":  {},
		"DELETE /templates/{id}": {},
		"GET /health":            {},
		"GET /ready":             {},
	}
	actual := make(map[string]struct{}, len(expected))
	for path, pathItem := range document.Paths {
		for method := range pathItem {
			switch method {
			case "get", "post", "put", "patch", "delete", "options", "head", "trace":
				actual[strings.ToUpper(method)+" "+path] = struct{}{}
			}
		}
	}

	if len(actual) != len(expected) {
		t.Fatalf("contract has %d operations, want %d: %v", len(actual), len(expected), actual)
	}
	for operation := range expected {
		if _, ok := actual[operation]; !ok {
			t.Errorf("contract is missing %s", operation)
		}
	}
}

func TestTemplateContractUsesImmutableVersions(t *testing.T) {
	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(Spec, &document); err != nil {
		t.Fatalf("parse embedded OpenAPI contract: %v", err)
	}

	create := document.Components.Schemas["CreateTemplateRequest"]
	for _, field := range []string{"version", "source_repository", "source_commit"} {
		if _, ok := create.Properties[field]; !ok {
			t.Errorf("CreateTemplateRequest is missing %q", field)
		}
	}
	if _, ok := create.Properties["dockerfile_template"]; ok {
		t.Error("CreateTemplateRequest must not accept Dockerfile contents")
	}

	update := document.Components.Schemas["UpdateTemplateRequest"]
	if len(update.Properties) != 1 {
		t.Fatalf("UpdateTemplateRequest has %d fields, want only is_active", len(update.Properties))
	}
	if _, ok := update.Properties["is_active"]; !ok {
		t.Error("UpdateTemplateRequest is missing is_active")
	}

	response := document.Components.Schemas["TemplateResponse"]
	for _, field := range []string{"version", "source_repository", "source_commit"} {
		if _, ok := response.Properties[field]; !ok {
			t.Errorf("TemplateResponse is missing %q", field)
		}
	}
}
