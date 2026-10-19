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
