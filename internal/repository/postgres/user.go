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
	invalidTextRepresentation = "22P02"
)

// TODO(db): SQL is written by the database developer. Keep the parameter order
// and the returned column order: the Go code scans results positionally.
const (
	// Params: $1 email (normalized), $2 password_hash, $3 age.
	// Returns one row: id (UUID as text), is_blocked, created_at.
	// A duplicate email must fail with unique_violation (23505).
	createUserQuery = ""

	// Params: $1 email (normalized).
	// Returns zero or one row: id (UUID as text), email, password_hash, age, is_blocked, created_at.
	userByEmailQuery = ""

	// Params: $1 user id (UUID as text).
	// Returns the same columns as userByEmailQuery.
	userByIDQuery = ""
)

var errQueryNotWritten = errors.New("SQL query is not written yet")

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	if createUserQuery == "" {
		return domain.User{}, fmt.Errorf("create user: %w", errQueryNotWritten)
	}
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
	if query == "" {
		return domain.User{}, fmt.Errorf("%s: %w", operation, errQueryNotWritten)
	}
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
