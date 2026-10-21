package notify_test

import (
	"testing"

	"todoapp/internal/client/notify"
)

func TestNotify_EnabledToggle(t *testing.T) {
	notify.SetEnabled(false)
	if notify.IsEnabled() {
		t.Errorf("expected disabled")
	}

	notify.SetEnabled(true)
	if !notify.IsEnabled() {
		t.Errorf("expected enabled")
	}
}

func TestNotify_FunctionsDoNotPanic(t *testing.T) {
	notify.SetEnabled(false) // disable actual OS notification in test
	notify.NotifyReminder("Test Task", 1)
	notify.NotifyChatMessage("testuser", "Hello there!")
}
