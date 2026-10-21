package reminder

import (
	"context"
	"log/slog"
	"time"

	"todoapp/internal/models"
	"todoapp/internal/server/store"
)

// Store defines the database operations required by the reminder scheduler.
type Store interface {
	ListDueReminders(ctx context.Context, now time.Time, limit int) ([]store.Todo, error)
	MarkReminderSent(ctx context.Context, id string, now time.Time) error
}

// Broadcaster sends real-time events to users.
type Broadcaster interface {
	BroadcastToUser(userID int64, event models.WSEvent) error
}

// Scheduler periodically checks for due task reminders and dispatches WebSocket events.
type Scheduler struct {
	store       Store
	broadcaster Broadcaster
	logger      *slog.Logger
	interval    time.Duration
}

// NewScheduler creates a new reminder scheduler.
func NewScheduler(s Store, b Broadcaster, l *slog.Logger, interval time.Duration) *Scheduler {
	if l == nil {
		l = slog.Default()
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Scheduler{
		store:       s,
		broadcaster: b,
		logger:      l,
		interval:    interval,
	}
}

// Start runs the periodic check loop until the context is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("reminder scheduler started", "interval", s.interval)

	// Run an immediate check on startup
	s.CheckAndDispatch(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("reminder scheduler stopped")
			return
		case <-ticker.C:
			s.CheckAndDispatch(ctx)
		}
	}
}

// CheckAndDispatch queries for due reminders and dispatches events.
func (s *Scheduler) CheckAndDispatch(ctx context.Context) int {
	now := time.Now().UTC()
	todos, err := s.store.ListDueReminders(ctx, now, 50)
	if err != nil {
		s.logger.Error("failed to list due reminders", "error", err)
		return 0
	}

	dispatched := 0
	for _, t := range todos {
		if err := s.store.MarkReminderSent(ctx, t.ID, now); err != nil {
			s.logger.Error("failed to mark reminder sent", "todo_id", t.ID, "error", err)
			continue
		}

		event := models.WSEvent{
			Type: "reminder",
			Payload: models.WSReminder{
				TodoID:   t.ID,
				Date:     t.Date.Format(time.DateOnly),
				Title:    t.Title,
				Priority: t.Priority,
				RemindAt: t.RemindAt,
			},
		}

		if s.broadcaster != nil {
			if err := s.broadcaster.BroadcastToUser(t.UserID, event); err != nil {
				s.logger.Error("failed to broadcast reminder", "todo_id", t.ID, "user_id", t.UserID, "error", err)
			} else {
				s.logger.Info("dispatched task reminder", "todo_id", t.ID, "title", t.Title, "user_id", t.UserID)
				dispatched++
			}
		}
	}
	return dispatched
}
