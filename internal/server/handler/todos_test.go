package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"todoapp/internal/models"
)

func registerAndLogin(t *testing.T, ts *testServer, username, password string) (string, models.User) {
	t.Helper()
	res, body := ts.do(http.MethodPost, "/api/v1/auth/register", "", models.RegisterRequest{
		Username: username,
		Password: password,
		Timezone: "UTC",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register failed (%d): %s", res.StatusCode, body)
	}

	res, body = ts.do(http.MethodPost, "/api/v1/auth/login", "", models.LoginRequest{
		Username: username,
		Password: password,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login failed (%d): %s", res.StatusCode, body)
	}
	var loginResp models.LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		t.Fatalf("unmarshal login: %v", err)
	}
	return loginResp.Token, loginResp.User
}

func TestTodoCRUD(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "alice", "alice-password-123")

	// 1. Create Todo
	prio := uint8(1)
	createReq := models.CreateTodoRequest{
		Date:       "2026-10-20",
		Title:      "Buy groceries",
		Priority:   &prio,
		RepeatRule: "daily",
	}
	res, body := ts.do(http.MethodPost, "/api/v1/todos", token, createReq)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.StatusCode, body)
	}
	var created models.TodoResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("unmarshal created todo: %v", err)
	}
	if created.Todo.Title != "Buy groceries" || created.Todo.Priority != 1 || created.Todo.RepeatRule != "daily" {
		t.Fatalf("unexpected todo fields: %+v", created.Todo)
	}
	todoID := created.Todo.ID

	// 2. Get Todo by ID
	res, body = ts.do(http.MethodGet, "/api/v1/todos/"+todoID, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var fetched models.TodoResponse
	if err := json.Unmarshal(body, &fetched); err != nil {
		t.Fatalf("unmarshal fetched todo: %v", err)
	}
	if fetched.Todo.ID != todoID {
		t.Fatalf("expected ID %s, got %s", todoID, fetched.Todo.ID)
	}

	// 3. List Todos by Date
	res, body = ts.do(http.MethodGet, "/api/v1/todos?date=2026-10-20", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var listResp models.TodoListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp.Todos) != 1 || listResp.Todos[0].ID != todoID {
		t.Fatalf("expected 1 todo with ID %s, got %+v", todoID, listResp.Todos)
	}

	// 4. Update Todo (toggle done & edit title)
	done := true
	newTitle := "Buy organic groceries"
	res, body = ts.do(http.MethodPatch, "/api/v1/todos/"+todoID, token, models.UpdateTodoRequest{
		Done:  &done,
		Title: &newTitle,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.StatusCode, body)
	}
	var updated models.TodoResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if !updated.Todo.Done || updated.Todo.Title != newTitle {
		t.Fatalf("update failed: %+v", updated.Todo)
	}

	// 5. Delete Todo
	res, _ = ts.do(http.MethodDelete, "/api/v1/todos/"+todoID, token, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}

	// 6. Verify Get returns 404
	res, _ = ts.do(http.MethodGet, "/api/v1/todos/"+todoID, token, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.StatusCode)
	}
}

func TestTodoAuthorization(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "alice2", "alice-password-123")
	tokenBob, _ := registerAndLogin(t, ts, "bob2", "bob-password-123")

	// Alice creates a todo
	res, body := ts.do(http.MethodPost, "/api/v1/todos", tokenAlice, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Alice secret task",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("alice create failed (%d): %s", res.StatusCode, body)
	}
	var aliceTodo models.TodoResponse
	json.Unmarshal(body, &aliceTodo)
	todoID := aliceTodo.Todo.ID

	// Bob attempts to read Alice's todo
	res, _ = ts.do(http.MethodGet, "/api/v1/todos/"+todoID, tokenBob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unauthorized read, got %d", res.StatusCode)
	}

	// Bob attempts to update Alice's todo
	done := true
	res, _ = ts.do(http.MethodPatch, "/api/v1/todos/"+todoID, tokenBob, models.UpdateTodoRequest{
		Done: &done,
	})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unauthorized update, got %d", res.StatusCode)
	}

	// Bob attempts to delete Alice's todo
	res, _ = ts.do(http.MethodDelete, "/api/v1/todos/"+todoID, tokenBob, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unauthorized delete, got %d", res.StatusCode)
	}

	// Bob lists todos for the same date -> should be empty
	res, body = ts.do(http.MethodGet, "/api/v1/todos?date=2026-10-20", tokenBob, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var bobList models.TodoListResponse
	json.Unmarshal(body, &bobList)
	if len(bobList.Todos) != 0 {
		t.Fatalf("expected 0 todos for Bob, got %d", len(bobList.Todos))
	}
}

func TestTodoSyncSince(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "syncuser", "sync-password-123")

	// Create Task 1
	res, body := ts.do(http.MethodPost, "/api/v1/todos", token, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Task 1",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create 1: %s", body)
	}
	var t1 models.TodoResponse
	json.Unmarshal(body, &t1)

	// Sleep briefly so timestamps differ
	time.Sleep(10 * time.Millisecond)
	sinceAnchor := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(10 * time.Millisecond)

	// Create Task 2
	res, body = ts.do(http.MethodPost, "/api/v1/todos", token, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Task 2",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create 2: %s", body)
	}
	var t2 models.TodoResponse
	json.Unmarshal(body, &t2)

	// Sync since anchor -> should only return Task 2
	res, body = ts.do(http.MethodGet, "/api/v1/todos?since="+sinceAnchor, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sync since failed (%d): %s", res.StatusCode, body)
	}
	var syncResp models.TodoListResponse
	json.Unmarshal(body, &syncResp)
	if len(syncResp.Todos) != 1 || syncResp.Todos[0].ID != t2.Todo.ID {
		t.Fatalf("expected only Task 2 in sync, got %+v", syncResp.Todos)
	}

	// Delete Task 2
	time.Sleep(10 * time.Millisecond)
	sinceBeforeDelete := time.Now().UTC().Format(time.RFC3339Nano)
	time.Sleep(10 * time.Millisecond)

	ts.do(http.MethodDelete, "/api/v1/todos/"+t2.Todo.ID, token, nil)

	// Sync since before delete -> should include Task 2 with DeletedAt set
	res, body = ts.do(http.MethodGet, "/api/v1/todos?since="+sinceBeforeDelete, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sync deleted failed (%d): %s", res.StatusCode, body)
	}
	json.Unmarshal(body, &syncResp)
	if len(syncResp.Todos) != 1 || syncResp.Todos[0].DeletedAt == nil {
		t.Fatalf("expected 1 soft-deleted todo in sync, got %+v", syncResp.Todos)
	}
}

func TestTodoRollover(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "rolloveruser", "roll-password-123")

	// Task 1: undone
	ts.do(http.MethodPost, "/api/v1/todos", token, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Undone Task 1",
	})
	// Task 2: undone
	ts.do(http.MethodPost, "/api/v1/todos", token, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Undone Task 2",
	})
	// Task 3: done
	res, body := ts.do(http.MethodPost, "/api/v1/todos", token, models.CreateTodoRequest{
		Date:  "2026-10-20",
		Title: "Done Task 3",
	})
	var t3 models.TodoResponse
	json.Unmarshal(body, &t3)
	done := true
	ts.do(http.MethodPatch, "/api/v1/todos/"+t3.Todo.ID, token, models.UpdateTodoRequest{
		Done: &done,
	})

	// Rollover from 2026-10-20 to 2026-10-21
	res, body = ts.do(http.MethodPost, "/api/v1/todos/rollover", token, models.RolloverRequest{
		FromDate: "2026-10-20",
		ToDate:   "2026-10-21",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rollover failed (%d): %s", res.StatusCode, body)
	}
	var rollResp models.RolloverResponse
	json.Unmarshal(body, &rollResp)
	if rollResp.MovedCount != 2 {
		t.Fatalf("expected 2 tasks moved, got %d", rollResp.MovedCount)
	}

	// 2026-10-20 should now only have 1 task (Done Task 3)
	res, body = ts.do(http.MethodGet, "/api/v1/todos?date=2026-10-20", token, nil)
	var list20 models.TodoListResponse
	json.Unmarshal(body, &list20)
	if len(list20.Todos) != 1 || !list20.Todos[0].Done {
		t.Fatalf("expected 1 done task on 2026-10-20, got %+v", list20.Todos)
	}

	// 2026-10-21 should have the 2 moved tasks
	res, body = ts.do(http.MethodGet, "/api/v1/todos?date=2026-10-21", token, nil)
	var list21 models.TodoListResponse
	json.Unmarshal(body, &list21)
	if len(list21.Todos) != 2 {
		t.Fatalf("expected 2 tasks on 2026-10-21, got %d", len(list21.Todos))
	}
}

func TestTodoValidation(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "validuser", "valid-password-123")

	cases := []struct {
		name      string
		body      models.CreateTodoRequest
		wantField string
	}{
		{
			name:      "empty title",
			body:      models.CreateTodoRequest{Title: "   "},
			wantField: "title",
		},
		{
			name: "invalid priority high",
			body: func() models.CreateTodoRequest {
				p := uint8(4)
				return models.CreateTodoRequest{Title: "Task", Priority: &p}
			}(),
			wantField: "priority",
		},
		{
			name: "invalid priority zero",
			body: func() models.CreateTodoRequest {
				p := uint8(0)
				return models.CreateTodoRequest{Title: "Task", Priority: &p}
			}(),
			wantField: "priority",
		},
		{
			name:      "invalid repeat rule",
			body:      models.CreateTodoRequest{Title: "Task", RepeatRule: "yearly"},
			wantField: "repeat_rule",
		},
		{
			name:      "invalid date",
			body:      models.CreateTodoRequest{Title: "Task", Date: "2026-99-99"},
			wantField: "date",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, body := ts.do(http.MethodPost, "/api/v1/todos", token, tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", res.StatusCode, body)
			}
			var errResp models.ErrorResponse
			if err := json.Unmarshal(body, &errResp); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if errResp.Error.Code != models.CodeValidation || errResp.Error.Field != tc.wantField {
				t.Fatalf("expected validation error on %s, got %+v", tc.wantField, errResp)
			}
		})
	}
}

func TestBatchCreateTodos_Weekdays(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "batchuser", "batch-pass-123")

	// Sunday 2026-10-18 to Saturday 2026-10-24 -> weekdays_only should generate 5 tasks (Mon-Fri)
	desc := "Jadwal posting konten harian"
	rt := "09:00"
	req := models.BatchCreateTodoRequest{
		Title:        "Posting Konten",
		Description:  &desc,
		StartDate:    "2026-10-18",
		EndDate:      "2026-10-24",
		WeekdaysOnly: true,
		RemindTime:   &rt,
	}

	res, body := ts.do(http.MethodPost, "/api/v1/todos/batch", token, req)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.StatusCode, body)
	}

	var batchResp models.BatchCreateTodoResponse
	if err := json.Unmarshal(body, &batchResp); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}
	if batchResp.CreatedCount != 5 {
		t.Fatalf("expected 5 created todos, got %d", batchResp.CreatedCount)
	}
	for _, todo := range batchResp.Todos {
		if todo.Title != "Posting Konten" {
			t.Errorf("expected title 'Posting Konten', got %s", todo.Title)
		}
		if todo.Description == nil || *todo.Description != desc {
			t.Errorf("expected description %q, got %+v", desc, todo.Description)
		}
		if todo.RemindAt == nil {
			t.Errorf("expected remind_at to be set, got nil")
		}
		parsedDate, err := time.Parse(time.DateOnly, todo.Date)
		if err != nil {
			t.Fatalf("invalid date format: %v", err)
		}
		wd := parsedDate.Weekday()
		if wd == time.Sunday || wd == time.Saturday {
			t.Errorf("unexpected weekend date in weekdays_only batch: %s (%s)", todo.Date, wd)
		}
	}
}

func TestBatchCreateTodos_ItemsList(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "bulkuser", "bulk-pass-123")

	d1 := "Catatan 1"
	d2 := "Catatan 2"
	req := models.BatchCreateTodoRequest{
		Items: []models.CreateTodoRequest{
			{Title: "Task 1", Description: &d1, Date: "2026-10-20"},
			{Title: "Task 2", Description: &d2, Date: "2026-10-21"},
		},
	}

	res, body := ts.do(http.MethodPost, "/api/v1/todos/batch", token, req)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.StatusCode, body)
	}

	var batchResp models.BatchCreateTodoResponse
	if err := json.Unmarshal(body, &batchResp); err != nil {
		t.Fatalf("unmarshal batch response: %v", err)
	}
	if batchResp.CreatedCount != 2 {
		t.Fatalf("expected 2 created todos, got %d", batchResp.CreatedCount)
	}
}
