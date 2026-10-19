package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cristianortiz/dev-forge/docs/openapi"
	"github.com/gofiber/fiber/v3"
)

func newDocsTestApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	if err := registerDocsRoutes(app.Group("/api/v1")); err != nil {
		t.Fatalf("register documentation routes: %v", err)
	}
	return app
}

func TestCanonicalOpenAPISpecIsServed(t *testing.T) {
	app := newDocsTestApp(t)

	for _, path := range []string{"/api/v1/openapi.yaml"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("serve OpenAPI spec: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
			}
			if got := resp.Header.Get(fiber.HeaderContentType); got != "application/yaml; charset=utf-8" {
				t.Fatalf("expected YAML content type, got %q", got)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read OpenAPI spec response: %v", err)
			}
			if !bytes.Equal(body, openapi.Spec) {
				t.Fatal("served OpenAPI spec differs from the embedded canonical file")
			}
		})
	}
}

func TestScalarUIDocsAndOfflineAssetsAreServed(t *testing.T) {
	app := newDocsTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("serve Scalar UI: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderContentType); got != "text/html; charset=utf-8" {
		resp.Body.Close()
		t.Fatalf("expected HTML content type, got %q", got)
	}
	html, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read Scalar UI response: %v", err)
	}
	if !bytes.Contains(html, []byte("Dev Forge API")) ||
		!bytes.Contains(html, []byte(`"openapi":"3.2.0"`)) ||
		!bytes.Contains(html, []byte("getCurrentUser")) {
		t.Fatal("Scalar UI does not embed the canonical OpenAPI contract")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/docs/js/api-reference.min.js", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("serve offline Scalar asset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderContentType); got != "application/javascript; charset=utf-8" {
		t.Fatalf("expected JavaScript content type, got %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read offline Scalar asset: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected an embedded Scalar JavaScript asset")
	}
}
