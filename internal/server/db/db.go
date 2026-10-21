// Package db opens the MariaDB connection pool and runs migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"todoapp/migrations"
)

// NormalizeDSN parses a go-sql-driver/mysql DSN and forces the settings the
// application relies on:
//   - parseTime + loc=UTC so DATETIME columns scan into UTC time.Time
//   - session time_zone '+00:00' so CURRENT_TIMESTAMP is UTC as well
//   - utf8mb4 collation
//   - interpolateParams=false so every query with arguments goes through a
//     server-side prepared statement
func NormalizeDSN(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DSN: %w", err)
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_unicode_ci"
	cfg.InterpolateParams = false
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg.FormatDSN(), nil
}

// Open connects to MariaDB and verifies the connection.
func Open(ctx context.Context, dsn string) (*sqlx.DB, error) {
	norm, err := NormalizeDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sqlx.Open("mysql", norm)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

// NewMigrator returns a goose provider over the embedded migrations.
func NewMigrator(db *sqlx.DB) (*goose.Provider, error) {
	return newMigrator(db, migrations.FS)
}

func newMigrator(db *sqlx.DB, fsys fs.FS) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectMySQL, db.DB, fsys)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return p, nil
}

// MigrateUp applies all pending migrations.
func MigrateUp(ctx context.Context, db *sqlx.DB, log *slog.Logger) error {
	p, err := NewMigrator(db)
	if err != nil {
		return err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	}
	return nil
}
