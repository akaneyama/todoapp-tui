package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

func TestPasteIntoTextInput(t *testing.T) {
	// 1. Test basic paste into empty textinput
	in := textinput.New()
	in.SetValue("Hello World")
	in.SetCursor(5) // between "Hello" and " World"

	_ = CopyToClipboard(" Beautiful")
	pasted, err := PasteIntoTextInput(&in)
	if err != nil {
		t.Skipf("skipping clipboard test (no GUI/clipboard available in test environment): %v", err)
		return
	}

	if pasted != " Beautiful" {
		t.Errorf("expected pasted %q, got %q", " Beautiful", pasted)
	}
	if in.Value() != "Hello Beautiful World" {
		t.Errorf("expected value %q, got %q", "Hello Beautiful World", in.Value())
	}
	if in.Position() != 15 {
		t.Errorf("expected cursor position 15, got %d", in.Position())
	}
}

func TestPasteIntoTextarea(t *testing.T) {
	ta := textarea.New()
	ta.SetValue("Line 1\nLine 3")
	ta.CursorUp()

	_ = CopyToClipboard("Inserted Line\n")
	pasted, err := PasteIntoTextarea(&ta)
	if err != nil {
		t.Skipf("skipping clipboard test: %v", err)
		return
	}

	if pasted == "" {
		t.Errorf("expected non-empty pasted text")
	}
}
