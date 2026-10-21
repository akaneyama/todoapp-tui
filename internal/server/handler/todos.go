package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"todoapp/internal/models"
	"todoapp/internal/server/httpx"
	"todoapp/internal/server/middleware"
	"todoapp/internal/server/store"
)

func (a *api) handleListTodos(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	q := r.URL.Query()
	sinceStr := q.Get("since")
	if sinceStr != "" {
		sinceTime, err := time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			sinceTime, err = time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				httpx.WriteFieldError(w, "since", "must be RFC3339 format")
				return
			}
		}
		list, err := a.todos.ListTodosSince(r.Context(), u.ID, sinceTime)
		if err != nil {
			a.internalError(w, r, "list todos since", err)
			return
		}
		resp := models.TodoListResponse{
			Todos:      make([]models.Todo, 0, len(list)),
			ServerTime: time.Now().UTC(),
		}
		for _, item := range list {
			resp.Todos = append(resp.Todos, item.Public())
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
		return
	}

	dateStr := q.Get("date")
	var targetDate time.Time
	if dateStr != "" {
		d, err := time.Parse(time.DateOnly, dateStr)
		if err != nil {
			httpx.WriteFieldError(w, "date", "must be YYYY-MM-DD format")
			return
		}
		targetDate = d
	} else {
		targetDate = userToday(u.Timezone)
	}

	list, err := a.todos.ListTodosByDate(r.Context(), u.ID, targetDate)
	if err != nil {
		a.internalError(w, r, "list todos by date", err)
		return
	}
	resp := models.TodoListResponse{
		Todos:      make([]models.Todo, 0, len(list)),
		ServerTime: time.Now().UTC(),
	}
	for _, item := range list {
		resp.Todos = append(resp.Todos, item.Public())
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *api) handleCreateTodo(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.CreateTodoRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		httpx.WriteFieldError(w, "title", "title is required")
		return
	}
	if len(title) > 500 {
		httpx.WriteFieldError(w, "title", "title cannot exceed 500 characters")
		return
	}

	priority := uint8(2)
	if req.Priority != nil {
		if *req.Priority < 1 || *req.Priority > 3 {
			httpx.WriteFieldError(w, "priority", "priority must be 1, 2, or 3")
			return
		}
		priority = *req.Priority
	}

	repeatRule := "none"
	if req.RepeatRule != "" {
		switch req.RepeatRule {
		case "none", "daily", "weekly":
			repeatRule = req.RepeatRule
		default:
			httpx.WriteFieldError(w, "repeat_rule", "repeat_rule must be none, daily, or weekly")
			return
		}
	}

	var targetDate time.Time
	if req.Date != "" {
		d, err := time.Parse(time.DateOnly, req.Date)
		if err != nil {
			httpx.WriteFieldError(w, "date", "must be YYYY-MM-DD format")
			return
		}
		targetDate = d
	} else {
		targetDate = userToday(u.Timezone)
	}

	now := time.Now().UTC()
	t := store.Todo{
		ID:          uuid.New().String(),
		UserID:      u.ID,
		Date:        targetDate,
		Title:       title,
		Description: req.Description,
		Done:        false,
		Priority:    priority,
		RemindAt:    req.RemindAt,
		RepeatRule:  repeatRule,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := a.todos.CreateTodo(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, models.CodeConflict, "todo already exists")
			return
		}
		a.internalError(w, r, "create todo", err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, models.TodoResponse{Todo: t.Public()})
}

func (a *api) handleGetTodo(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	t, err := a.todos.GetTodo(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "todo not found")
			return
		}
		a.internalError(w, r, "get todo", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.TodoResponse{Todo: t.Public()})
}

func (a *api) handleUpdateTodo(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	t, err := a.todos.GetTodo(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "todo not found")
			return
		}
		a.internalError(w, r, "get todo for update", err)
		return
	}

	var req models.UpdateTodoRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			httpx.WriteFieldError(w, "title", "title is required")
			return
		}
		if len(title) > 500 {
			httpx.WriteFieldError(w, "title", "title cannot exceed 500 characters")
			return
		}
		t.Title = title
	}

	if req.Description != nil {
		t.Description = req.Description
	}

	if req.Done != nil {
		t.Done = *req.Done
	}

	if req.Priority != nil {
		if *req.Priority < 1 || *req.Priority > 3 {
			httpx.WriteFieldError(w, "priority", "priority must be 1, 2, or 3")
			return
		}
		t.Priority = *req.Priority
	}

	if req.Date != nil {
		d, err := time.Parse(time.DateOnly, *req.Date)
		if err != nil {
			httpx.WriteFieldError(w, "date", "must be YYYY-MM-DD format")
			return
		}
		t.Date = d
	}

	if req.ClearRemindAt {
		t.RemindAt = nil
		t.RemindedAt = nil
	} else if req.RemindAt != nil {
		t.RemindAt = req.RemindAt
		t.RemindedAt = nil
	}

	if req.RepeatRule != nil {
		switch *req.RepeatRule {
		case "none", "daily", "weekly":
			t.RepeatRule = *req.RepeatRule
		default:
			httpx.WriteFieldError(w, "repeat_rule", "repeat_rule must be none, daily, or weekly")
			return
		}
	}

	t.UpdatedAt = time.Now().UTC()
	if err := a.todos.UpdateTodo(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "todo not found")
			return
		}
		a.internalError(w, r, "update todo", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.TodoResponse{Todo: t.Public()})
}

func (a *api) handleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if err := a.todos.DeleteTodo(r.Context(), u.ID, id, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "todo not found")
			return
		}
		a.internalError(w, r, "delete todo", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleRolloverTodos(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.RolloverRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	fromDate, err := time.Parse(time.DateOnly, req.FromDate)
	if err != nil {
		httpx.WriteFieldError(w, "from_date", "must be YYYY-MM-DD format")
		return
	}
	toDate, err := time.Parse(time.DateOnly, req.ToDate)
	if err != nil {
		httpx.WriteFieldError(w, "to_date", "must be YYYY-MM-DD format")
		return
	}

	count, err := a.todos.RolloverTodos(r.Context(), u.ID, fromDate, toDate, time.Now().UTC())
	if err != nil {
		a.internalError(w, r, "rollover todos", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.RolloverResponse{MovedCount: count})
}

func userToday(tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (a *api) handleBatchCreateTodos(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.BatchCreateTodoRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	now := time.Now().UTC()
	var todosToCreate []store.Todo

	items := req.Items
	if len(items) == 0 {
		items = req.Todos
	}

	if len(items) > 0 {
		if len(items) > 365 {
			httpx.WriteFieldError(w, "items", "cannot create more than 365 items at once")
			return
		}
		for i, item := range items {
			title := strings.TrimSpace(item.Title)
			if title == "" {
				httpx.WriteFieldError(w, fmt.Sprintf("items[%d].title", i), "title is required")
				return
			}
			if len(title) > 500 {
				httpx.WriteFieldError(w, fmt.Sprintf("items[%d].title", i), "title cannot exceed 500 characters")
				return
			}

			priority := uint8(2)
			if item.Priority != nil {
				if *item.Priority < 1 || *item.Priority > 3 {
					httpx.WriteFieldError(w, fmt.Sprintf("items[%d].priority", i), "priority must be 1, 2, or 3")
					return
				}
				priority = *item.Priority
			}

			repeatRule := "none"
			if item.RepeatRule != "" {
				switch item.RepeatRule {
				case "none", "daily", "weekly":
					repeatRule = item.RepeatRule
				default:
					httpx.WriteFieldError(w, fmt.Sprintf("items[%d].repeat_rule", i), "repeat_rule must be none, daily, or weekly")
					return
				}
			}

			var targetDate time.Time
			if item.Date != "" {
				d, err := time.Parse(time.DateOnly, item.Date)
				if err != nil {
					httpx.WriteFieldError(w, fmt.Sprintf("items[%d].date", i), "must be YYYY-MM-DD format")
					return
				}
				targetDate = d
			} else {
				targetDate = userToday(u.Timezone)
			}

			todosToCreate = append(todosToCreate, store.Todo{
				ID:          uuid.New().String(),
				UserID:      u.ID,
				Date:        targetDate,
				Title:       title,
				Description: item.Description,
				Done:        false,
				Priority:    priority,
				RemindAt:    item.RemindAt,
				RepeatRule:  repeatRule,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
		}
	} else {
		// Mode 1: Range generation (e.g. Monday to Friday)
		title := strings.TrimSpace(req.Title)
		if title == "" {
			httpx.WriteFieldError(w, "title", "title is required")
			return
		}
		if len(title) > 500 {
			httpx.WriteFieldError(w, "title", "title cannot exceed 500 characters")
			return
		}

		var startDate, endDate time.Time
		if req.StartDate != "" {
			d, err := time.Parse(time.DateOnly, req.StartDate)
			if err != nil {
				httpx.WriteFieldError(w, "start_date", "must be YYYY-MM-DD format")
				return
			}
			startDate = d
		} else {
			startDate = userToday(u.Timezone)
		}

		if req.EndDate != "" {
			d, err := time.Parse(time.DateOnly, req.EndDate)
			if err != nil {
				httpx.WriteFieldError(w, "end_date", "must be YYYY-MM-DD format")
				return
			}
			endDate = d
		} else {
			endDate = startDate
		}

		if endDate.Before(startDate) {
			httpx.WriteFieldError(w, "end_date", "end_date cannot be before start_date")
			return
		}

		if endDate.Sub(startDate) > 366*24*time.Hour {
			httpx.WriteFieldError(w, "end_date", "date range cannot exceed 366 days")
			return
		}

		priority := uint8(2)
		if req.Priority != nil {
			if *req.Priority < 1 || *req.Priority > 3 {
				httpx.WriteFieldError(w, "priority", "priority must be 1, 2, or 3")
				return
			}
			priority = *req.Priority
		}

		repeatRule := "none"
		if req.RepeatRule != "" {
			switch req.RepeatRule {
			case "none", "daily", "weekly":
				repeatRule = req.RepeatRule
			default:
				httpx.WriteFieldError(w, "repeat_rule", "repeat_rule must be none, daily, or weekly")
				return
			}
		}

		loc, err := time.LoadLocation(u.Timezone)
		if err != nil {
			loc = time.UTC
		}

		var remindHour, remindMin int
		var hasRemindTime bool
		if req.RemindTime != nil && strings.TrimSpace(*req.RemindTime) != "" {
			rt, err := time.Parse("15:04", strings.TrimSpace(*req.RemindTime))
			if err != nil {
				httpx.WriteFieldError(w, "remind_time", "must be HH:MM format (24-hour)")
				return
			}
			remindHour = rt.Hour()
			remindMin = rt.Minute()
			hasRemindTime = true
		}

		for cur := startDate; !cur.After(endDate); cur = cur.AddDate(0, 0, 1) {
			if req.WeekdaysOnly {
				wd := cur.Weekday()
				if wd == time.Sunday || wd == time.Saturday {
					continue
				}
			}

			var remindAt *time.Time
			if hasRemindTime {
				remindLocal := time.Date(cur.Year(), cur.Month(), cur.Day(), remindHour, remindMin, 0, 0, loc)
				remindUTC := remindLocal.UTC()
				remindAt = &remindUTC
			}

			todosToCreate = append(todosToCreate, store.Todo{
				ID:          uuid.New().String(),
				UserID:      u.ID,
				Date:        cur,
				Title:       title,
				Description: req.Description,
				Done:        false,
				Priority:    priority,
				RemindAt:    remindAt,
				RepeatRule:  repeatRule,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
		}
	}

	if len(todosToCreate) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, models.CodeBadRequest, "no todos matched the specified range/criteria")
		return
	}

	if err := a.todos.CreateTodosBatch(r.Context(), todosToCreate); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, models.CodeConflict, "one or more todos already exist")
			return
		}
		a.internalError(w, r, "batch create todos", err)
		return
	}

	resp := models.BatchCreateTodoResponse{
		CreatedCount: len(todosToCreate),
		Todos:        make([]models.Todo, len(todosToCreate)),
	}
	for i, t := range todosToCreate {
		resp.Todos[i] = t.Public()
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}
