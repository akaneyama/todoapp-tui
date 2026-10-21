// Package memstore is an in-memory, concurrency-safe implementation of the
// store methods, used by unit and HTTP tests so they run without MariaDB.
// It mirrors the semantics of the MariaDB store (sentinel errors, expiry
// filtering) closely enough for business-logic tests.
package memstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"todoapp/internal/models"
	"todoapp/internal/server/store"
)

// Store is the in-memory store.
type Store struct {
	mu            sync.Mutex
	nextID        int64
	nextRoomID    int64
	nextMessageID int64
	users         map[int64]store.User
	sessions      map[string]store.Session // key: hex(token_hash)
	todos         map[string]store.Todo    // key: id
	journals      map[string]store.JournalEntry // key: id
	rooms         map[int64]store.Room
	roomMembers   map[int64]map[int64]time.Time // roomID -> userID -> joinedAt
	messages      map[int64]store.Message       // messageID -> message
	readState     map[string]int64              // userID:roomID -> lastReadMessageID
	now           func() time.Time
}

// New returns an empty store. now is used for created_at defaults; pass
// nil to use time.Now.
func New(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		users:       map[int64]store.User{},
		sessions:    map[string]store.Session{},
		todos:       map[string]store.Todo{},
		journals:    map[string]store.JournalEntry{},
		rooms:       map[int64]store.Room{},
		roomMembers: map[int64]map[int64]time.Time{},
		messages:    map[int64]store.Message{},
		readState:   map[string]int64{},
		now:         now,
	}
}

// Ping always succeeds.
func (s *Store) Ping(context.Context) error { return nil }

// CreateUser implements the store contract.
func (s *Store) CreateUser(_ context.Context, username, passwordHash, timezone string) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		// utf8mb4_unicode_ci is case-insensitive; mirror that.
		if strings.EqualFold(u.Username, username) {
			return store.User{}, fmt.Errorf("create user: %w", store.ErrConflict)
		}
	}
	s.nextID++
	now := s.now().UTC()
	u := store.User{
		ID:           s.nextID,
		Username:     username,
		PasswordHash: passwordHash,
		Timezone:     timezone,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users[u.ID] = u
	return u, nil
}

// GetUserByID implements the store contract.
func (s *Store) GetUserByID(_ context.Context, id int64) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return store.User{}, fmt.Errorf("get user %d: %w", id, store.ErrNotFound)
	}
	return u, nil
}

// GetUserByUsername implements the store contract.
func (s *Store) GetUserByUsername(_ context.Context, username string) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if strings.EqualFold(u.Username, username) {
			return u, nil
		}
	}
	return store.User{}, fmt.Errorf("get user by username: %w", store.ErrNotFound)
}

// UpdateUserTimezone implements the store contract.
func (s *Store) UpdateUserTimezone(_ context.Context, userID int64, timezone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return nil // UPDATE on a missing row affects 0 rows, not an error.
	}
	u.Timezone = timezone
	u.UpdatedAt = s.now().UTC()
	s.users[userID] = u
	return nil
}

// CreateSession implements the store contract.
func (s *Store) CreateSession(_ context.Context, sess store.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := hex.EncodeToString(sess.TokenHash)
	if _, ok := s.sessions[k]; ok {
		return fmt.Errorf("create session: %w", store.ErrConflict)
	}
	if _, ok := s.users[sess.UserID]; !ok {
		return fmt.Errorf("create session: unknown user %d (foreign key)", sess.UserID)
	}
	sess.TokenHash = append([]byte(nil), sess.TokenHash...)
	s.sessions[k] = sess
	return nil
}

// GetSessionWithUser implements the store contract.
func (s *Store) GetSessionWithUser(_ context.Context, tokenHash []byte, now time.Time) (store.Session, store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[hex.EncodeToString(tokenHash)]
	if !ok || !sess.ExpiresAt.After(now) {
		return store.Session{}, store.User{}, fmt.Errorf("get session: %w", store.ErrNotFound)
	}
	u, ok := s.users[sess.UserID]
	if !ok {
		return store.Session{}, store.User{}, fmt.Errorf("get session: %w", store.ErrNotFound)
	}
	return sess, u, nil
}

// TouchSession implements the store contract.
func (s *Store) TouchSession(_ context.Context, tokenHash []byte, lastUsedAt, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := hex.EncodeToString(tokenHash)
	sess, ok := s.sessions[k]
	if !ok {
		return nil
	}
	sess.LastUsedAt = lastUsedAt
	sess.ExpiresAt = expiresAt
	s.sessions[k] = sess
	return nil
}

// DeleteSession implements the store contract.
func (s *Store) DeleteSession(_ context.Context, tokenHash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, hex.EncodeToString(tokenHash))
	return nil
}

// DeleteExpiredSessions implements the store contract.
func (s *Store) DeleteExpiredSessions(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for k, sess := range s.sessions {
		if !sess.ExpiresAt.After(now) {
			delete(s.sessions, k)
			n++
		}
	}
	return n, nil
}

// SessionCount returns the number of stored sessions (test helper).
func (s *Store) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// CreateTodo implements the store contract.
func (s *Store) CreateTodo(_ context.Context, t store.Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.todos[t.ID]; ok {
		return fmt.Errorf("create todo: %w", store.ErrConflict)
	}
	s.todos[t.ID] = t
	return nil
}

// CreateTodosBatch implements the store contract.
func (s *Store) CreateTodosBatch(_ context.Context, todos []store.Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range todos {
		if _, ok := s.todos[t.ID]; ok {
			return fmt.Errorf("create todos batch: %w", store.ErrConflict)
		}
	}
	for _, t := range todos {
		s.todos[t.ID] = t
	}
	return nil
}

// GetTodo implements the store contract.
func (s *Store) GetTodo(_ context.Context, userID int64, id string) (store.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok || t.UserID != userID || t.DeletedAt != nil {
		return store.Todo{}, fmt.Errorf("get todo: %w", store.ErrNotFound)
	}
	return t, nil
}

// ListTodosByDate implements the store contract.
func (s *Store) ListTodosByDate(_ context.Context, userID int64, date time.Time) ([]store.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dateStr := date.Format(time.DateOnly)
	var res []store.Todo
	for _, t := range s.todos {
		if t.UserID == userID && t.DeletedAt == nil && t.Date.Format(time.DateOnly) == dateStr {
			res = append(res, t)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Done != res[j].Done {
			return !res[i].Done // undone first
		}
		if res[i].Priority != res[j].Priority {
			return res[i].Priority < res[j].Priority
		}
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})
	return res, nil
}

// ListTodosSince implements the store contract.
func (s *Store) ListTodosSince(_ context.Context, userID int64, since time.Time) ([]store.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []store.Todo
	for _, t := range s.todos {
		if t.UserID == userID && t.UpdatedAt.After(since) {
			res = append(res, t)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].UpdatedAt.Before(res[j].UpdatedAt)
	})
	return res, nil
}

// UpdateTodo implements the store contract.
func (s *Store) UpdateTodo(_ context.Context, t store.Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.todos[t.ID]
	if !ok || existing.UserID != t.UserID || existing.DeletedAt != nil {
		return fmt.Errorf("update todo: %w", store.ErrNotFound)
	}
	s.todos[t.ID] = t
	return nil
}

// DeleteTodo implements the store contract.
func (s *Store) DeleteTodo(_ context.Context, userID int64, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.todos[id]
	if !ok || existing.UserID != userID || existing.DeletedAt != nil {
		return fmt.Errorf("delete todo: %w", store.ErrNotFound)
	}
	existing.DeletedAt = &now
	existing.UpdatedAt = now
	s.todos[id] = existing
	return nil
}

// RolloverTodos implements the store contract.
func (s *Store) RolloverTodos(_ context.Context, userID int64, fromDate, toDate, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fromStr := fromDate.Format(time.DateOnly)
	var count int64
	for id, t := range s.todos {
		if t.UserID == userID && t.DeletedAt == nil && !t.Done && t.Date.Format(time.DateOnly) == fromStr {
			t.Date = toDate
			t.UpdatedAt = now
			s.todos[id] = t
			count++
		}
	}
	return count, nil
}

// ListDueReminders implements the store contract.
func (s *Store) ListDueReminders(_ context.Context, now time.Time, limit int) ([]store.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []store.Todo
	for _, t := range s.todos {
		if t.RemindAt != nil && !t.RemindAt.After(now) && t.RemindedAt == nil && !t.Done && t.DeletedAt == nil {
			res = append(res, t)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].RemindAt.Before(*res[j].RemindAt)
	})
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// MarkReminderSent implements the store contract.
func (s *Store) MarkReminderSent(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.todos[id]
	if !ok || t.RemindedAt != nil {
		return nil
	}
	t.RemindedAt = &now
	s.todos[id] = t
	return nil
}

// ListJournalsByDate implements the store contract.
func (s *Store) ListJournalsByDate(_ context.Context, userID int64, date time.Time) ([]store.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dateStr := date.Format(time.DateOnly)
	var res []store.JournalEntry
	for _, j := range s.journals {
		if j.UserID == userID && j.DeletedAt == nil && j.Date.Format(time.DateOnly) == dateStr {
			res = append(res, j)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})
	return res, nil
}

// GetJournalByID implements the store contract.
func (s *Store) GetJournalByID(_ context.Context, userID int64, id string) (store.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.journals[id]
	if !ok || j.UserID != userID || j.DeletedAt != nil {
		return store.JournalEntry{}, fmt.Errorf("get journal by id: %w", store.ErrNotFound)
	}
	return j, nil
}

// GetJournalByDate implements the store contract.
func (s *Store) GetJournalByDate(_ context.Context, userID int64, date time.Time) (store.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dateStr := date.Format(time.DateOnly)
	var matched []store.JournalEntry
	for _, j := range s.journals {
		if j.UserID == userID && j.DeletedAt == nil && j.Date.Format(time.DateOnly) == dateStr {
			matched = append(matched, j)
		}
	}
	if len(matched) == 0 {
		return store.JournalEntry{}, fmt.Errorf("get journal by date: %w", store.ErrNotFound)
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.Before(matched[j].CreatedAt)
	})
	return matched[0], nil
}

// UpsertJournal implements the store contract.
func (s *Store) UpsertJournal(_ context.Context, j store.JournalEntry) (store.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.ID == "" {
		j.ID = uuid.New().String()
	}
	existing, ok := s.journals[j.ID]
	if ok {
		j.CreatedAt = existing.CreatedAt
	}
	j.DeletedAt = nil
	s.journals[j.ID] = j
	return j, nil
}

// DeleteJournalByID implements the store contract.
func (s *Store) DeleteJournalByID(_ context.Context, userID int64, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.journals[id]
	if !ok || j.UserID != userID || j.DeletedAt != nil {
		return fmt.Errorf("delete journal by id: %w", store.ErrNotFound)
	}
	j.DeletedAt = &now
	j.UpdatedAt = now
	s.journals[id] = j
	return nil
}

// DeleteJournal implements the store contract.
func (s *Store) DeleteJournal(_ context.Context, userID int64, date time.Time, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dateStr := date.Format(time.DateOnly)
	var found bool
	for id, j := range s.journals {
		if j.UserID == userID && j.DeletedAt == nil && j.Date.Format(time.DateOnly) == dateStr {
			j.DeletedAt = &now
			j.UpdatedAt = now
			s.journals[id] = j
			found = true
		}
	}
	if !found {
		return fmt.Errorf("delete journal: %w", store.ErrNotFound)
	}
	return nil
}

// SearchJournal implements the store contract.
func (s *Store) SearchJournal(_ context.Context, userID int64, searchQuery string) ([]models.JournalSearchMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	searchQuery = strings.TrimSpace(searchQuery)
	if searchQuery == "" {
		return nil, nil
	}
	words := strings.Fields(strings.ToLower(searchQuery))
	if len(words) == 0 {
		return nil, nil
	}

	var matches []models.JournalSearchMatch
	for _, j := range s.journals {
		if j.UserID == userID && j.DeletedAt == nil {
			titleLower := strings.ToLower(j.Title)
			tagsLower := strings.ToLower(j.Tags)
			bodyLower := strings.ToLower(j.Body)

			allMatch := true
			for _, w := range words {
				clean := strings.Trim(w, "#,;")
				if clean == "" {
					clean = w
				}
				matchWord := strings.Contains(titleLower, w) || strings.Contains(titleLower, clean) ||
					strings.Contains(tagsLower, w) || strings.Contains(tagsLower, clean) ||
					strings.Contains(bodyLower, w)
				if !matchWord {
					allMatch = false
					break
				}
			}

			if allMatch {
				snippet := j.Body
				if len(snippet) > 150 {
					snippet = snippet[:150] + "..."
				}
				matches = append(matches, models.JournalSearchMatch{
					ID:        j.ID,
					Date:      j.Date.Format(time.DateOnly),
					Title:     j.Title,
					Snippet:   snippet,
					Tags:      j.Tags,
					UpdatedAt: j.UpdatedAt,
				})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Date > matches[j].Date
	})
	return matches, nil
}

// ListJournalSince implements the store contract.
func (s *Store) ListJournalSince(_ context.Context, userID int64, since time.Time) ([]store.JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res []store.JournalEntry
	for _, j := range s.journals {
		if j.UserID == userID && j.UpdatedAt.After(since) {
			res = append(res, j)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].UpdatedAt.Before(res[j].UpdatedAt)
	})
	return res, nil
}

// CreateRoom implements the store contract.
func (s *Store) CreateRoom(_ context.Context, name string, isDM bool, dmKey *string) (store.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRoomID++
	r := store.Room{
		ID:        s.nextRoomID,
		Name:      name,
		IsDM:      isDM,
		DMKey:     dmKey,
		CreatedAt: s.now().UTC(),
	}
	s.rooms[r.ID] = r
	if s.roomMembers[r.ID] == nil {
		s.roomMembers[r.ID] = map[int64]time.Time{}
	}
	return r, nil
}

// GetRoom implements the store contract.
func (s *Store) GetRoom(_ context.Context, roomID int64) (store.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rooms[roomID]
	if !ok {
		return store.Room{}, fmt.Errorf("get room: %w", store.ErrNotFound)
	}
	return r, nil
}

// GetOrCreateDMRoom implements the store contract.
func (s *Store) GetOrCreateDMRoom(ctx context.Context, userA, userB int64) (store.Room, error) {
	s.mu.Lock()
	minID, maxID := min(userA, userB), max(userA, userB)
	dmKey := fmt.Sprintf("%d:%d", minID, maxID)
	for _, r := range s.rooms {
		if r.IsDM && r.DMKey != nil && *r.DMKey == dmKey {
			s.mu.Unlock()
			return r, nil
		}
	}
	s.mu.Unlock()

	created, err := s.CreateRoom(ctx, "dm", true, &dmKey)
	if err != nil {
		return store.Room{}, err
	}
	_ = s.AddRoomMember(ctx, created.ID, userA)
	_ = s.AddRoomMember(ctx, created.ID, userB)
	return created, nil
}

// AddRoomMember implements the store contract.
func (s *Store) AddRoomMember(_ context.Context, roomID, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.roomMembers[roomID] == nil {
		s.roomMembers[roomID] = map[int64]time.Time{}
	}
	s.roomMembers[roomID][userID] = s.now().UTC()
	return nil
}

// IsRoomMember implements the store contract.
func (s *Store) IsRoomMember(_ context.Context, roomID, userID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.roomMembers[roomID]
	if !ok {
		return false, nil
	}
	_, isMember := members[userID]
	return isMember, nil
}

// ListRoomMembers implements the store contract.
func (s *Store) ListRoomMembers(_ context.Context, roomID int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.roomMembers[roomID]
	if !ok {
		return nil, nil
	}
	var res []int64
	for uID := range members {
		res = append(res, uID)
	}
	return res, nil
}

// ListUserRooms implements the store contract.
func (s *Store) ListUserRooms(_ context.Context, userID int64) ([]models.Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []models.Room
	for rID, members := range s.roomMembers {
		if _, ok := members[userID]; ok {
			room := s.rooms[rID]
			name := room.Name
			if room.IsDM {
				for otherID := range members {
					if otherID != userID {
						if u, exists := s.users[otherID]; exists {
							name = "@" + u.Username
						}
					}
				}
			}

			readKey := fmt.Sprintf("%d:%d", userID, rID)
			lastReadID := s.readState[readKey]

			unread := 0
			var lastMsg *models.Message
			for _, m := range s.messages {
				if m.RoomID == rID {
					if m.ID > lastReadID {
						unread++
					}
					if lastMsg == nil || m.ID > lastMsg.ID {
						sender := "unknown"
						if u, exists := s.users[m.UserID]; exists {
							sender = u.Username
						}
						lastMsg = &models.Message{
							ID:        m.ID,
							RoomID:    m.RoomID,
							UserID:    m.UserID,
							Username:  sender,
							Body:      m.Body,
							CreatedAt: m.CreatedAt,
						}
					}
				}
			}

			result = append(result, models.Room{
				ID:          room.ID,
				Name:        name,
				IsDM:        room.IsDM,
				CreatedAt:   room.CreatedAt,
				UnreadCount: unread,
				LastMessage: lastMsg,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// CreateMessage implements the store contract.
func (s *Store) CreateMessage(_ context.Context, roomID, userID int64, body string) (models.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextMessageID++
	now := s.now().UTC()
	m := store.Message{
		ID:        s.nextMessageID,
		RoomID:    roomID,
		UserID:    userID,
		Body:      body,
		CreatedAt: now,
	}
	s.messages[m.ID] = m

	username := "unknown"
	if u, ok := s.users[userID]; ok {
		username = u.Username
	}

	return models.Message{
		ID:        m.ID,
		RoomID:    m.RoomID,
		UserID:    m.UserID,
		Username:  username,
		Body:      body,
		CreatedAt: now,
	}, nil
}

// ListMessages implements the store contract.
func (s *Store) ListMessages(_ context.Context, roomID int64, beforeID int64, limit int) ([]models.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var matched []store.Message
	for _, m := range s.messages {
		if m.RoomID == roomID {
			if beforeID == 0 || m.ID < beforeID {
				matched = append(matched, m)
			}
		}
	}

	// Sort newest first to apply limit
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID > matched[j].ID
	})

	hasMore := len(matched) > limit
	if hasMore {
		matched = matched[:limit]
	}

	// Reverse to chronological order (oldest first)
	result := make([]models.Message, len(matched))
	for i, m := range matched {
		username := "unknown"
		if u, ok := s.users[m.UserID]; ok {
			username = u.Username
		}
		result[len(matched)-1-i] = models.Message{
			ID:        m.ID,
			RoomID:    m.RoomID,
			UserID:    m.UserID,
			Username:  username,
			Body:      m.Body,
			CreatedAt: m.CreatedAt,
		}
	}
	return result, hasMore, nil
}

// UpdateReadState implements the store contract.
func (s *Store) UpdateReadState(_ context.Context, userID, roomID, lastReadID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%d:%d", userID, roomID)
	if lastReadID > s.readState[key] {
		s.readState[key] = lastReadID
	}
	return nil
}



