package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const userColumns = `id, username, password_hash, timezone, created_at, updated_at`

// CreateUser inserts a user. Returns ErrConflict if the username is taken.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash, timezone string) (User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, timezone) VALUES (?, ?, ?)`,
		username, passwordHash, timezone)
	if err != nil {
		if isDuplicate(err) {
			return User{}, fmt.Errorf("create user: %w", ErrConflict)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("create user: last insert id: %w", err)
	}
	return s.GetUserByID(ctx, id)
}

// GetUserByID returns the user with the given id or ErrNotFound.
func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("get user %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return u, nil
}

// GetUserByUsername returns the user with the given username or ErrNotFound.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT `+userColumns+` FROM users WHERE username = ?`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("get user by username: %w", ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by username: %w", err)
	}
	return u, nil
}

// UpdateUserTimezone sets the user's IANA timezone.
func (s *Store) UpdateUserTimezone(ctx context.Context, userID int64, timezone string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET timezone = ? WHERE id = ?`, timezone, userID); err != nil {
		return fmt.Errorf("update timezone for user %d: %w", userID, err)
	}
	return nil
}
