package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"todoapp/internal/models"
)

// JournalEntry is the persisted journal row in MariaDB.
type JournalEntry struct {
	ID        string     `db:"id"`
	UserID    int64      `db:"user_id"`
	Date      time.Time  `db:"date"`
	Title     string     `db:"title"`
	Body      string     `db:"body"`
	Tags      string     `db:"tags"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

// Public converts the DB row into the shared wire representation.
func (j JournalEntry) Public() models.JournalEntry {
	return models.JournalEntry{
		ID:        j.ID,
		Date:      j.Date.Format(time.DateOnly),
		Title:     j.Title,
		Body:      j.Body,
		Tags:      j.Tags,
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
		DeletedAt: j.DeletedAt,
	}
}

// ListJournalsByDate fetches all active journal entries for a user on a given date.
func (s *Store) ListJournalsByDate(ctx context.Context, userID int64, date time.Time) ([]JournalEntry, error) {
	const query = `
		SELECT id, user_id, date, title, body, tags, created_at, updated_at, deleted_at
		FROM journal_entries
		WHERE user_id = ? AND date = ? AND deleted_at IS NULL
		ORDER BY created_at ASC`
	var list []JournalEntry
	if err := s.db.SelectContext(ctx, &list, query, userID, date.Format(time.DateOnly)); err != nil {
		return nil, fmt.Errorf("list journals by date: %w", err)
	}
	return list, nil
}

// GetJournalByID fetches an active journal entry by ID.
func (s *Store) GetJournalByID(ctx context.Context, userID int64, id string) (JournalEntry, error) {
	const query = `
		SELECT id, user_id, date, title, body, tags, created_at, updated_at, deleted_at
		FROM journal_entries
		WHERE user_id = ? AND id = ? AND deleted_at IS NULL`
	var j JournalEntry
	if err := s.db.GetContext(ctx, &j, query, userID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JournalEntry{}, fmt.Errorf("get journal by id: %w", ErrNotFound)
		}
		return JournalEntry{}, fmt.Errorf("get journal by id: %w", err)
	}
	return j, nil
}

// GetJournalByDate fetches the first active journal entry for a user on a given date.
func (s *Store) GetJournalByDate(ctx context.Context, userID int64, date time.Time) (JournalEntry, error) {
	const query = `
		SELECT id, user_id, date, title, body, tags, created_at, updated_at, deleted_at
		FROM journal_entries
		WHERE user_id = ? AND date = ? AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1`
	var j JournalEntry
	if err := s.db.GetContext(ctx, &j, query, userID, date.Format(time.DateOnly)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JournalEntry{}, fmt.Errorf("get journal by date: %w", ErrNotFound)
		}
		return JournalEntry{}, fmt.Errorf("get journal by date: %w", err)
	}
	return j, nil
}

// UpsertJournal inserts or updates a journal entry for a user by ID.
func (s *Store) UpsertJournal(ctx context.Context, j JournalEntry) (JournalEntry, error) {
	const query = `
		INSERT INTO journal_entries (id, user_id, date, title, body, tags, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)
		ON DUPLICATE KEY UPDATE
			date = VALUES(date),
			title = VALUES(title),
			body = VALUES(body),
			tags = VALUES(tags),
			deleted_at = NULL,
			updated_at = VALUES(updated_at)`
	_, err := s.db.ExecContext(ctx, query,
		j.ID,
		j.UserID,
		j.Date.Format(time.DateOnly),
		j.Title,
		j.Body,
		j.Tags,
		j.CreatedAt,
		j.UpdatedAt,
	)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("upsert journal: %w", err)
	}
	return s.GetJournalByID(ctx, j.UserID, j.ID)
}

// DeleteJournalByID soft-deletes a specific journal entry by ID.
func (s *Store) DeleteJournalByID(ctx context.Context, userID int64, id string, now time.Time) error {
	const query = `
		UPDATE journal_entries
		SET deleted_at = ?, updated_at = ?
		WHERE user_id = ? AND id = ? AND deleted_at IS NULL`
	res, err := s.db.ExecContext(ctx, query, now, now, userID, id)
	if err != nil {
		return fmt.Errorf("delete journal by id: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete journal by id rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("delete journal by id: %w", ErrNotFound)
	}
	return nil
}

// DeleteJournal soft-deletes journal entries for a user on a given date.
func (s *Store) DeleteJournal(ctx context.Context, userID int64, date time.Time, now time.Time) error {
	const query = `
		UPDATE journal_entries
		SET deleted_at = ?, updated_at = ?
		WHERE user_id = ? AND date = ? AND deleted_at IS NULL`
	res, err := s.db.ExecContext(ctx, query, now, now, userID, date.Format(time.DateOnly))
	if err != nil {
		return fmt.Errorf("delete journal: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete journal rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("delete journal: %w", ErrNotFound)
	}
	return nil
}

type searchRow struct {
	ID        string    `db:"id"`
	Date      time.Time `db:"date"`
	Title     string    `db:"title"`
	Body      string    `db:"body"`
	Tags      string    `db:"tags"`
	UpdatedAt time.Time `db:"updated_at"`
}

// SearchJournal performs a search on journal title, tags, and body.
func (s *Store) SearchJournal(ctx context.Context, userID int64, searchQuery string) ([]models.JournalSearchMatch, error) {
	searchQuery = strings.TrimSpace(searchQuery)
	if searchQuery == "" {
		return nil, nil
	}

	words := strings.Fields(searchQuery)
	if len(words) == 0 {
		return nil, nil
	}

	var whereClauses []string
	var args []any
	args = append(args, userID)

	for _, w := range words {
		raw := w
		clean := strings.Trim(w, "#,;")
		if clean == "" {
			clean = raw
		}
		rawPattern := "%" + raw + "%"
		cleanPattern := "%" + clean + "%"

		whereClauses = append(whereClauses, "(title LIKE ? OR title LIKE ? OR tags LIKE ? OR tags LIKE ? OR body LIKE ?)")
		args = append(args, rawPattern, cleanPattern, rawPattern, cleanPattern, rawPattern)
	}

	// For ordering: prioritize entries where the entire query or first word matches title or tags
	firstClean := strings.Trim(words[0], "#,;")
	if firstClean == "" {
		firstClean = words[0]
	}
	wholePattern := "%" + searchQuery + "%"
	firstPattern := "%" + firstClean + "%"

	orderArgs := []any{
		wholePattern, firstPattern,
		wholePattern, firstPattern,
	}

	query := fmt.Sprintf(`
		SELECT id, date, title, body, tags, updated_at
		FROM journal_entries
		WHERE user_id = ? AND deleted_at IS NULL AND %s
		ORDER BY
		  CASE
		    WHEN title LIKE ? OR title LIKE ? THEN 1
		    WHEN tags LIKE ? OR tags LIKE ? THEN 2
		    ELSE 3
		  END,
		  date DESC, created_at DESC
		LIMIT 50`, strings.Join(whereClauses, " AND "))

	allArgs := append(args, orderArgs...)

	var rows []searchRow
	if err := s.db.SelectContext(ctx, &rows, query, allArgs...); err != nil {
		return nil, fmt.Errorf("search journal: %w", err)
	}

	matches := make([]models.JournalSearchMatch, 0, len(rows))
	for _, r := range rows {
		matches = append(matches, models.JournalSearchMatch{
			ID:        r.ID,
			Date:      r.Date.Format(time.DateOnly),
			Title:     r.Title,
			Snippet:   createSnippet(r.Body, searchQuery, 150),
			Tags:      r.Tags,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return matches, nil
}

// ListJournalSince returns all journal entries updated after `since`.
func (s *Store) ListJournalSince(ctx context.Context, userID int64, since time.Time) ([]JournalEntry, error) {
	const query = `
		SELECT id, user_id, date, title, body, tags, created_at, updated_at, deleted_at
		FROM journal_entries
		WHERE user_id = ? AND updated_at > ?
		ORDER BY updated_at ASC`
	var list []JournalEntry
	if err := s.db.SelectContext(ctx, &list, query, userID, since); err != nil {
		return nil, fmt.Errorf("list journal since: %w", err)
	}
	return list, nil
}

func createSnippet(body, query string, maxLen int) string {
	clean := strings.ReplaceAll(body, "\n", " ")
	lower := strings.ToLower(clean)
	qLower := strings.ToLower(strings.Trim(query, "+*~\""))

	idx := strings.Index(lower, qLower)
	if idx < 0 || len(clean) <= maxLen {
		if len(clean) > maxLen {
			return clean[:maxLen] + "..."
		}
		return clean
	}

	start := max(0, idx-40)
	end := min(len(clean), start+maxLen)
	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(clean) {
		suffix = "..."
	}
	return prefix + strings.TrimSpace(clean[start:end]) + suffix
}
