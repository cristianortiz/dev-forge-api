package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/auth/ports"
)

// -- mock repo ---------------------------------------------------------------

type mockUserRepo struct {
	getByZitadelID func(ctx context.Context, zitadelID string) (*domain.User, error)
	upsert         func(ctx context.Context, u *domain.User) error
	getByID        func(ctx context.Context, id uuid.UUID) (*domain.User, error)
	list           func(ctx context.Context) ([]*domain.User, error)
}

func (m *mockUserRepo) GetByZitadelID(ctx context.Context, zitadelID string) (*domain.User, error) {
	return m.getByZitadelID(ctx, zitadelID)
}
func (m *mockUserRepo) Upsert(ctx context.Context, u *domain.User) error {
	return m.upsert(ctx, u)
}
func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return m.getByID(ctx, id)
}
func (m *mockUserRepo) List(ctx context.Context) ([]*domain.User, error) {
	return m.list(ctx)
}

func newTestService(repo ports.UserRepository) *AuthService {
	return &AuthService{repo: repo, logger: zap.NewNop()}
}

// -- extractRoles ------------------------------------------------------------

func TestExtractRoles_NilClaims(t *testing.T) {
	if got := extractRoles(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractRoles_MissingClaim(t *testing.T) {
	claims := map[string]any{"sub": "user1"}
	if got := extractRoles(claims); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractRoles_Present(t *testing.T) {
	claims := map[string]any{
		"urn:zitadel:iam:org:project:roles": map[string]any{
			"admin": map[string]any{"org1": "org-name"},
		},
	}
	got := extractRoles(claims)
	if len(got) != 1 || got[0] != "admin" {
		t.Errorf("expected [admin], got %v", got)
	}
}

func TestExtractRoles_WrongType(t *testing.T) {
	claims := map[string]any{
		"urn:zitadel:iam:org:project:roles": "not-a-map",
	}
	if got := extractRoles(claims); got != nil {
		t.Errorf("expected nil for wrong type, got %v", got)
	}
}

func TestExtractRoles_MultipleRoles(t *testing.T) {
	claims := map[string]any{
		"urn:zitadel:iam:org:project:roles": map[string]any{
			"admin":     map[string]any{"org1": "org-name"},
			"developer": map[string]any{"org1": "org-name"},
		},
	}
	got := extractRoles(claims)
	if len(got) != 2 {
		t.Errorf("expected 2 roles, got %d: %v", len(got), got)
	}
}

// -- SyncUser ----------------------------------------------------------------

func TestSyncUser_ExistingUser(t *testing.T) {
	existing := &domain.User{ID: uuid.New(), ZitadelID: "z1", Role: domain.RoleAdmin}
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) { return existing, nil },
	}
	user, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{ZitadelID: "z1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("got user %v, want %v", user.ID, existing.ID)
	}
}

func TestSyncUser_NewUser_DefaultsDeveloper(t *testing.T) {
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		upsert: func(_ context.Context, _ *domain.User) error { return nil },
	}
	user, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{
		ZitadelID: "z2",
		Email:     "b@c.com",
		Roles:     []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Role != domain.RoleDeveloper {
		t.Errorf("expected RoleDeveloper, got %v", user.Role)
	}
}

func TestSyncUser_NewUser_AdminRole(t *testing.T) {
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		upsert: func(_ context.Context, _ *domain.User) error { return nil },
	}
	user, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{
		ZitadelID: "z3",
		Roles:     []string{"admin"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Role != domain.RoleAdmin {
		t.Errorf("expected RoleAdmin, got %v", user.Role)
	}
}

func TestSyncUser_NewUser_InvalidRole_DefaultsDeveloper(t *testing.T) {
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		upsert: func(_ context.Context, _ *domain.User) error { return nil },
	}
	user, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{
		ZitadelID: "z4",
		Roles:     []string{"superadmin", "poweruser"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Role != domain.RoleDeveloper {
		t.Errorf("expected RoleDeveloper for unknown roles, got %v", user.Role)
	}
}

func TestSyncUser_RepoLookupError(t *testing.T) {
	dbErr := errors.New("db connection lost")
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) { return nil, dbErr },
	}
	_, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{ZitadelID: "z5"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSyncUser_UpsertError(t *testing.T) {
	repo := &mockUserRepo{
		getByZitadelID: func(_ context.Context, _ string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
		upsert: func(_ context.Context, _ *domain.User) error { return errors.New("upsert failed") },
	}
	_, err := newTestService(repo).SyncUser(context.Background(), &ports.Claims{ZitadelID: "z6"})
	if err == nil {
		t.Fatal("expected upsert error, got nil")
	}
}

// -- GetUserByID -------------------------------------------------------------

func TestGetUserByID_InvalidUUID(t *testing.T) {
	_, err := newTestService(&mockUserRepo{}).GetUserByID(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestGetUserByID_ValidUUID(t *testing.T) {
	id := uuid.New()
	expected := &domain.User{ID: id}
	repo := &mockUserRepo{
		getByID: func(_ context.Context, got uuid.UUID) (*domain.User, error) {
			if got != id {
				return nil, errors.New("unexpected id")
			}
			return expected, nil
		},
	}
	user, err := newTestService(repo).GetUserByID(context.Background(), id.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != id {
		t.Errorf("got user %v, want %v", user.ID, id)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	repo := &mockUserRepo{
		getByID: func(_ context.Context, _ uuid.UUID) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		},
	}
	_, err := newTestService(repo).GetUserByID(context.Background(), uuid.New().String())
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}
