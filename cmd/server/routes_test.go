package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	httpswagger "github.com/swaggo/http-swagger"
)

func TestSwaggerUIIsServedThroughFiberAdaptor(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/docs/*", adaptor.HTTPHandler(httpswagger.WrapHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/index.html", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("serve Swagger UI: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("expected HTML content type, got %q", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read Swagger UI response: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected Swagger UI response body")
	}
}
