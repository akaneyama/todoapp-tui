// Package handler wires the HTTP API: routing, middleware and handlers.
package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"todoapp/internal/models"
	"todoapp/internal/server/auth"
	"todoapp/internal/server/httpx"
	"todoapp/internal/server/middleware"
	"todoapp/internal/server/store"
	wsHub "todoapp/internal/server/ws"
)

// Pinger reports backend health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// TodoStore defines the data access methods needed by the todo handlers.
type TodoStore interface {
	CreateTodo(ctx context.Context, t store.Todo) error
	CreateTodosBatch(ctx context.Context, todos []store.Todo) error
	GetTodo(ctx context.Context, userID int64, id string) (store.Todo, error)
	ListTodosByDate(ctx context.Context, userID int64, date time.Time) ([]store.Todo, error)
	ListTodosSince(ctx context.Context, userID int64, since time.Time) ([]store.Todo, error)
	UpdateTodo(ctx context.Context, t store.Todo) error
	DeleteTodo(ctx context.Context, userID int64, id string, now time.Time) error
	RolloverTodos(ctx context.Context, userID int64, fromDate, toDate, now time.Time) (int64, error)
}

// JournalStore defines the data access methods needed by the journal handlers.
type JournalStore interface {
	ListJournalsByDate(ctx context.Context, userID int64, date time.Time) ([]store.JournalEntry, error)
	GetJournalByDate(ctx context.Context, userID int64, date time.Time) (store.JournalEntry, error)
	UpsertJournal(ctx context.Context, j store.JournalEntry) (store.JournalEntry, error)
	DeleteJournal(ctx context.Context, userID int64, date time.Time, now time.Time) error
	DeleteJournalByID(ctx context.Context, userID int64, id string, now time.Time) error
	SearchJournal(ctx context.Context, userID int64, searchQuery string) ([]models.JournalSearchMatch, error)
	ListJournalSince(ctx context.Context, userID int64, since time.Time) ([]store.JournalEntry, error)
}

// ChatStore defines data access methods for rooms and messages.
type ChatStore interface {
	CreateRoom(ctx context.Context, name string, isDM bool, dmKey *string) (store.Room, error)
	GetRoom(ctx context.Context, roomID int64) (store.Room, error)
	GetOrCreateDMRoom(ctx context.Context, userA, userB int64) (store.Room, error)
	AddRoomMember(ctx context.Context, roomID, userID int64) error
	IsRoomMember(ctx context.Context, roomID, userID int64) (bool, error)
	ListRoomMembers(ctx context.Context, roomID int64) ([]int64, error)
	ListUserRooms(ctx context.Context, userID int64) ([]models.Room, error)
	CreateMessage(ctx context.Context, roomID, userID int64, body string) (models.Message, error)
	ListMessages(ctx context.Context, roomID int64, beforeID int64, limit int) ([]models.Message, bool, error)
	UpdateReadState(ctx context.Context, userID, roomID, lastReadID int64) error
}

// UserLookup provides user lookup by username.
type UserLookup interface {
	GetUserByUsername(ctx context.Context, username string) (store.User, error)
}

// RateLimit configures a token bucket.
type RateLimit struct {
	PerMinute float64
	Burst     int
}

// Deps are the dependencies of the API router.
type Deps struct {
	Auth         *auth.Service
	Todos        TodoStore
	Journal      JournalStore
	Chat         ChatStore
	UserLookup   UserLookup
	Hub          *wsHub.Hub
	Health       Pinger
	Logger       *slog.Logger
	TrustProxy   bool
	MaxBodyBytes int64
	// AuthRate limits /auth/login and /auth/register per client IP and
	// /auth/login additionally per username.
	AuthRate RateLimit
}

type api struct {
	auth          *auth.Service
	todos         TodoStore
	journal       JournalStore
	chat          ChatStore
	userLookup    UserLookup
	hub           *wsHub.Hub
	health        Pinger
	log           *slog.Logger
	usernameLimit *middleware.KeyedLimiter
}

// NewRouter builds the HTTP handler for the API.
func NewRouter(d Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	a := &api{
		auth:          d.Auth,
		todos:         d.Todos,
		journal:       d.Journal,
		chat:          d.Chat,
		userLookup:    d.UserLookup,
		hub:           d.Hub,
		health:        d.Health,
		log:           d.Logger,
		usernameLimit: middleware.NewKeyedLimiter(d.AuthRate.PerMinute, d.AuthRate.Burst),
	}
	ipLimit := middleware.NewKeyedLimiter(d.AuthRate.PerMinute, d.AuthRate.Burst)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	if d.TrustProxy {
		r.Use(chimw.RealIP)
	}
	r.Use(middleware.Logger(d.Logger))
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxBody(d.MaxBodyBytes))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusMethodNotAllowed, models.CodeBadRequest, "method not allowed")
	})

	r.Get("/healthz", a.handleHealth)
	r.Get("/ws", a.handleWS)

	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimitByIP(ipLimit))
			r.Post("/auth/register", a.handleRegister)
			r.Post("/auth/login", a.handleLogin)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(d.Auth, d.Logger))
			r.Post("/auth/logout", a.handleLogout)
			r.Get("/me", a.handleMe)

			// Todos
			r.Get("/todos", a.handleListTodos)
			r.Post("/todos", a.handleCreateTodo)
			r.Post("/todos/batch", a.handleBatchCreateTodos)
			r.Get("/todos/{id}", a.handleGetTodo)
			r.Patch("/todos/{id}", a.handleUpdateTodo)
			r.Delete("/todos/{id}", a.handleDeleteTodo)
			r.Post("/todos/rollover", a.handleRolloverTodos)

			// Journal
			r.Get("/journal", a.handleGetJournal)
			r.Put("/journal", a.handleSaveJournal)
			r.Delete("/journal", a.handleDeleteJournal)
			r.Get("/journal/search", a.handleSearchJournal)

			// Chat
			r.Get("/rooms", a.handleListRooms)
			r.Post("/rooms", a.handleCreateRoom)
			r.Post("/rooms/dm", a.handleCreateDM)
			r.Post("/rooms/{id}/join", a.handleJoinRoom)
			r.Get("/rooms/{id}/messages", a.handleListMessages)
			r.Post("/rooms/{id}/messages", a.handleSendMessage)
			r.Post("/rooms/{id}/read", a.handleMarkRead)
		})
	})
	return r
}

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.health.Ping(ctx); err != nil {
		a.log.WarnContext(ctx, "health check failed", "err", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// internalError logs err with request context and returns a generic 500.
func (a *api) internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	a.log.ErrorContext(r.Context(), msg, "err", err, "request_id", chimw.GetReqID(r.Context()))
	httpx.WriteError(w, http.StatusInternalServerError, models.CodeInternal, "internal error")
}
