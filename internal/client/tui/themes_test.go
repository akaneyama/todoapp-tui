package tui_test

import (
	"testing"

	"todoapp/internal/client/tui"
)

func TestThemes_ApplyAndList(t *testing.T) {
	themes := tui.ListThemes()
	if len(themes) < 10 {
		t.Fatalf("expected at least 10 themes, got %d", len(themes))
	}

	// Verify all themes can be applied
	for _, th := range themes {
		if !tui.ApplyTheme(th) {
			t.Fatalf("expected apply theme '%s' to succeed", th)
		}
		if tui.CurrentTheme() != th {
			t.Errorf("expected current theme '%s', got '%s'", th, tui.CurrentTheme())
		}
	}

	// Test Pink themes
	pinkThemes := []string{"sakura", "rose-pine", "bubblegum"}
	for _, p := range pinkThemes {
		if !tui.ApplyTheme(p) {
			t.Fatalf("expected apply pink theme '%s' to succeed", p)
		}
	}

	// Test Light / Bright themes
	lightThemes := []string{"latte", "solarized-light", "nord-light"}
	for _, l := range lightThemes {
		if !tui.ApplyTheme(l) {
			t.Fatalf("expected apply light theme '%s' to succeed", l)
		}
	}

	// Test aliases
	if !tui.ApplyTheme("pink") || tui.CurrentTheme() != "sakura" {
		t.Errorf("expected alias 'pink' to apply 'sakura', got %s", tui.CurrentTheme())
	}
	if !tui.ApplyTheme("cerah") || tui.CurrentTheme() != "latte" {
		t.Errorf("expected alias 'cerah' to apply 'latte', got %s", tui.CurrentTheme())
	}
	if !tui.ApplyTheme("light") || tui.CurrentTheme() != "latte" {
		t.Errorf("expected alias 'light' to apply 'latte', got %s", tui.CurrentTheme())
	}
	if !tui.ApplyTheme("solarized") || tui.CurrentTheme() != "solarized-light" {
		t.Errorf("expected alias 'solarized' to apply 'solarized-light', got %s", tui.CurrentTheme())
	}
	if !tui.ApplyTheme("rose") || tui.CurrentTheme() != "rose-pine" {
		t.Errorf("expected alias 'rose' to apply 'rose-pine', got %s", tui.CurrentTheme())
	}

	// Invalid theme
	if tui.ApplyTheme("non-existent-theme-xyz") {
		t.Errorf("expected invalid theme to fail")
	}

	// Restore catppuccin
	if !tui.ApplyTheme("catppuccin") {
		t.Fatalf("expected apply catppuccin to succeed")
	}
	if tui.CurrentTheme() != "catppuccin" {
		t.Errorf("expected current theme catppuccin, got %s", tui.CurrentTheme())
	}
}
