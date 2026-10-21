package store_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"todoapp/internal/server/auth"
	"todoapp/internal/server/db"
	"todoapp/internal/server/store"
)

// openTestDB connects to the database in TEST_MARIADB_DSN, resets the schema
// (migrate down to 0, then up) and returns the connection. The test is
// skipped when the variable is unset. Because it DROPS ALL TABLES, the
// database name must contain "test".
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MARIADB_DSN")
	if dsn == "" {
		t.Skip("TEST_MARIADB_DSN not set; skipping MariaDB integration test")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MARIADB_DSN: %v", err)
	}
	if !strings.Contains(strings.ToLower(cfg.DBName), "test") {
		t.Fatalf("refusing to run: database %q does not contain \"test\" (this test drops all tables)", cfg.DBName)
	}

	ctx := context.Background()
	conn, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	p, err := db.NewMigrator(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return conn
}

func TestMariaDBUsersAndSessions(t *testing.T) {
	conn := openTestDB(t)
	st := store.New(conn)
	ctx := context.Background()

	u, err := st.CreateUser(ctx, "alice", "hash", "Asia/Jakarta")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.CreatedAt.IsZero() || u.CreatedAt.Location() != time.UTC {
		t.Fatalf("unexpected user %+v", u)
	}

	// Unique index is case-insensitive (utf8mb4_unicode_ci).
	if _, err := st.CreateUser(ctx, "ALICE", "hash", "UTC"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate username err = %v; want ErrConflict", err)
	}
	if _, err := st.GetUserByUsername(ctx, "nobody"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing user err = %v; want ErrNotFound", err)
	}
	if err := st.UpdateUserTimezone(ctx, u.ID, "Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetUserByUsername(ctx, "alice")
	if err != nil || got.Timezone != "Europe/Berlin" {
		t.Fatalf("GetUserByUsername = %+v, %v", got, err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	tok, _ := auth.NewToken()
	hash := auth.HashToken(tok)
	sess := store.Session{TokenHash: hash, UserID: u.ID, ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastUsedAt: now}
	if err := st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	gs, gu, err := st.GetSessionWithUser(ctx, hash, now)
	if err != nil {
		t.Fatalf("GetSessionWithUser: %v", err)
	}
	if !bytes.Equal(gs.TokenHash, hash) || gu.ID != u.ID || !gs.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("session = %+v user = %+v", gs, gu)
	}

	// Expired relative to a later clock.
	if _, _, err := st.GetSessionWithUser(ctx, hash, now.Add(2*time.Hour)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired session err = %v; want ErrNotFound", err)
	}

	// Sliding refresh.
	if err := st.TouchSession(ctx, hash, now.Add(time.Hour), now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.GetSessionWithUser(ctx, hash, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("touched session should be valid: %v", err)
	}

	n, err := st.DeleteExpiredSessions(ctx, now.Add(4*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpiredSessions = %d, %v; want 1", n, err)
	}
	if err := st.DeleteSession(ctx, hash); err != nil {
		t.Fatalf("DeleteSession on missing row should not fail: %v", err)
	}
}

func TestMariaDBSchemaConstraints(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	st := store.New(conn)
	u, err := st.CreateUser(ctx, "bob", "hash", "UTC")
	if err != nil {
		t.Fatal(err)
	}

	// priority CHECK constraint (1..3).
	_, err = conn.ExecContext(ctx,
		`INSERT INTO todos (id, user_id, date, title, priority) VALUES (UUID(), ?, CURDATE(), 't', 5)`, u.ID)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for priority=5")
	}

	// One journal entry per user per date.
	ins := `INSERT INTO journal_entries (id, user_id, date, body) VALUES (UUID(), ?, '2026-01-01', 'x')`
	if _, err := conn.ExecContext(ctx, ins, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, ins, u.ID); err == nil {
		t.Fatal("expected unique violation on (user_id, date)")
	}

	// FK cascade: deleting the user removes their data.
	if _, err := conn.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	var cnt int
	if err := conn.GetContext(ctx, &cnt, `SELECT COUNT(*) FROM journal_entries WHERE user_id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("journal rows after user delete = %d; want 0", cnt)
	}
}

func TestMariaDBTodos(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	st := store.New(conn)

	u, err := st.CreateUser(ctx, "todouser", "hash", "UTC")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	date := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	todo := store.Todo{
		ID:         "00000000-0000-0000-0000-000000000001",
		UserID:     u.ID,
		Date:       date,
		Title:      "DB Task",
		Done:       false,
		Priority:   2,
		RepeatRule: "none",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := st.CreateTodo(ctx, todo); err != nil {
		t.Fatalf("create todo: %v", err)
	}

	got, err := st.GetTodo(ctx, u.ID, todo.ID)
	if err != nil {
		t.Fatalf("get todo: %v", err)
	}
	if got.Title != "DB Task" {
		t.Fatalf("title mismatch: %s", got.Title)
	}

	list, err := st.ListTodosByDate(ctx, u.ID, date)
	if err != nil {
		t.Fatalf("list todos: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}

	todo.Done = true
	todo.UpdatedAt = time.Now().UTC()
	if err := st.UpdateTodo(ctx, todo); err != nil {
		t.Fatalf("update todo: %v", err)
	}

	if err := st.DeleteTodo(ctx, u.ID, todo.ID, time.Now().UTC()); err != nil {
		t.Fatalf("delete todo: %v", err)
	}

	_, err = st.GetTodo(ctx, u.ID, todo.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

