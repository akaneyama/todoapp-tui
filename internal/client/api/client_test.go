package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"todoapp/internal/client/api"
	"todoapp/internal/models"
)

func TestClientMethods(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req models.LoginRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Username == "alice" && req.Password == "password123" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.LoginResponse{
				Token:     "test-token",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				User:      models.User{ID: 1, Username: "alice"},
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Error: models.APIError{Code: models.CodeInvalidCredentials, Message: "invalid credentials"},
		})
	})

	mux.HandleFunc("/api/v1/todos", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(models.TodoListResponse{
				Todos: []models.Todo{
					{ID: "t-1", Title: "Test Task", Date: "2026-10-20"},
				},
				ServerTime: time.Now(),
			})
		case http.MethodPost:
			var req models.CreateTodoRequest
			json.NewDecoder(r.Body).Decode(&req)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(models.TodoResponse{
				Todo: models.Todo{ID: "t-new", Title: req.Title, Date: req.Date},
			})
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := api.New(srv.URL, "")
	ctx := context.Background()

	// 1. Failed Login
	_, err := c.Login(ctx, "alice", "wrongpass", "UTC")
	if err == nil {
		t.Fatal("expected login failure")
	}

	// 2. Successful Login
	resp, err := c.Login(ctx, "alice", "password123", "UTC")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token != "test-token" || c.Token() != "test-token" {
		t.Fatalf("token mismatch")
	}

	// 3. List Todos
	list, err := c.ListTodos(ctx, "2026-10-20", nil)
	if err != nil {
		t.Fatalf("list todos: %v", err)
	}
	if len(list.Todos) != 1 || list.Todos[0].ID != "t-1" {
		t.Fatalf("unexpected todos: %+v", list.Todos)
	}

	// 4. Create Todo
	created, err := c.CreateTodo(ctx, models.CreateTodoRequest{
		Title: "New Task",
		Date:  "2026-10-20",
	})
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	if created.Title != "New Task" {
		t.Fatalf("title mismatch: %s", created.Title)
	}
}
