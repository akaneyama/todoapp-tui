// Package store implements persistence on MariaDB using sqlx. Every query
// uses `?` placeholders; with interpolateParams=false the driver sends them
// as server-side prepared statements.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"todoapp/internal/models"
)

// Sentinel errors returned by store implementations (including memstore).
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// User is the persisted user row, including the password hash. Never send
// this over the wire; use Public().
type User struct {
	ID           int64     `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	Timezone     string    `db:"timezone"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// Public converts the row into the shared wire type.
func (u User) Public() models.User {
	return models.User{
		ID:        u.ID,
		Username:  u.Username,
		Timezone:  u.Timezone,
		CreatedAt: u.CreatedAt,
	}
}

// Session is a persisted login session.
type Session struct {
	TokenHash  []byte    `db:"token_hash"`
	UserID     int64     `db:"user_id"`
	ExpiresAt  time.Time `db:"expires_at"`
	CreatedAt  time.Time `db:"created_at"`
	LastUsedAt time.Time `db:"last_used_at"`
}

// Store is the MariaDB-backed store.
type Store struct {
	db *sqlx.DB
}

// New wraps an open connection pool.
func New(db *sqlx.DB) *Store {
	return &Store{db: db}
}

// Ping checks database connectivity (used by /healthz).
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	return nil
}

// mysqlErrDuplicateEntry is ER_DUP_ENTRY.
const mysqlErrDuplicateEntry = 1062

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == mysqlErrDuplicateEntry
}
