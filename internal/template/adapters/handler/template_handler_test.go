package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// ── mock service ─────────────────────────────────────────────────────────────

type mockTemplateService struct {
	listFn       func(ctx context.Context, filter ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error)
	getByIDFn    func(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error)
	createFn     func(ctx context.Context, input ports.CreateTemplateInput) (*domain.ProjectTemplate, error)
	updateFn     func(ctx context.Context, id uuid.UUID, input ports.UpdateTemplateInput) (*domain.ProjectTemplate, error)
	deactivateFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTemplateService) List(ctx context.Context, f ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
	return m.listFn(ctx, f)
}
func (m *mockTemplateService) GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockTemplateService) Create(ctx context.Context, input ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
	return m.createFn(ctx, input)
}
func (m *mockTemplateService) Update(ctx context.Context, id uuid.UUID, input ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
	return m.updateFn(ctx, id, input)
}
func (m *mockTemplateService) Deactivate(ctx context.Context, id uuid.UUID) error {
	return m.deactivateFn(ctx, id)
}

// passThrough is a no-op middleware that allows all requests through.
var passThrough = func(c *fiber.Ctx) error { return c.Next() }

// newTestApp builds a Fiber app with the template handler registered.
// authMW and adminMW are replaced with passThrough so tests focus on handler logic.
func newTestApp(svc ports.TemplateService) *fiber.App {
	app := fiber.New(fiber.Config{
		// Return errors as JSON for easier assertion.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				code = fe.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})
	New(svc, zap.NewNop()).RegisterRoutes(app, passThrough, passThrough)
	return app
}

// sampleTemplate returns a fully-populated template for use in mock responses.
func sampleTemplate() *domain.ProjectTemplate {
	return &domain.ProjectTemplate{
		ID:                 uuid.New(),
		Name:               "Go REST API",
		Slug:               "go-rest-api",
		Description:        "A Go REST API scaffold",
		Language:           "go",
		Framework:          "fiber",
		DockerfileTemplate: "FROM golang:1.26-alpine",
		DefaultParams:      []byte(`{"PORT":"8080"}`),
		DefaultScopeConfig: []byte(`{"replicas":1}`),
		RepoTemplateURL:    "",
		IsActive:           true,
		CreatedAt:          time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
	}
}

// jsonBody returns a reader over the JSON-encoded payload.
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewBuffer(b)
}

// ── GET /templates ────────────────────────────────────────────────────────────

func TestList_ReturnsEmptyArray(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		listFn: func(_ context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			return nil, nil
		},
	})
	req := httptest.NewRequest("GET", "/templates", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var result []any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestList_ReturnsTemplates(t *testing.T) {
	tmpl := sampleTemplate()
	app := newTestApp(&mockTemplateService{
		listFn: func(_ context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			return []*domain.ProjectTemplate{tmpl}, nil
		},
	})
	req := httptest.NewRequest("GET", "/templates", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 template, got %d", len(result))
	}
	if result[0]["slug"] != tmpl.Slug {
		t.Errorf("slug = %q, want %q", result[0]["slug"], tmpl.Slug)
	}
}

func TestList_ForwardsQueryParams(t *testing.T) {
	var gotFilter ports.ListTemplatesFilter
	app := newTestApp(&mockTemplateService{
		listFn: func(_ context.Context, f ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			gotFilter = f
			return nil, nil
		},
	})
	req := httptest.NewRequest("GET", "/templates?language=go&framework=fiber", nil)
	app.Test(req) //nolint:errcheck

	if gotFilter.Language != "go" {
		t.Errorf("Language = %q, want %q", gotFilter.Language, "go")
	}
	if gotFilter.Framework != "fiber" {
		t.Errorf("Framework = %q, want %q", gotFilter.Framework, "fiber")
	}
}

func TestList_ServiceError_Returns500(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		listFn: func(_ context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			return nil, errors.New("db unavailable")
		},
	})
	req := httptest.NewRequest("GET", "/templates", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ── GET /templates/:id ────────────────────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	req := httptest.NewRequest("GET", "/templates/not-a-uuid", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
			return nil, domain.ErrTemplateNotFound
		},
	})
	req := httptest.NewRequest("GET", "/templates/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetByID_Success_Returns200(t *testing.T) {
	tmpl := sampleTemplate()
	app := newTestApp(&mockTemplateService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
			return tmpl, nil
		},
	})
	req := httptest.NewRequest("GET", "/templates/"+tmpl.ID.String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != tmpl.ID.String() {
		t.Errorf("id = %q, want %q", body["id"], tmpl.ID.String())
	}
	if body["slug"] != tmpl.Slug {
		t.Errorf("slug = %q, want %q", body["slug"], tmpl.Slug)
	}
}

func TestGetByID_ServiceError_Returns500(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
			return nil, errors.New("unexpected")
		},
	})
	req := httptest.NewRequest("GET", "/templates/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ── POST /templates ───────────────────────────────────────────────────────────

func TestCreate_MissingRequiredFields_Returns422(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	body := jsonBody(t, map[string]any{"description": "no required fields"})
	req := httptest.NewRequest("POST", "/templates", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	req := httptest.NewRequest("POST", "/templates", bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreate_SlugConflict_Returns409(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		createFn: func(_ context.Context, _ ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
			return nil, domain.ErrSlugAlreadyExists
		},
	})
	body := jsonBody(t, map[string]any{
		"name":                "Go REST API",
		"slug":                "go-rest-api",
		"language":            "go",
		"dockerfile_template": "FROM golang:1.26-alpine",
	})
	req := httptest.NewRequest("POST", "/templates", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
}

func TestCreate_Success_Returns201(t *testing.T) {
	tmpl := sampleTemplate()
	app := newTestApp(&mockTemplateService{
		createFn: func(_ context.Context, _ ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
			return tmpl, nil
		},
	})
	body := jsonBody(t, map[string]any{
		"name":                "Go REST API",
		"slug":                "go-rest-api",
		"language":            "go",
		"dockerfile_template": "FROM golang:1.26-alpine",
	})
	req := httptest.NewRequest("POST", "/templates", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["slug"] != tmpl.Slug {
		t.Errorf("slug = %q, want %q", result["slug"], tmpl.Slug)
	}
}

func TestCreate_ServiceError_Returns500(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		createFn: func(_ context.Context, _ ports.CreateTemplateInput) (*domain.ProjectTemplate, error) {
			return nil, errors.New("db error")
		},
	})
	body := jsonBody(t, map[string]any{
		"name":                "Go REST API",
		"slug":                "go-rest-api",
		"language":            "go",
		"dockerfile_template": "FROM golang:1.26-alpine",
	})
	req := httptest.NewRequest("POST", "/templates", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ── PUT /templates/:id ────────────────────────────────────────────────────────

func TestUpdate_InvalidUUID_Returns400(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	req := httptest.NewRequest("PUT", "/templates/not-a-uuid", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
			return nil, domain.ErrTemplateNotFound
		},
	})
	req := httptest.NewRequest("PUT", "/templates/"+uuid.New().String(), bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestUpdate_InvalidURL_Returns422(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	body := jsonBody(t, map[string]any{"repo_template_url": "not-a-url"})
	req := httptest.NewRequest("PUT", "/templates/"+uuid.New().String(), body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestUpdate_Success_Returns200(t *testing.T) {
	tmpl := sampleTemplate()
	app := newTestApp(&mockTemplateService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
			return tmpl, nil
		},
	})
	req := httptest.NewRequest("PUT", "/templates/"+tmpl.ID.String(), bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestUpdate_ServiceError_Returns500(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		updateFn: func(_ context.Context, _ uuid.UUID, _ ports.UpdateTemplateInput) (*domain.ProjectTemplate, error) {
			return nil, errors.New("db error")
		},
	})
	req := httptest.NewRequest("PUT", "/templates/"+uuid.New().String(), bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

// ── DELETE /templates/:id ─────────────────────────────────────────────────────

func TestDeactivate_InvalidUUID_Returns400(t *testing.T) {
	app := newTestApp(&mockTemplateService{})
	req := httptest.NewRequest("DELETE", "/templates/not-a-uuid", nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestDeactivate_NotFound_Returns404(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		deactivateFn: func(_ context.Context, _ uuid.UUID) error {
			return domain.ErrTemplateNotFound
		},
	})
	req := httptest.NewRequest("DELETE", "/templates/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeactivate_Success_Returns204(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		deactivateFn: func(_ context.Context, _ uuid.UUID) error { return nil },
	})
	req := httptest.NewRequest("DELETE", "/templates/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestDeactivate_ServiceError_Returns500(t *testing.T) {
	app := newTestApp(&mockTemplateService{
		deactivateFn: func(_ context.Context, _ uuid.UUID) error {
			return errors.New("db error")
		},
	})
	req := httptest.NewRequest("DELETE", "/templates/"+uuid.New().String(), nil)
	resp, _ := app.Test(req)

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}
