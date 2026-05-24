package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/auth/ports"
)

// -- mock AuthService --------------------------------------------------------

type mockAuthService struct {
	validateToken func(ctx context.Context, rawToken string) (*ports.Claims, error)
	syncUser      func(ctx context.Context, claims *ports.Claims) (*domain.User, error)
	getMe         func(ctx context.Context, rawToken string) (*domain.User, error)
	getUserByID   func(ctx context.Context, id string) (*domain.User, error)
}

func (m *mockAuthService) ValidateToken(ctx context.Context, rawToken string) (*ports.Claims, error) {
	return m.validateToken(ctx, rawToken)
}
func (m *mockAuthService) SyncUser(ctx context.Context, claims *ports.Claims) (*domain.User, error) {
	return m.syncUser(ctx, claims)
}
func (m *mockAuthService) GetMe(ctx context.Context, rawToken string) (*domain.User, error) {
	return m.getMe(ctx, rawToken)
}
func (m *mockAuthService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return m.getUserByID(ctx, id)
}

// -- Authenticated -----------------------------------------------------------

func TestAuthenticated_MissingToken(t *testing.T) {
	app := fiber.New()
	app.Get("/", Authenticated(&mockAuthService{}, zap.NewNop()), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthenticated_InvalidToken(t *testing.T) {
	svc := &mockAuthService{
		validateToken: func(_ context.Context, _ string) (*ports.Claims, error) {
			return nil, errors.New("token expired")
		},
	}
	app := fiber.New()
	app.Get("/", Authenticated(svc, zap.NewNop()), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthenticated_SyncUserError(t *testing.T) {
	svc := &mockAuthService{
		validateToken: func(_ context.Context, _ string) (*ports.Claims, error) {
			return &ports.Claims{ZitadelID: "z1"}, nil
		},
		syncUser: func(_ context.Context, _ *ports.Claims) (*domain.User, error) {
			return nil, errors.New("db error")
		},
	}
	app := fiber.New()
	app.Get("/", Authenticated(svc, zap.NewNop()), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestAuthenticated_Success(t *testing.T) {
	uid := uuid.New()
	claims := &ports.Claims{ZitadelID: "z1", Email: "a@b.com"}
	user := &domain.User{ID: uid, Role: domain.RoleAdmin}

	svc := &mockAuthService{
		validateToken: func(_ context.Context, _ string) (*ports.Claims, error) { return claims, nil },
		syncUser:      func(_ context.Context, _ *ports.Claims) (*domain.User, error) { return user, nil },
	}

	var (
		gotUser   *domain.User
		gotClaims *ports.Claims
	)
	app := fiber.New()
	app.Get("/", Authenticated(svc, zap.NewNop()), func(c *fiber.Ctx) error {
		gotUser = GetUser(c)
		gotClaims = GetClaims(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if gotUser == nil || gotUser.ID != uid {
		t.Errorf("user not set in context correctly: %v", gotUser)
	}
	if gotClaims == nil || gotClaims.ZitadelID != "z1" {
		t.Errorf("claims not set in context correctly: %v", gotClaims)
	}
}

// -- bearerToken -------------------------------------------------------------

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"lowercase bearer", "bearer abc123", ""}, // has space + wrong case → rejected
		{"token scheme", "Token abc123", ""},      // has space + wrong scheme → rejected
		{"empty header", "", ""},
		{"bearer only", "Bearer ", "Bearer"}, // HTTP strips trailing space → "Bearer" (no space → passed as bare token; auth service rejects it)
		{"bearer with spaces", "Bearer tok en", "tok en"},
		{"bare token", "myrawtoken123", "myrawtoken123"}, // Swagger UI apiKey format
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			var got string
			app.Get("/", func(c *fiber.Ctx) error {
				got = bearerToken(c)
				return c.SendStatus(fiber.StatusOK)
			})
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if _, err := app.Test(req); err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if got != tt.want {
				t.Errorf("bearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

// -- GetUser / GetClaims -----------------------------------------------------

func TestGetUser_SetInLocals(t *testing.T) {
	app := fiber.New()
	id := uuid.New()
	user := &domain.User{ID: id, Role: domain.RoleAdmin}
	var got *domain.User

	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals(LocalsUser, user)
		got = GetUser(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got == nil || got.ID != id {
		t.Errorf("GetUser() = %v, want user with id %v", got, id)
	}
}

func TestGetUser_NotSet_ReturnsNil(t *testing.T) {
	app := fiber.New()
	var got *domain.User

	app.Get("/", func(c *fiber.Ctx) error {
		got = GetUser(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got != nil {
		t.Errorf("GetUser() = %v, want nil", got)
	}
}

func TestGetClaims_SetInLocals(t *testing.T) {
	app := fiber.New()
	claims := &ports.Claims{ZitadelID: "z1", Email: "a@b.com"}
	var got *ports.Claims

	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals(LocalsClaims, claims)
		got = GetClaims(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got == nil || got.ZitadelID != "z1" {
		t.Errorf("GetClaims() = %v, want %v", got, claims)
	}
}

func TestGetClaims_NotSet_ReturnsNil(t *testing.T) {
	app := fiber.New()
	var got *ports.Claims

	app.Get("/", func(c *fiber.Ctx) error {
		got = GetClaims(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got != nil {
		t.Errorf("GetClaims() = %v, want nil", got)
	}
}

// -- RequireRole -------------------------------------------------------------

func TestRequireRole_AllowedRole(t *testing.T) {
	app := fiber.New()
	app.Get("/",
		func(c *fiber.Ctx) error {
			c.Locals(LocalsUser, &domain.User{Role: domain.RoleAdmin})
			return c.Next()
		},
		RequireRole(domain.RoleAdmin),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRequireRole_ForbiddenRole(t *testing.T) {
	app := fiber.New()
	app.Get("/",
		func(c *fiber.Ctx) error {
			c.Locals(LocalsUser, &domain.User{Role: domain.RoleViewer})
			return c.Next()
		},
		RequireRole(domain.RoleAdmin),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRequireRole_NoUserInContext(t *testing.T) {
	app := fiber.New()
	app.Get("/",
		RequireRole(domain.RoleAdmin),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRequireRole_MultipleRolesAllowed(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleAdmin, domain.RoleDeveloper} {
		t.Run(string(role), func(t *testing.T) {
			app := fiber.New()
			app.Get("/",
				func(c *fiber.Ctx) error {
					c.Locals(LocalsUser, &domain.User{Role: role})
					return c.Next()
				},
				RequireRole(domain.RoleAdmin, domain.RoleDeveloper),
				func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
			)
			req := httptest.NewRequest("GET", "/", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != fiber.StatusOK {
				t.Errorf("role %v: expected 200, got %d", role, resp.StatusCode)
			}
		})
	}
}

func TestRequireRole_ViewerExcludedFromMultiple(t *testing.T) {
	app := fiber.New()
	app.Get("/",
		func(c *fiber.Ctx) error {
			c.Locals(LocalsUser, &domain.User{Role: domain.RoleViewer})
			return c.Next()
		},
		RequireRole(domain.RoleAdmin, domain.RoleDeveloper),
		func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) },
	)
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
