package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kungrem23/pulse-backend/internal/domain"
)

// TODO(db): SQL is written by the database developer. Keep the parameter order
// and the returned column order: the Go code scans results positionally.
// Time checks use the database clock (now()).
const (
	// Params: $1 user_id, $2 token_hash, $3 expires_at.
	// Inserts a refresh_sessions row. Returns one row: id (UUID as text).
	createRefreshSessionQuery = ""

	// Params: $1 token_hash, $2 user_id.
	// Returns zero or one row: id (UUID as text) of the session with this hash
	// owned by this user that is not revoked and not expired.
	// Must lock the row (FOR UPDATE) and keep "revoked_at IS NULL" in WHERE:
	// a concurrent rotation then waits and finds no row after the first commits.
	lockActiveRefreshSessionQuery = ""

	// Params: $1 old session id, $2 new session id.
	// Marks the old session revoked now and sets replaced_by to the new id.
	markRefreshSessionReplacedQuery = ""

	// Params: $1 user_id, $2 token_hash.
	// Marks the user's session with this hash revoked now if it is not revoked yet.
	// Zero affected rows is fine.
	revokeRefreshSessionQuery = ""
)

type RefreshSessionRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshSessionRepository(pool *pgxpool.Pool) *RefreshSessionRepository {
	return &RefreshSessionRepository{pool: pool}
}

func (r *RefreshSessionRepository) Create(ctx context.Context, session domain.RefreshSession) error {
	if createRefreshSessionQuery == "" {
		return fmt.Errorf("create refresh session: %w", errQueryNotWritten)
	}
	var id string
	err := r.pool.QueryRow(ctx, createRefreshSessionQuery, session.UserID, session.TokenHash, session.ExpiresAt).Scan(&id)
	if err != nil {
		return fmt.Errorf("create refresh session: %w", err)
	}
	return nil
}

func (r *RefreshSessionRepository) Rotate(ctx context.Context, oldTokenHash string, next domain.RefreshSession) error {
	if lockActiveRefreshSessionQuery == "" || createRefreshSessionQuery == "" || markRefreshSessionReplacedQuery == "" {
		return fmt.Errorf("rotate refresh session: %w", errQueryNotWritten)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rotate refresh session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var oldID string
	err = tx.QueryRow(ctx, lockActiveRefreshSessionQuery, oldTokenHash, next.UserID).Scan(&oldID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || pgErrorCode(err) == invalidTextRepresentation {
			return fmt.Errorf("rotate refresh session: %w", domain.ErrNotFound)
		}
		return fmt.Errorf("rotate refresh session: lock: %w", err)
	}

	var newID string
	err = tx.QueryRow(ctx, createRefreshSessionQuery, next.UserID, next.TokenHash, next.ExpiresAt).Scan(&newID)
	if err != nil {
		return fmt.Errorf("rotate refresh session: insert: %w", err)
	}
	if _, err := tx.Exec(ctx, markRefreshSessionReplacedQuery, oldID, newID); err != nil {
		return fmt.Errorf("rotate refresh session: revoke old: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("rotate refresh session: commit: %w", err)
	}
	return nil
}

func (r *RefreshSessionRepository) Revoke(ctx context.Context, userID, tokenHash string) error {
	if revokeRefreshSessionQuery == "" {
		return fmt.Errorf("revoke refresh session: %w", errQueryNotWritten)
	}
	if _, err := r.pool.Exec(ctx, revokeRefreshSessionQuery, userID, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	return nil
}
