// Package notify provides cross-platform desktop notifications and audio alerts
// for task reminders and chat messages.
package notify

import (
	"fmt"
	"sync/atomic"

	"github.com/gen2brain/beeep"
)

var enabled atomic.Bool

func init() {
	enabled.Store(true)
}

// SetEnabled toggles desktop notifications on or off.
func SetEnabled(val bool) {
	enabled.Store(val)
}

// IsEnabled returns whether desktop notifications are currently active.
func IsEnabled() bool {
	return enabled.Load()
}

// NotifyReminder dispatches a desktop alert with sound for a task reminder.
// It also rings the terminal bell (\a) and runs asynchronously so it never blocks the UI.
func NotifyReminder(title string, priority uint8) {
	// Terminal bell (always sound in terminal)
	fmt.Print("\a")

	if !enabled.Load() {
		return
	}

	go func() {
		prioLabel := fmt.Sprintf("P%d", priority)
		switch priority {
		case 1:
			prioLabel = "P1 Tinggi"
		case 2:
			prioLabel = "P2 Sedang"
		case 3:
			prioLabel = "P3 Rendah"
		}
		header := fmt.Sprintf("⏰ Pengingat To-do [%s]", prioLabel)
		_ = beeep.Alert(header, title, "")
	}()
}

// NotifyChatMessage dispatches a desktop notification for an incoming chat message.
func NotifyChatMessage(sender, body string) {
	if !enabled.Load() {
		return
	}

	go func() {
		if len(body) > 100 {
			body = body[:97] + "..."
		}
		header := fmt.Sprintf("💬 Pesan Baru dari %s", sender)
		_ = beeep.Notify(header, body, "")
	}()
}
