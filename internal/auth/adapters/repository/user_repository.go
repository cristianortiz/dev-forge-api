package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cristianortiz/dev-forge/internal/auth/domain"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
)

// UserRepository implements ports.UserRepository using PostgreSQL via pgx.
type UserRepository struct {
	db *database.DB
}

// New creates a UserRepository.
func New(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByID returns the user with the given internal UUID.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const q = `
		SELECT id, zitadel_id, email, name, role, created_at
		FROM users WHERE id = $1`
	row := r.db.Pool.QueryRow(ctx, q, id)
	return scanUser(row)
}

// GetByZitadelID returns the user with the given Zitadel subject ID.
func (r *UserRepository) GetByZitadelID(ctx context.Context, zitadelID string) (*domain.User, error) {
	const q = `
		SELECT id, zitadel_id, email, name, role, created_at
		FROM users WHERE zitadel_id = $1`
	row := r.db.Pool.QueryRow(ctx, q, zitadelID)
	return scanUser(row)
}

// Upsert inserts a new user or updates email/name on conflict with zitadel_id.
func (r *UserRepository) Upsert(ctx context.Context, user *domain.User) error {
	const q = `
		INSERT INTO users (id, zitadel_id, email, name, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (zitadel_id) DO UPDATE
			SET email = EXCLUDED.email,
			    name  = EXCLUDED.name`
	_, err := r.db.Pool.Exec(ctx, q,
		user.ID, user.ZitadelID, user.Email, user.Name, string(user.Role), user.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}

// List returns all users ordered by creation date (newest first).
func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	const q = `
		SELECT id, zitadel_id, email, name, role, created_at
		FROM users ORDER BY created_at DESC`
	rows, err := r.db.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.ZitadelID, &u.Email, &u.Name, &role, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.Role = domain.Role(role)
	return &u, nil
}
