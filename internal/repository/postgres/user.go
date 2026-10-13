package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

const (
	uniqueViolation           = "23505"
	foreignKeyViolation       = "23503"
	invalidTextRepresentation = "22P02"
)

// Parameter and result column order follow the repository contract.
// The Go code scans results positionally.
const (
	// Params: $1 email (normalized), $2 password_hash, $3 nullable age.
	// Registration supplies nil; profile saves set the age later.
	// Returns one row: id (UUID as text), is_blocked, created_at.
	// A duplicate email must fail with unique_violation (23505).
	createUserQuery = `
INSERT INTO users (email, password_hash, age)
VALUES ($1, $2, $3)
RETURNING id::text, is_blocked, created_at
`

	// Params: $1 email (normalized).
	// Returns zero or one row: id (UUID as text), email, password_hash, age, is_blocked, created_at.
	userByEmailQuery = `
SELECT id::text, email, password_hash, age, is_blocked, created_at
FROM users WHERE email = $1
`

	// Params: $1 user id (UUID as text).
	// Returns the same columns as userByEmailQuery.
	userByIDQuery = `
SELECT id::text, email, password_hash, age, is_blocked, created_at
FROM users WHERE id = $1::uuid
`
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	err := r.pool.QueryRow(ctx, createUserQuery, user.Email, user.PasswordHash, user.Age).
		Scan(&user.ID, &user.IsBlocked, &user.CreatedAt)
	if err != nil {
		if pgErrorCode(err) == uniqueViolation {
			return domain.User{}, fmt.Errorf("create user: %w", domain.ErrConflict)
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *UserRepository) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.one(ctx, "find user by email", userByEmailQuery, email)
}

func (r *UserRepository) ByID(ctx context.Context, id string) (domain.User, error) {
	return r.one(ctx, "find user by id", userByIDQuery, id)
}

func (r *UserRepository) one(ctx context.Context, operation, query string, argument string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, query, argument).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Age, &user.IsBlocked, &user.CreatedAt,
	)
	if err != nil {
		// A malformed UUID cannot identify an existing user.
		if errors.Is(err, pgx.ErrNoRows) || pgErrorCode(err) == invalidTextRepresentation {
			return domain.User{}, fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
		}
		return domain.User{}, fmt.Errorf("%s: %w", operation, err)
	}
	return user, nil
}

func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
