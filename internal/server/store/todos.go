package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"todoapp/internal/models"
)

// Todo is the persisted todo row in MariaDB.
type Todo struct {
	ID          string     `db:"id"`
	UserID      int64      `db:"user_id"`
	Date        time.Time  `db:"date"`
	Title       string     `db:"title"`
	Description *string    `db:"description"`
	Done        bool       `db:"done"`
	Priority    uint8      `db:"priority"`
	RemindAt    *time.Time `db:"remind_at"`
	RemindedAt  *time.Time `db:"reminded_at"`
	RepeatRule  string     `db:"repeat_rule"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	DeletedAt   *time.Time `db:"deleted_at"`
}

// Public converts the DB row into the shared wire representation.
func (t Todo) Public() models.Todo {
	return models.Todo{
		ID:          t.ID,
		Date:        t.Date.Format(time.DateOnly),
		Title:       t.Title,
		Description: t.Description,
		Done:        t.Done,
		Priority:    t.Priority,
		RemindAt:    t.RemindAt,
		RemindedAt:  t.RemindedAt,
		RepeatRule:  t.RepeatRule,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		DeletedAt:   t.DeletedAt,
	}
}

// CreateTodo inserts a new todo into MariaDB.
func (s *Store) CreateTodo(ctx context.Context, t Todo) error {
	const query = `
		INSERT INTO todos (id, user_id, date, title, description, done, priority, remind_at, repeat_rule, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		t.ID,
		t.UserID,
		t.Date.Format(time.DateOnly),
		t.Title,
		t.Description,
		t.Done,
		t.Priority,
		t.RemindAt,
		t.RepeatRule,
		t.CreatedAt,
		t.UpdatedAt,
	)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("create todo: %w", ErrConflict)
		}
		return fmt.Errorf("create todo: %w", err)
	}
	return nil
}

// CreateTodosBatch inserts multiple todos in a single transaction.
func (s *Store) CreateTodosBatch(ctx context.Context, todos []Todo) error {
	if len(todos) == 0 {
		return nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const query = `
		INSERT INTO todos (id, user_id, date, title, description, done, priority, remind_at, repeat_rule, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PreparexContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch stmt: %w", err)
	}
	defer stmt.Close()

	for _, t := range todos {
		_, err := stmt.ExecContext(ctx,
			t.ID,
			t.UserID,
			t.Date.Format(time.DateOnly),
			t.Title,
			t.Description,
			t.Done,
			t.Priority,
			t.RemindAt,
			t.RepeatRule,
			t.CreatedAt,
			t.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("batch insert todo: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}
	return nil
}

// GetTodo fetches a single non-deleted todo belonging to userID.
func (s *Store) GetTodo(ctx context.Context, userID int64, id string) (Todo, error) {
	const query = `
		SELECT id, user_id, date, title, description, done, priority, remind_at, reminded_at, repeat_rule, created_at, updated_at, deleted_at
		FROM todos
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	var t Todo
	if err := s.db.GetContext(ctx, &t, query, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Todo{}, fmt.Errorf("get todo: %w", ErrNotFound)
		}
		return Todo{}, fmt.Errorf("get todo: %w", err)
	}
	return t, nil
}

// ListTodosByDate returns all non-deleted todos for a user on a given date.
func (s *Store) ListTodosByDate(ctx context.Context, userID int64, date time.Time) ([]Todo, error) {
	const query = `
		SELECT id, user_id, date, title, description, done, priority, remind_at, reminded_at, repeat_rule, created_at, updated_at, deleted_at
		FROM todos
		WHERE user_id = ? AND date = ? AND deleted_at IS NULL
		ORDER BY done ASC, priority ASC, created_at ASC`
	var list []Todo
	if err := s.db.SelectContext(ctx, &list, query, userID, date.Format(time.DateOnly)); err != nil {
		return nil, fmt.Errorf("list todos by date: %w", err)
	}
	return list, nil
}

// ListTodosSince returns all todos (including soft-deleted) updated after `since`.
func (s *Store) ListTodosSince(ctx context.Context, userID int64, since time.Time) ([]Todo, error) {
	const query = `
		SELECT id, user_id, date, title, description, done, priority, remind_at, reminded_at, repeat_rule, created_at, updated_at, deleted_at
		FROM todos
		WHERE user_id = ? AND updated_at > ?
		ORDER BY updated_at ASC`
	var list []Todo
	if err := s.db.SelectContext(ctx, &list, query, userID, since); err != nil {
		return nil, fmt.Errorf("list todos since: %w", err)
	}
	return list, nil
}

// UpdateTodo updates an existing non-deleted todo owned by userID.
func (s *Store) UpdateTodo(ctx context.Context, t Todo) error {
	const query = `
		UPDATE todos
		SET date = ?, title = ?, description = ?, done = ?, priority = ?, remind_at = ?, reminded_at = ?, repeat_rule = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	res, err := s.db.ExecContext(ctx, query,
		t.Date.Format(time.DateOnly),
		t.Title,
		t.Description,
		t.Done,
		t.Priority,
		t.RemindAt,
		t.RemindedAt,
		t.RepeatRule,
		t.UpdatedAt,
		t.ID,
		t.UserID,
	)
	if err != nil {
		return fmt.Errorf("update todo: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update todo rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("update todo: %w", ErrNotFound)
	}
	return nil
}

// ListDueReminders returns pending reminders where remind_at <= now, reminded_at IS NULL, done = FALSE, and deleted_at IS NULL.
func (s *Store) ListDueReminders(ctx context.Context, now time.Time, limit int) ([]Todo, error) {
	const query = `
		SELECT id, user_id, date, title, description, done, priority, remind_at, reminded_at, repeat_rule, created_at, updated_at, deleted_at
		FROM todos
		WHERE remind_at IS NOT NULL
		  AND remind_at <= ?
		  AND reminded_at IS NULL
		  AND done = FALSE
		  AND deleted_at IS NULL
		ORDER BY remind_at ASC
		LIMIT ?`
	var list []Todo
	if err := s.db.SelectContext(ctx, &list, query, now, limit); err != nil {
		return nil, fmt.Errorf("list due reminders: %w", err)
	}
	return list, nil
}

// MarkReminderSent sets reminded_at = now on a todo if it has not already been marked.
func (s *Store) MarkReminderSent(ctx context.Context, id string, now time.Time) error {
	const query = `
		UPDATE todos
		SET reminded_at = ?, updated_at = updated_at
		WHERE id = ? AND reminded_at IS NULL`
	_, err := s.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("mark reminder sent: %w", err)
	}
	return nil
}

// DeleteTodo soft-deletes a todo by setting deleted_at.
func (s *Store) DeleteTodo(ctx context.Context, userID int64, id string, now time.Time) error {
	const query = `
		UPDATE todos
		SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	res, err := s.db.ExecContext(ctx, query, now, now, id, userID)
	if err != nil {
		return fmt.Errorf("delete todo: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete todo rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("delete todo: %w", ErrNotFound)
	}
	return nil
}

// RolloverTodos moves all incomplete todos from fromDate to toDate.
func (s *Store) RolloverTodos(ctx context.Context, userID int64, fromDate, toDate, now time.Time) (int64, error) {
	const query = `
		UPDATE todos
		SET date = ?, updated_at = ?
		WHERE user_id = ? AND date = ? AND done = FALSE AND deleted_at IS NULL`
	res, err := s.db.ExecContext(ctx, query,
		toDate.Format(time.DateOnly),
		now,
		userID,
		fromDate.Format(time.DateOnly),
	)
	if err != nil {
		return 0, fmt.Errorf("rollover todos: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rollover todos rows: %w", err)
	}
	return rows, nil
}
