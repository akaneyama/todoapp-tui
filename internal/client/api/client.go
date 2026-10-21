package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"todoapp/internal/models"
)

// Error wraps server-returned APIError with HTTP status code.
type Error struct {
	StatusCode int
	Code       string
	Message    string
	Field      string
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Field, e.Message, e.Code)
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// Client interacts with the todoapp HTTP REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	token      string
}

// New creates a new API client.
func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://192.168.96.159:8080"
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// SetToken updates the bearer authentication token.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// Token retrieves the current bearer token.
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	fullURL := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var errEnv models.ErrorResponse
		if err := json.Unmarshal(respBytes, &errEnv); err == nil && errEnv.Error.Message != "" {
			return &Error{
				StatusCode: res.StatusCode,
				Code:       errEnv.Error.Code,
				Message:    errEnv.Error.Message,
				Field:      errEnv.Error.Field,
			}
		}
		return &Error{
			StatusCode: res.StatusCode,
			Code:       "http_error",
			Message:    fmt.Sprintf("server returned HTTP %d: %s", res.StatusCode, string(respBytes)),
		}
	}

	if out != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

// Register registers a new account.
func (c *Client) Register(ctx context.Context, username, password, timezone string) (models.User, error) {
	req := models.RegisterRequest{
		Username: username,
		Password: password,
		Timezone: timezone,
	}
	var resp models.UserResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/auth/register", req, &resp); err != nil {
		return models.User{}, err
	}
	return resp.User, nil
}

// Login authenticates an account and stores the token.
func (c *Client) Login(ctx context.Context, username, password, timezone string) (models.LoginResponse, error) {
	req := models.LoginRequest{
		Username: username,
		Password: password,
		Timezone: timezone,
	}
	var resp models.LoginResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", req, &resp); err != nil {
		return models.LoginResponse{}, err
	}
	c.SetToken(resp.Token)
	return resp, nil
}

// Logout invalidates the active session.
func (c *Client) Logout(ctx context.Context) error {
	if err := c.do(ctx, http.MethodPost, "/api/v1/auth/logout", nil, nil); err != nil {
		return err
	}
	c.SetToken("")
	return nil
}

// Me retrieves the authenticated user's profile.
func (c *Client) Me(ctx context.Context) (models.User, error) {
	var resp models.UserResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/me", nil, &resp); err != nil {
		return models.User{}, err
	}
	return resp.User, nil
}

// ListTodos queries todos by date or since timestamp.
func (c *Client) ListTodos(ctx context.Context, date string, since *time.Time) (models.TodoListResponse, error) {
	params := url.Values{}
	if since != nil && !since.IsZero() {
		params.Set("since", since.Format(time.RFC3339Nano))
	} else if date != "" {
		params.Set("date", date)
	}

	path := "/api/v1/todos"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp models.TodoListResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return models.TodoListResponse{}, err
	}
	return resp, nil
}

// CreateTodo creates a new task.
func (c *Client) CreateTodo(ctx context.Context, req models.CreateTodoRequest) (models.Todo, error) {
	var resp models.TodoResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/todos", req, &resp); err != nil {
		return models.Todo{}, err
	}
	return resp.Todo, nil
}

// CreateBatchTodos creates multiple tasks at once or generates tasks across a date range.
func (c *Client) CreateBatchTodos(ctx context.Context, req models.BatchCreateTodoRequest) (models.BatchCreateTodoResponse, error) {
	var resp models.BatchCreateTodoResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/todos/batch", req, &resp); err != nil {
		return models.BatchCreateTodoResponse{}, err
	}
	return resp, nil
}

// GetTodo retrieves a single task by ID.
func (c *Client) GetTodo(ctx context.Context, id string) (models.Todo, error) {
	var resp models.TodoResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/todos/"+id, nil, &resp); err != nil {
		return models.Todo{}, err
	}
	return resp.Todo, nil
}

// UpdateTodo modifies an existing task.
func (c *Client) UpdateTodo(ctx context.Context, id string, req models.UpdateTodoRequest) (models.Todo, error) {
	var resp models.TodoResponse
	if err := c.do(ctx, http.MethodPatch, "/api/v1/todos/"+id, req, &resp); err != nil {
		return models.Todo{}, err
	}
	return resp.Todo, nil
}

// DeleteTodo deletes a task by ID.
func (c *Client) DeleteTodo(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/todos/"+id, nil, nil)
}

// RolloverTodos moves uncompleted tasks from fromDate to toDate.
func (c *Client) RolloverTodos(ctx context.Context, fromDate, toDate string) (models.RolloverResponse, error) {
	req := models.RolloverRequest{
		FromDate: fromDate,
		ToDate:   toDate,
	}
	var resp models.RolloverResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/todos/rollover", req, &resp); err != nil {
		return models.RolloverResponse{}, err
	}
	return resp, nil
}

// GetJournal retrieves the journal entries for a specific date (YYYY-MM-DD).
// Returns a JournalResponse with .Entries and the primary .Entry.
func (c *Client) GetJournal(ctx context.Context, date string) (*models.JournalResponse, error) {
	var resp models.JournalResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/journal?date="+date, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SaveJournal creates or updates a journal entry for a date.
func (c *Client) SaveJournal(ctx context.Context, req models.SaveJournalRequest) (*models.JournalEntry, error) {
	var resp models.JournalResponse
	if err := c.do(ctx, http.MethodPut, "/api/v1/journal", req, &resp); err != nil {
		return nil, err
	}
	return resp.Entry, nil
}

// DeleteJournal deletes all journal entries for a date.
func (c *Client) DeleteJournal(ctx context.Context, date string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/journal?date="+date, nil, nil)
}

// DeleteJournalEntry deletes a specific journal entry by its ID.
func (c *Client) DeleteJournalEntry(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/journal?id="+id, nil, nil)
}

// SearchJournal performs a fulltext search across journal entries.
func (c *Client) SearchJournal(ctx context.Context, query string) (models.JournalSearchResponse, error) {
	var resp models.JournalSearchResponse
	path := "/api/v1/journal/search?q=" + url.QueryEscape(query)
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return models.JournalSearchResponse{}, err
	}
	return resp, nil
}

// ListJournalSince fetches journal updates since a given timestamp for incremental sync.
func (c *Client) ListJournalSince(ctx context.Context, since time.Time) (models.JournalListResponse, error) {
	var resp models.JournalListResponse
	path := "/api/v1/journal?since=" + url.QueryEscape(since.Format(time.RFC3339Nano))
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return models.JournalListResponse{}, err
	}
	return resp, nil
}

// ListRooms retrieves all chat rooms the user is a member of.
func (c *Client) ListRooms(ctx context.Context) ([]models.Room, error) {
	var resp models.RoomListResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/rooms", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Rooms, nil
}

// CreateRoom creates a new public channel.
func (c *Client) CreateRoom(ctx context.Context, name string) (models.Room, error) {
	var resp models.RoomResponse
	req := models.CreateRoomRequest{Name: name}
	if err := c.do(ctx, http.MethodPost, "/api/v1/rooms", req, &resp); err != nil {
		return models.Room{}, err
	}
	return resp.Room, nil
}

// CreateDM creates or opens a direct message conversation with a user.
func (c *Client) CreateDM(ctx context.Context, username string) (models.Room, error) {
	var resp models.RoomResponse
	req := models.CreateDMRequest{Username: username}
	if err := c.do(ctx, http.MethodPost, "/api/v1/rooms/dm", req, &resp); err != nil {
		return models.Room{}, err
	}
	return resp.Room, nil
}

// JoinRoom joins an existing public channel.
func (c *Client) JoinRoom(ctx context.Context, roomID int64) error {
	path := fmt.Sprintf("/api/v1/rooms/%d/join", roomID)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// ListMessages loads messages in a room with pagination.
func (c *Client) ListMessages(ctx context.Context, roomID int64, beforeID int64, limit int) (models.MessageListResponse, error) {
	path := fmt.Sprintf("/api/v1/rooms/%d/messages", roomID)
	params := url.Values{}
	if beforeID > 0 {
		params.Set("before_id", strconv.FormatInt(beforeID, 10))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp models.MessageListResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return models.MessageListResponse{}, err
	}
	return resp, nil
}

// SendMessage sends a text message into a room.
func (c *Client) SendMessage(ctx context.Context, roomID int64, body string) (models.Message, error) {
	path := fmt.Sprintf("/api/v1/rooms/%d/messages", roomID)
	req := models.SendMessageRequest{Body: body}
	var resp models.MessageResponse
	if err := c.do(ctx, http.MethodPost, path, req, &resp); err != nil {
		return models.Message{}, err
	}
	return resp.Message, nil
}

// MarkRead marks messages in a room up to lastReadID as read.
func (c *Client) MarkRead(ctx context.Context, roomID int64, lastReadID int64) error {
	path := fmt.Sprintf("/api/v1/rooms/%d/read", roomID)
	req := models.MarkReadRequest{LastReadMessageID: lastReadID}
	return c.do(ctx, http.MethodPost, path, req, nil)
}
