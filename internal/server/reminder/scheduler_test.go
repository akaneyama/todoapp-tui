package reminder_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"todoapp/internal/models"
	"todoapp/internal/server/reminder"
	"todoapp/internal/server/store"
	"todoapp/internal/server/store/memstore"
)

type mockBroadcaster struct {
	mu     sync.Mutex
	events []struct {
		UserID int64
		Event  models.WSEvent
	}
}

func (m *mockBroadcaster) BroadcastToUser(userID int64, event models.WSEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, struct {
		UserID int64
		Event  models.WSEvent
	}{UserID: userID, Event: event})
	return nil
}

func TestScheduler_CheckAndDispatch(t *testing.T) {
	st := memstore.New(nil)
	b := &mockBroadcaster{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sched := reminder.NewScheduler(st, b, logger, 50*time.Millisecond)

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Todo due for reminder
	dueTime := now.Add(-5 * time.Minute)
	err := st.CreateTodo(ctx, store.Todo{
		ID:         "todo-due-1",
		UserID:     10,
		Date:       now,
		Title:      "Buy groceries",
		Done:       false,
		Priority:   2,
		RemindAt:   &dueTime,
		RepeatRule: "none",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create todo error: %v", err)
	}

	// 2. Todo with future reminder (should NOT be dispatched)
	futureTime := now.Add(2 * time.Hour)
	err = st.CreateTodo(ctx, store.Todo{
		ID:         "todo-future-2",
		UserID:     10,
		Date:       now,
		Title:      "Doctor appointment",
		Done:       false,
		Priority:   1,
		RemindAt:   &futureTime,
		RepeatRule: "none",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create todo error: %v", err)
	}

	// 3. Todo that is already done (should NOT be dispatched)
	err = st.CreateTodo(ctx, store.Todo{
		ID:         "todo-done-3",
		UserID:     10,
		Date:       now,
		Title:      "Old completed task",
		Done:       true,
		Priority:   3,
		RemindAt:   &dueTime,
		RepeatRule: "none",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create todo error: %v", err)
	}

	// First dispatch run
	dispatched := sched.CheckAndDispatch(ctx)
	if dispatched != 1 {
		t.Fatalf("expected 1 dispatched reminder, got %d", dispatched)
	}

	b.mu.Lock()
	if len(b.events) != 1 {
		t.Fatalf("expected 1 event sent, got %d", len(b.events))
	}
	evt := b.events[0]
	if evt.UserID != 10 {
		t.Errorf("expected user ID 10, got %d", evt.UserID)
	}
	if evt.Event.Type != "reminder" {
		t.Errorf("expected event type reminder, got %s", evt.Event.Type)
	}
	rem, ok := evt.Event.Payload.(models.WSReminder)
	if !ok || rem.TodoID != "todo-due-1" {
		t.Errorf("expected payload to contain todo-due-1, got %+v", evt.Event.Payload)
	}
	b.mu.Unlock()

	// Second dispatch run immediately: should NOT dispatch again because reminded_at was set!
	dispatched2 := sched.CheckAndDispatch(ctx)
	if dispatched2 != 0 {
		t.Errorf("expected 0 dispatched on second run, got %d", dispatched2)
	}
}
