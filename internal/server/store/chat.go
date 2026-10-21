package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"todoapp/internal/models"
)

// Room is the DB representation of a room.
type Room struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	IsDM      bool      `db:"is_dm"`
	DMKey     *string   `db:"dm_key"`
	CreatedAt time.Time `db:"created_at"`
}

// Message is the DB representation of a message.
type Message struct {
	ID        int64     `db:"id"`
	RoomID    int64     `db:"room_id"`
	UserID    int64     `db:"user_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`
}

// CreateRoom creates a new public or private room.
func (s *Store) CreateRoom(ctx context.Context, name string, isDM bool, dmKey *string) (Room, error) {
	now := time.Now().UTC()
	const query = `INSERT INTO rooms (name, is_dm, dm_key, created_at) VALUES (?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, query, name, isDM, dmKey, now)
	if err != nil {
		if isDuplicate(err) {
			return Room{}, fmt.Errorf("create room: %w", ErrConflict)
		}
		return Room{}, fmt.Errorf("create room: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Room{}, fmt.Errorf("last insert id: %w", err)
	}
	return Room{
		ID:        id,
		Name:      name,
		IsDM:      isDM,
		DMKey:     dmKey,
		CreatedAt: now,
	}, nil
}

// GetRoom retrieves a room by its ID.
func (s *Store) GetRoom(ctx context.Context, roomID int64) (Room, error) {
	const query = `SELECT id, name, is_dm, dm_key, created_at FROM rooms WHERE id = ?`
	var r Room
	if err := s.db.GetContext(ctx, &r, query, roomID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Room{}, fmt.Errorf("get room %d: %w", roomID, ErrNotFound)
		}
		return Room{}, fmt.Errorf("get room %d: %w", roomID, err)
	}
	return r, nil
}

// GetOrCreateDMRoom retrieves an existing DM room between two users or creates one.
func (s *Store) GetOrCreateDMRoom(ctx context.Context, userA, userB int64) (Room, error) {
	minID, maxID := min(userA, userB), max(userA, userB)
	dmKey := fmt.Sprintf("%d:%d", minID, maxID)

	const findQuery = `SELECT id, name, is_dm, dm_key, created_at FROM rooms WHERE dm_key = ?`
	var r Room
	err := s.db.GetContext(ctx, &r, findQuery, dmKey)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Room{}, fmt.Errorf("find dm room: %w", err)
	}

	// Create new DM room
	created, err := s.CreateRoom(ctx, "dm", true, &dmKey)
	if err != nil {
		return Room{}, err
	}
	if err := s.AddRoomMember(ctx, created.ID, userA); err != nil {
		return Room{}, err
	}
	if err := s.AddRoomMember(ctx, created.ID, userB); err != nil {
		return Room{}, err
	}
	return created, nil
}

// AddRoomMember adds a user to a room.
func (s *Store) AddRoomMember(ctx context.Context, roomID, userID int64) error {
	const query = `
		INSERT INTO room_members (room_id, user_id, joined_at)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE joined_at = joined_at`
	_, err := s.db.ExecContext(ctx, query, roomID, userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("add room member: %w", err)
	}
	return nil
}

// IsRoomMember checks if a user is a member of a room.
func (s *Store) IsRoomMember(ctx context.Context, roomID, userID int64) (bool, error) {
	const query = `SELECT COUNT(*) FROM room_members WHERE room_id = ? AND user_id = ?`
	var count int
	if err := s.db.GetContext(ctx, &count, query, roomID, userID); err != nil {
		return false, fmt.Errorf("check room member: %w", err)
	}
	return count > 0, nil
}

// ListRoomMembers returns all user IDs belonging to a room.
func (s *Store) ListRoomMembers(ctx context.Context, roomID int64) ([]int64, error) {
	const query = `SELECT user_id FROM room_members WHERE room_id = ?`
	var memberIDs []int64
	if err := s.db.SelectContext(ctx, &memberIDs, query, roomID); err != nil {
		return nil, fmt.Errorf("list room members: %w", err)
	}
	return memberIDs, nil
}

type roomListRow struct {
	ID          int64     `db:"id"`
	Name        string    `db:"name"`
	IsDM        bool      `db:"is_dm"`
	DMKey       *string   `db:"dm_key"`
	CreatedAt   time.Time `db:"created_at"`
	LastReadID  int64     `db:"last_read_id"`
}

// ListUserRooms returns all rooms a user belongs to, with resolved names for DMs and unread counts.
func (s *Store) ListUserRooms(ctx context.Context, userID int64) ([]models.Room, error) {
	const query = `
		SELECT r.id, r.name, r.is_dm, r.dm_key, r.created_at,
		       COALESCE(rs.last_read_message_id, 0) AS last_read_id
		FROM rooms r
		JOIN room_members rm ON rm.room_id = r.id
		LEFT JOIN read_state rs ON rs.room_id = r.id AND rs.user_id = ?
		WHERE rm.user_id = ?
		ORDER BY r.created_at ASC`
	var rows []roomListRow
	if err := s.db.SelectContext(ctx, &rows, query, userID, userID); err != nil {
		return nil, fmt.Errorf("list user rooms: %w", err)
	}

	result := make([]models.Room, 0, len(rows))
	for _, row := range rows {
		roomName := row.Name
		if row.IsDM {
			// Find the other member's username
			const otherUserQuery = `
				SELECT u.username
				FROM room_members rm
				JOIN users u ON u.id = rm.user_id
				WHERE rm.room_id = ? AND rm.user_id != ?
				LIMIT 1`
			var otherName string
			_ = s.db.GetContext(ctx, &otherName, otherUserQuery, row.ID, userID)
			if otherName != "" {
				roomName = "@" + otherName
			}
		}

		// Calculate unread count
		const unreadQuery = `SELECT COUNT(*) FROM messages WHERE room_id = ? AND id > ?`
		var unread int
		_ = s.db.GetContext(ctx, &unread, unreadQuery, row.ID, row.LastReadID)

		// Get last message
		var lastMsg *models.Message
		const lastMsgQuery = `
			SELECT m.id, m.room_id, m.user_id, u.username, m.body, m.created_at
			FROM messages m
			JOIN users u ON u.id = m.user_id
			WHERE m.room_id = ?
			ORDER BY m.id DESC LIMIT 1`
		type msgRow struct {
			ID        int64     `db:"id"`
			RoomID    int64     `db:"room_id"`
			UserID    int64     `db:"user_id"`
			Username  string    `db:"username"`
			Body      string    `db:"body"`
			CreatedAt time.Time `db:"created_at"`
		}
		var lr msgRow
		if err := s.db.GetContext(ctx, &lr, lastMsgQuery, row.ID); err == nil {
			lastMsg = &models.Message{
				ID:        lr.ID,
				RoomID:    lr.RoomID,
				UserID:    lr.UserID,
				Username:  lr.Username,
				Body:      lr.Body,
				CreatedAt: lr.CreatedAt,
			}
		}

		result = append(result, models.Room{
			ID:          row.ID,
			Name:        roomName,
			IsDM:        row.IsDM,
			CreatedAt:   row.CreatedAt,
			UnreadCount: unread,
			LastMessage: lastMsg,
		})
	}
	return result, nil
}

// CreateMessage inserts a new message and returns the populated wire model.
func (s *Store) CreateMessage(ctx context.Context, roomID, userID int64, body string) (models.Message, error) {
	now := time.Now().UTC()
	const query = `INSERT INTO messages (room_id, user_id, body, created_at) VALUES (?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, query, roomID, userID, body, now)
	if err != nil {
		return models.Message{}, fmt.Errorf("create message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Message{}, fmt.Errorf("last insert id: %w", err)
	}

	// Fetch username
	var username string
	const userQuery = `SELECT username FROM users WHERE id = ?`
	if err := s.db.GetContext(ctx, &username, userQuery, userID); err != nil {
		username = "unknown"
	}

	return models.Message{
		ID:        id,
		RoomID:    roomID,
		UserID:    userID,
		Username:  username,
		Body:      body,
		CreatedAt: now,
	}, nil
}

// ListMessages retrieves messages in a room with cursor pagination (beforeID).
// Returned in chronological order (oldest first).
func (s *Store) ListMessages(ctx context.Context, roomID int64, beforeID int64, limit int) ([]models.Message, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var query string
	var args []any
	if beforeID > 0 {
		query = `
			SELECT m.id, m.room_id, m.user_id, u.username, m.body, m.created_at
			FROM messages m
			JOIN users u ON u.id = m.user_id
			WHERE m.room_id = ? AND m.id < ?
			ORDER BY m.id DESC
			LIMIT ?`
		args = []any{roomID, beforeID, limit + 1}
	} else {
		query = `
			SELECT m.id, m.room_id, m.user_id, u.username, m.body, m.created_at
			FROM messages m
			JOIN users u ON u.id = m.user_id
			WHERE m.room_id = ?
			ORDER BY m.id DESC
			LIMIT ?`
		args = []any{roomID, limit + 1}
	}

	type msgRow struct {
		ID        int64     `db:"id"`
		RoomID    int64     `db:"room_id"`
		UserID    int64     `db:"user_id"`
		Username  string    `db:"username"`
		Body      string    `db:"body"`
		CreatedAt time.Time `db:"created_at"`
	}
	var rows []msgRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, false, fmt.Errorf("list messages: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	// Reverse rows to chronological order
	result := make([]models.Message, len(rows))
	for i, r := range rows {
		result[len(rows)-1-i] = models.Message{
			ID:        r.ID,
			RoomID:    r.RoomID,
			UserID:    r.UserID,
			Username:  r.Username,
			Body:      r.Body,
			CreatedAt: r.CreatedAt,
		}
	}
	return result, hasMore, nil
}

// UpdateReadState marks messages up to lastReadID as read.
func (s *Store) UpdateReadState(ctx context.Context, userID, roomID, lastReadID int64) error {
	const query = `
		INSERT INTO read_state (user_id, room_id, last_read_message_id, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			last_read_message_id = GREATEST(last_read_message_id, VALUES(last_read_message_id)),
			updated_at = VALUES(updated_at)`
	_, err := s.db.ExecContext(ctx, query, userID, roomID, lastReadID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update read state: %w", err)
	}
	return nil
}
