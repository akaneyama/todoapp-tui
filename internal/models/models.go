// Package models contains the wire types shared by the server and the TUI
// client. Keep these free of server-only concerns (e.g. password hashes).
package models

import "time"

// User is the public representation of an account.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterRequest is the body of POST /api/v1/auth/register.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Timezone is an IANA zone name such as "Asia/Jakarta". Optional;
	// defaults to "UTC".
	Timezone string `json:"timezone,omitempty"`
}

// LoginRequest is the body of POST /api/v1/auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Timezone, when set, updates the stored timezone of the user so the
	// server's notion of "today" follows the device the user is on.
	Timezone string `json:"timezone,omitempty"`
}

// LoginResponse is returned on successful login.
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// UserResponse wraps a single user.
type UserResponse struct {
	User User `json:"user"`
}

// ErrorResponse is the envelope used for every non-2xx JSON response.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// APIError describes a failure in a machine-readable (Code) and
// human-readable (Message) way.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Field is set for validation errors to point at the offending input.
	Field string `json:"field,omitempty"`
}

// Error codes returned by the API.
const (
	CodeBadRequest         = "bad_request"
	CodeValidation         = "validation_failed"
	CodeUnauthenticated    = "unauthenticated"
	CodeInvalidCredentials = "invalid_credentials"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeTooLarge           = "payload_too_large"
	CodeRateLimited        = "rate_limited"
	CodeInternal           = "internal_error"
)

// Todo is the shared wire representation of a todo task.
type Todo struct {
	ID          string     `json:"id"`
	Date        string     `json:"date"` // YYYY-MM-DD
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Done        bool       `json:"done"`
	Priority    uint8      `json:"priority"`
	RemindAt    *time.Time `json:"remind_at,omitempty"`
	RemindedAt  *time.Time `json:"reminded_at,omitempty"`
	RepeatRule  string     `json:"repeat_rule"` // "none", "daily", "weekly"
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// CreateTodoRequest is the payload for POST /api/v1/todos.
type CreateTodoRequest struct {
	Date        string     `json:"date,omitempty"` // YYYY-MM-DD, defaults to today
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Priority    *uint8     `json:"priority,omitempty"` // 1 (high), 2 (medium), 3 (low); defaults to 2
	RemindAt    *time.Time `json:"remind_at,omitempty"`
	RepeatRule  string     `json:"repeat_rule,omitempty"` // "none", "daily", "weekly"
}

// UpdateTodoRequest is the payload for PATCH /api/v1/todos/{id}.
type UpdateTodoRequest struct {
	Title         *string    `json:"title,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Done          *bool      `json:"done,omitempty"`
	Priority      *uint8     `json:"priority,omitempty"`
	Date          *string    `json:"date,omitempty"` // YYYY-MM-DD
	RemindAt      *time.Time `json:"remind_at,omitempty"`
	ClearRemindAt bool       `json:"clear_remind_at,omitempty"`
	RepeatRule    *string    `json:"repeat_rule,omitempty"`
}

// BatchCreateTodoRequest defines parameters for generating recurring or bulk todos.
type BatchCreateTodoRequest struct {
	// Mode 1: Range generation (e.g. setiap Senin sampai Jumat)
	Title        string   `json:"title,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Priority     *uint8   `json:"priority,omitempty"`
	StartDate    string   `json:"start_date,omitempty"` // YYYY-MM-DD
	EndDate      string   `json:"end_date,omitempty"`   // YYYY-MM-DD
	DaysOfWeek   []int    `json:"days_of_week,omitempty"` // 1=Mon .. 7=Sun
	WeekdaysOnly bool     `json:"weekdays_only,omitempty"` // shortcut for Mon-Fri
	RemindTime   *string  `json:"remind_time,omitempty"`  // "HH:MM" e.g. "09:00"
	RepeatRule   string   `json:"repeat_rule,omitempty"`

	// Mode 2: Explicit list of todos
	Todos []CreateTodoRequest `json:"todos,omitempty"`
	Items []CreateTodoRequest `json:"items,omitempty"`
}

// BatchCreateTodoResponse reports how many todos were created.
type BatchCreateTodoResponse struct {
	CreatedCount int    `json:"created_count"`
	Todos        []Todo `json:"todos"`
}

// TodoResponse wraps a single Todo.
type TodoResponse struct {
	Todo Todo `json:"todo"`
}

// TodoListResponse wraps a slice of Todo items and the server timestamp.
type TodoListResponse struct {
	Todos      []Todo    `json:"todos"`
	ServerTime time.Time `json:"server_time"`
}

// RolloverRequest is the payload for POST /api/v1/todos/rollover.
type RolloverRequest struct {
	FromDate string `json:"from_date"` // YYYY-MM-DD
	ToDate   string `json:"to_date"`   // YYYY-MM-DD
}

// RolloverResponse reports how many incomplete todos were moved.
type RolloverResponse struct {
	MovedCount int64 `json:"moved_count"`
}

// JournalEntry is the shared wire representation of a daily journal entry.
type JournalEntry struct {
	ID        string     `json:"id"`
	Date      string     `json:"date"` // YYYY-MM-DD
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Tags      string     `json:"tags"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// SaveJournalRequest is the payload for PUT /api/v1/journal.
type SaveJournalRequest struct {
	ID    string `json:"id,omitempty"` // If set, updates existing note. If empty, creates new note.
	Date  string `json:"date"`         // YYYY-MM-DD
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
	Tags  string `json:"tags,omitempty"`
}

// JournalResponse wraps a single JournalEntry and list of entries for a date.
type JournalResponse struct {
	Entry   *JournalEntry  `json:"entry"`             // Primary / first entry for backward compatibility
	Entries []JournalEntry `json:"entries,omitempty"` // All notes for the date
}

// JournalListResponse wraps a list of journal entries for sync.
type JournalListResponse struct {
	Entries    []JournalEntry `json:"entries"`
	ServerTime time.Time      `json:"server_time"`
}

// JournalSearchMatch represents a matching entry in fulltext search.
type JournalSearchMatch struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	Tags      string    `json:"tags"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JournalSearchResponse is the response of GET /api/v1/journal/search.
type JournalSearchResponse struct {
	Query   string               `json:"query"`
	Matches []JournalSearchMatch `json:"matches"`
}

// Room represents a public channel or a private DM room.
type Room struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	IsDM        bool      `json:"is_dm"`
	CreatedAt   time.Time `json:"created_at"`
	UnreadCount int       `json:"unread_count"`
	LastMessage *Message  `json:"last_message,omitempty"`
}

// Message is a chat message in a room.
type Message struct {
	ID        int64     `json:"id"`
	RoomID    int64     `json:"room_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRoomRequest is the payload for POST /api/v1/rooms.
type CreateRoomRequest struct {
	Name string `json:"name"`
}

// CreateDMRequest is the payload for POST /api/v1/rooms/dm.
type CreateDMRequest struct {
	Username string `json:"username"`
}

// SendMessageRequest is the payload for POST /api/v1/rooms/{id}/messages.
type SendMessageRequest struct {
	Body string `json:"body"`
}

// MarkReadRequest is the payload for POST /api/v1/rooms/{id}/read.
type MarkReadRequest struct {
	LastReadMessageID int64 `json:"last_read_message_id"`
}

// RoomListResponse wraps a list of rooms.
type RoomListResponse struct {
	Rooms []Room `json:"rooms"`
}

// RoomResponse wraps a single room.
type RoomResponse struct {
	Room Room `json:"room"`
}

// MessageListResponse wraps messages with pagination metadata.
type MessageListResponse struct {
	Messages []Message `json:"messages"`
	HasMore  bool      `json:"has_more"`
}

// MessageResponse wraps a single message.
type MessageResponse struct {
	Message Message `json:"message"`
}

// WSEvent is the standard envelope for WebSocket communication.
type WSEvent struct {
	Type    string `json:"type"` // "message", "read", "presence", "error"
	Payload any    `json:"payload,omitempty"`
}

// WSPresence is emitted when a user's online state changes.
type WSPresence struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

// WSReminder is emitted when a task reminder is triggered.
type WSReminder struct {
	TodoID   string     `json:"todo_id"`
	Date     string     `json:"date"` // YYYY-MM-DD
	Title    string     `json:"title"`
	Priority uint8      `json:"priority"`
	RemindAt *time.Time `json:"remind_at,omitempty"`
}



