package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

// CopyToClipboard copies the given string to the system clipboard.
func CopyToClipboard(text string) error {
	return clipboard.WriteAll(text)
}

// ReadFromClipboard reads plain text from the system clipboard.
func ReadFromClipboard() (string, error) {
	return clipboard.ReadAll()
}

// PasteIntoTextInput inserts text from the system clipboard into the textinput at the current cursor position.
func PasteIntoTextInput(in *textinput.Model) (string, error) {
	text, err := clipboard.ReadAll()
	if err != nil || text == "" {
		return "", err
	}

	// Normalize and remove newlines for single-line inputs
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")

	val := []rune(in.Value())
	pos := in.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(val) {
		pos = len(val)
	}

	left := val[:pos]
	right := val[pos:]
	newTextRunes := []rune(text)

	if in.CharLimit > 0 {
		avail := in.CharLimit - len(val)
		if avail <= 0 {
			return "", nil
		}
		if len(newTextRunes) > avail {
			newTextRunes = newTextRunes[:avail]
		}
	}

	combined := make([]rune, 0, len(left)+len(newTextRunes)+len(right))
	combined = append(combined, left...)
	combined = append(combined, newTextRunes...)
	combined = append(combined, right...)

	in.SetValue(string(combined))
	in.SetCursor(pos + len(newTextRunes))
	return string(newTextRunes), nil
}

// PasteIntoTextarea inserts text from the system clipboard into the textarea.
func PasteIntoTextarea(ta *textarea.Model) (string, error) {
	text, err := clipboard.ReadAll()
	if err != nil || text == "" {
		return "", err
	}

	// Normalize CRLF to LF
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	ta.InsertString(text)
	return text, nil
}
