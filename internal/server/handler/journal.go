package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"todoapp/internal/models"
	"todoapp/internal/server/httpx"
	"todoapp/internal/server/middleware"
	"todoapp/internal/server/store"
)

func (a *api) handleGetJournal(w http.ResponseWriter, r *http.Request) {
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
		list, err := a.journal.ListJournalSince(r.Context(), u.ID, sinceTime)
		if err != nil {
			a.internalError(w, r, "list journal since", err)
			return
		}
		resp := models.JournalListResponse{
			Entries:    make([]models.JournalEntry, 0, len(list)),
			ServerTime: time.Now().UTC(),
		}
		for _, item := range list {
			resp.Entries = append(resp.Entries, item.Public())
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

	list, err := a.journal.ListJournalsByDate(r.Context(), u.ID, targetDate)
	if err != nil {
		a.internalError(w, r, "list journals by date", err)
		return
	}

	if len(list) == 0 {
		httpx.WriteJSON(w, http.StatusOK, models.JournalResponse{Entry: nil, Entries: []models.JournalEntry{}})
		return
	}

	entries := make([]models.JournalEntry, len(list))
	for i, it := range list {
		entries[i] = it.Public()
	}
	first := entries[0]
	httpx.WriteJSON(w, http.StatusOK, models.JournalResponse{Entry: &first, Entries: entries})
}

func (a *api) handleSaveJournal(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.SaveJournalRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
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

	title := strings.TrimSpace(req.Title)
	if len(title) > 255 {
		httpx.WriteFieldError(w, "title", "title cannot exceed 255 characters")
		return
	}

	tags := strings.TrimSpace(req.Tags)
	if len(tags) > 512 {
		httpx.WriteFieldError(w, "tags", "tags cannot exceed 512 characters")
		return
	}

	entryID := strings.TrimSpace(req.ID)
	if entryID == "" {
		entryID = uuid.New().String()
	}

	now := time.Now().UTC()
	entry := store.JournalEntry{
		ID:        entryID,
		UserID:    u.ID,
		Date:      targetDate,
		Title:     title,
		Body:      req.Body,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}

	saved, err := a.journal.UpsertJournal(r.Context(), entry)
	if err != nil {
		a.internalError(w, r, "upsert journal", err)
		return
	}

	pub := saved.Public()
	httpx.WriteJSON(w, http.StatusOK, models.JournalResponse{Entry: &pub})
}

func (a *api) handleDeleteJournal(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id != "" {
		if err := a.journal.DeleteJournalByID(r.Context(), u.ID, id, time.Now().UTC()); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "journal entry not found")
				return
			}
			a.internalError(w, r, "delete journal by id", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		httpx.WriteFieldError(w, "date", "either 'id' or 'date' query parameter is required")
		return
	}
	targetDate, err := time.Parse(time.DateOnly, dateStr)
	if err != nil {
		httpx.WriteFieldError(w, "date", "must be YYYY-MM-DD format")
		return
	}

	if err := a.journal.DeleteJournal(r.Context(), u.ID, targetDate, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "journal entry not found")
			return
		}
		a.internalError(w, r, "delete journal", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleSearchJournal(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httpx.WriteFieldError(w, "q", "search query 'q' cannot be empty")
		return
	}

	matches, err := a.journal.SearchJournal(r.Context(), u.ID, query)
	if err != nil {
		a.internalError(w, r, "search journal", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.JournalSearchResponse{
		Query:   query,
		Matches: matches,
	})
}
