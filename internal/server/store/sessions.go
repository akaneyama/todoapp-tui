package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CreateSession stores a new session.
func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sess.TokenHash, sess.UserID, sess.ExpiresAt, sess.CreatedAt, sess.LastUsedAt)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("create session: %w", ErrConflict)
		}
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

type sessionUserRow struct {
	Session
	UUsername     string    `db:"u_username"`
	UPasswordHash string    `db:"u_password_hash"`
	UTimezone     string    `db:"u_timezone"`
	UCreatedAt    time.Time `db:"u_created_at"`
	UUpdatedAt    time.Time `db:"u_updated_at"`
}

// GetSessionWithUser returns a non-expired session (relative to now) and
// its owner, or ErrNotFound.
func (s *Store) GetSessionWithUser(ctx context.Context, tokenHash []byte, now time.Time) (Session, User, error) {
	var row sessionUserRow
	err := s.db.GetContext(ctx, &row,
		`SELECT s.token_hash, s.user_id, s.expires_at, s.created_at, s.last_used_at,
		        u.username      AS u_username,
		        u.password_hash AS u_password_hash,
		        u.timezone      AS u_timezone,
		        u.created_at    AS u_created_at,
		        u.updated_at    AS u_updated_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = ? AND s.expires_at > ?`,
		tokenHash, now)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, fmt.Errorf("get session: %w", ErrNotFound)
	}
	if err != nil {
		return Session{}, User{}, fmt.Errorf("get session: %w", err)
	}
	u := User{
		ID:           row.UserID,
		Username:     row.UUsername,
		PasswordHash: row.UPasswordHash,
		Timezone:     row.UTimezone,
		CreatedAt:    row.UCreatedAt,
		UpdatedAt:    row.UUpdatedAt,
	}
	return row.Session, u, nil
}

// TouchSession extends a session (sliding expiry).
func (s *Store) TouchSession(ctx context.Context, tokenHash []byte, lastUsedAt, expiresAt time.Time) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ?, expires_at = ? WHERE token_hash = ?`,
		lastUsedAt, expiresAt, tokenHash); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// DeleteSession removes a session. Deleting a missing session is not an
// error (logout is idempotent).
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions that expired at or before now.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: rows affected: %w", err)
	}
	return n, nil
}
