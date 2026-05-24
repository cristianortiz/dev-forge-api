package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/template/domain"
	"github.com/cristianortiz/dev-forge/internal/template/ports"
)

// -- mock repo ---------------------------------------------------------------

type mockTemplateRepo struct {
	list       func(ctx context.Context, filter ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error)
	getByID    func(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error)
	getBySlug  func(ctx context.Context, slug string) (*domain.ProjectTemplate, error)
	create     func(ctx context.Context, t *domain.ProjectTemplate) error
	update     func(ctx context.Context, t *domain.ProjectTemplate) error
	deactivate func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTemplateRepo) List(ctx context.Context, f ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
	return m.list(ctx, f)
}
func (m *mockTemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ProjectTemplate, error) {
	return m.getByID(ctx, id)
}
func (m *mockTemplateRepo) GetBySlug(ctx context.Context, slug string) (*domain.ProjectTemplate, error) {
	return m.getBySlug(ctx, slug)
}
func (m *mockTemplateRepo) Create(ctx context.Context, t *domain.ProjectTemplate) error {
	return m.create(ctx, t)
}
func (m *mockTemplateRepo) Update(ctx context.Context, t *domain.ProjectTemplate) error {
	return m.update(ctx, t)
}
func (m *mockTemplateRepo) Deactivate(ctx context.Context, id uuid.UUID) error {
	return m.deactivate(ctx, id)
}

func newTestSvc(repo ports.TemplateRepository) *TemplateService {
	return New(repo, zap.NewNop())
}

// validInput is a minimal valid CreateTemplateInput for reuse across tests.
var validInput = ports.CreateTemplateInput{
	Name:               "Go REST API",
	Slug:               "go-rest-api",
	Language:           "go",
	DockerfileTemplate: "FROM golang:1.26-alpine",
}

// -- Create ------------------------------------------------------------------

func TestCreate_Success(t *testing.T) {
	repo := &mockTemplateRepo{
		create: func(_ context.Context, _ *domain.ProjectTemplate) error { return nil },
	}
	tmpl, err := newTestSvc(repo).Create(context.Background(), validInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.ID == (uuid.UUID{}) {
		t.Error("expected non-zero ID")
	}
	if !tmpl.IsActive {
		t.Error("new template should be active")
	}
	if tmpl.Name != validInput.Name {
		t.Errorf("Name = %q, want %q", tmpl.Name, validInput.Name)
	}
}

func TestCreate_DefaultJSONBFields(t *testing.T) {
	repo := &mockTemplateRepo{
		create: func(_ context.Context, _ *domain.ProjectTemplate) error { return nil },
	}
	tmpl, err := newTestSvc(repo).Create(context.Background(), validInput)
	if err != nil {
		t.Fatal(err)
	}
	if string(tmpl.DefaultParams) != "{}" {
		t.Errorf("DefaultParams = %s, want {}", tmpl.DefaultParams)
	}
	if string(tmpl.DefaultScopeConfig) != "{}" {
		t.Errorf("DefaultScopeConfig = %s, want {}", tmpl.DefaultScopeConfig)
	}
}

func TestCreate_ProvidedJSONBNotOverwritten(t *testing.T) {
	input := validInput
	input.DefaultParams = []byte(`{"PORT":"8080"}`)
	input.DefaultScopeConfig = []byte(`{"replicas":2}`)

	repo := &mockTemplateRepo{
		create: func(_ context.Context, _ *domain.ProjectTemplate) error { return nil },
	}
	tmpl, err := newTestSvc(repo).Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if string(tmpl.DefaultParams) != `{"PORT":"8080"}` {
		t.Errorf("DefaultParams overwritten: %s", tmpl.DefaultParams)
	}
	if string(tmpl.DefaultScopeConfig) != `{"replicas":2}` {
		t.Errorf("DefaultScopeConfig overwritten: %s", tmpl.DefaultScopeConfig)
	}
}

func TestCreate_MissingName(t *testing.T) {
	input := validInput
	input.Name = ""
	_, err := newTestSvc(&mockTemplateRepo{}).Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCreate_MissingSlug(t *testing.T) {
	input := validInput
	input.Slug = ""
	_, err := newTestSvc(&mockTemplateRepo{}).Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
}

func TestCreate_MissingLanguage(t *testing.T) {
	input := validInput
	input.Language = ""
	_, err := newTestSvc(&mockTemplateRepo{}).Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing language")
	}
}

func TestCreate_MissingDockerfile(t *testing.T) {
	input := validInput
	input.DockerfileTemplate = ""
	_, err := newTestSvc(&mockTemplateRepo{}).Create(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing dockerfile_template")
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockTemplateRepo{
		create: func(_ context.Context, _ *domain.ProjectTemplate) error {
			return domain.ErrSlugAlreadyExists
		},
	}
	_, err := newTestSvc(repo).Create(context.Background(), validInput)
	if err == nil {
		t.Fatal("expected error from repo")
	}
	if !errors.Is(err, domain.ErrSlugAlreadyExists) {
		t.Errorf("expected ErrSlugAlreadyExists, got %v", err)
	}
}

// -- List --------------------------------------------------------------------

func TestList_DelegatesToRepo(t *testing.T) {
	want := []*domain.ProjectTemplate{
		{ID: uuid.New(), Name: "Go REST API", Language: "go"},
	}
	repo := &mockTemplateRepo{
		list: func(_ context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			return want, nil
		},
	}
	got, err := newTestSvc(repo).List(context.Background(), ports.ListTemplatesFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestList_RepoError(t *testing.T) {
	repo := &mockTemplateRepo{
		list: func(_ context.Context, _ ports.ListTemplatesFilter) ([]*domain.ProjectTemplate, error) {
			return nil, errors.New("db error")
		},
	}
	_, err := newTestSvc(repo).List(context.Background(), ports.ListTemplatesFilter{})
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

// -- GetByID -----------------------------------------------------------------

func TestGetByID_Found(t *testing.T) {
	id := uuid.New()
	want := &domain.ProjectTemplate{ID: id, Name: "React SPA", Language: "javascript"}
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, got uuid.UUID) (*domain.ProjectTemplate, error) {
			if got == id {
				return want, nil
			}
			return nil, domain.ErrTemplateNotFound
		},
	}
	tmpl, err := newTestSvc(repo).GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Name != want.Name {
		t.Errorf("got %v, want %v", tmpl.Name, want.Name)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
			return nil, domain.ErrTemplateNotFound
		},
	}
	_, err := newTestSvc(repo).GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}
}

// -- Update ------------------------------------------------------------------

func TestUpdate_PartialFields(t *testing.T) {
	existing := &domain.ProjectTemplate{
		ID:       uuid.New(),
		Name:     "Old Name",
		Language: "go",
		IsActive: true,
	}
	var saved *domain.ProjectTemplate
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) { return existing, nil },
		update:  func(_ context.Context, t *domain.ProjectTemplate) error { saved = t; return nil },
	}
	_, err := newTestSvc(repo).Update(context.Background(), existing.ID, ports.UpdateTemplateInput{
		Name: "New Name",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "New Name" {
		t.Errorf("Name not updated: %v", saved.Name)
	}
	if saved.Language != "go" {
		t.Errorf("Language should be unchanged: %v", saved.Language)
	}
}

func TestUpdate_IsActiveFalse(t *testing.T) {
	f := false
	existing := &domain.ProjectTemplate{ID: uuid.New(), IsActive: true}
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) { return existing, nil },
		update:  func(_ context.Context, _ *domain.ProjectTemplate) error { return nil },
	}
	updated, err := newTestSvc(repo).Update(context.Background(), existing.ID, ports.UpdateTemplateInput{
		IsActive: &f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.IsActive {
		t.Error("expected IsActive = false after update")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) {
			return nil, domain.ErrTemplateNotFound
		},
	}
	_, err := newTestSvc(repo).Update(context.Background(), uuid.New(), ports.UpdateTemplateInput{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestUpdate_RepoUpdateError(t *testing.T) {
	existing := &domain.ProjectTemplate{ID: uuid.New(), Name: "Old"}
	repo := &mockTemplateRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.ProjectTemplate, error) { return existing, nil },
		update:  func(_ context.Context, _ *domain.ProjectTemplate) error { return errors.New("update failed") },
	}
	_, err := newTestSvc(repo).Update(context.Background(), existing.ID, ports.UpdateTemplateInput{Name: "New"})
	if err == nil {
		t.Fatal("expected update error")
	}
}

// -- Deactivate --------------------------------------------------------------

func TestDeactivate_Success(t *testing.T) {
	id := uuid.New()
	called := false
	repo := &mockTemplateRepo{
		deactivate: func(_ context.Context, got uuid.UUID) error {
			if got == id {
				called = true
			}
			return nil
		},
	}
	if err := newTestSvc(repo).Deactivate(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("repo.Deactivate not called with correct id")
	}
}

func TestDeactivate_NotFound(t *testing.T) {
	repo := &mockTemplateRepo{
		deactivate: func(_ context.Context, _ uuid.UUID) error { return domain.ErrTemplateNotFound },
	}
	err := newTestSvc(repo).Deactivate(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrTemplateNotFound) {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}
}
