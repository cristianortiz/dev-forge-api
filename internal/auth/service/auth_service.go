package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rs"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/auth/ports"
)

// AuthService implements ports.AuthService using OIDC token introspection.
// Introspection works with both opaque tokens (PATs) and JWTs.
type AuthService struct {
	repo           ports.UserRepository
	resourceServer rs.ResourceServer
	logger         *zap.Logger
}

// New creates an AuthService using JWT Profile (private key) for token introspection.
// keyPath is the path to the JSON key file downloaded from the Zitadel console for the API app.
func New(ctx context.Context, repo ports.UserRepository, issuer, keyPath string, logger *zap.Logger) (*AuthService, error) {
	resourceServer, err := rs.NewResourceServerFromKeyFile(ctx, issuer, keyPath)
	if err != nil {
		return nil, fmt.Errorf("initializing resource server: %w", err)
	}
	return &AuthService{repo: repo, resourceServer: resourceServer, logger: logger}, nil
}

// ValidateToken introspects the token against Zitadel. Works for PATs and JWTs.
func (s *AuthService) ValidateToken(ctx context.Context, rawToken string) (*ports.Claims, error) {
	resp, err := rs.Introspect[*oidc.IntrospectionResponse](ctx, s.resourceServer, rawToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !resp.Active {
		return nil, errors.New("token is not active")
	}

	claims := &ports.Claims{
		ZitadelID: resp.Subject,
		Email:     resp.Email,
		Name:      resp.Name,
		Roles:     extractRoles(resp.Claims),
	}
	return claims, nil
}

// SyncUser creates the user in the DB on first login or returns the existing record.
func (s *AuthService) SyncUser(ctx context.Context, claims *ports.Claims) (*domain.User, error) {
	user, err := s.repo.GetByZitadelID(ctx, claims.ZitadelID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	// Determine role from JWT claims; default to developer.
	role := domain.RoleDeveloper
	for _, r := range claims.Roles {
		if domain.Role(r).IsValid() {
			role = domain.Role(r)
			break
		}
	}

	user = &domain.User{
		ID:        uuid.New(),
		ZitadelID: claims.ZitadelID,
		Email:     claims.Email,
		Name:      claims.Name,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.Upsert(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	s.logger.Info("new user synced from Zitadel",
		zap.String("zitadel_id", claims.ZitadelID),
		zap.String("email", claims.Email),
		zap.String("role", string(role)),
	)
	return user, nil
}

// GetMe validates the token and returns the corresponding (possibly newly created) user.
func (s *AuthService) GetMe(ctx context.Context, rawToken string) (*domain.User, error) {
	claims, err := s.ValidateToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	return s.SyncUser(ctx, claims)
}

// GetUserByID returns a user by their internal UUID.
func (s *AuthService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	return s.repo.GetByID(ctx, uid)
}

// extractRoles reads the Zitadel role claim from the introspection response.
// The claim structure is: { "urn:zitadel:iam:org:project:roles": { "role-name": { "org-id": "org-name" } } }
func extractRoles(claims map[string]any) []string {
	const roleClaim = "urn:zitadel:iam:org:project:roles"
	rolesRaw, ok := claims[roleClaim]
	if !ok {
		return nil
	}
	rolesMap, ok := rolesRaw.(map[string]any)
	if !ok {
		return nil
	}
	roles := make([]string, 0, len(rolesMap))
	for role := range rolesMap {
		roles = append(roles, role)
	}
	return roles
}
