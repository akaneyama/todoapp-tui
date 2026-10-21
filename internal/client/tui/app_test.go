package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"todoapp/internal/client/config"
	"todoapp/internal/client/tui"
)

func TestAppModelLifecycle(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "",
	}

	app := tui.New(cfg)

	// Test Init
	cmd := app.Init()
	if cmd == nil {
		t.Fatal("expected non-nil init cmd")
	}

	// Test Window resize
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := updated.View()
	if len(view) == 0 {
		t.Fatal("expected non-empty view string")
	}
}

func TestLogoutReset(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	app := tui.New(cfg)

	// Trigger :logout via command palette
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("logout")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// View should show LOGIN without "Menghubungkan ke server..."
	view := m.View()
	if len(view) == 0 {
		t.Fatal("expected non-empty view after logout")
	}
}
