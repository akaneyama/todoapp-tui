package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/config"
	"todoapp/internal/models"
)

func TestAppViewHeightNeverExceedsTerminalHeight(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	testSizes := []struct {
		w, h int
	}{
		{80, 24},
		{80, 20},
		{100, 30},
		{120, 40},
		{70, 22},
		{60, 18},
	}

	for _, sz := range testSizes {
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			app := New(cfg)
			m, _ := app.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
			m, _ = m.Update(loginSuccessMsg{
				Token: "test-token",
				User:  models.User{ID: 1, Username: "akane"},
			})

			appModel := m.(AppModel)
			appModel.journalView.entries = []models.JournalEntry{
				{ID: "1", Title: "dd", Body: "note 1 content", UpdatedAt: time.Now()},
				{ID: "2", Title: "cara install A", Body: "#dada\nddasdad\nline3\nline4\nline5", UpdatedAt: time.Now()},
			}
			appModel.journalView.entryCursor = 0
			appModel.journalView.updateViewportContent()

			// Test with and without global toast
			for _, toast := range []string{"", "Catatan berhasil disimpan!", "Selamat datang, akane!"} {
				appModel.globalToast = toast
				view := appModel.View()
				actualH := lipgloss.Height(view)

				lines := strings.Split(view, "\n")
				for idx, l := range lines {
					lw := lipgloss.Width(l)
					if lw > sz.w {
						t.Errorf("[%dx%d toast=%q] Line %d width %d > terminal width %d:\n%s", sz.w, sz.h, toast, idx, lw, sz.w, l)
					}
				}

				if actualH > sz.h {
					t.Errorf("[%dx%d toast=%q] App view height %d EXCEEDS terminal height %d (OVERFLOW by %d lines!)",
						sz.w, sz.h, toast, actualH, sz.h, actualH-sz.h)
				}
			}
		})
	}
}

func TestJournalArrowUpNoDuplicate(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	app := New(cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(loginSuccessMsg{
		Token: "test-token",
		User:  models.User{ID: 1, Username: "akane"},
	})

	appModel := m.(AppModel)
	appModel.journalView.entries = []models.JournalEntry{
		{ID: "1", Title: "dd", Tags: "dasdp", Body: "ddasdad\nsdasddasdasda", UpdatedAt: time.Now()},
		{ID: "2", Title: "cara install A", Tags: "", Body: "#dada\nddasdad", UpdatedAt: time.Now()},
	}
	appModel.journalView.entryCursor = 1 // Cursor on note 2
	appModel.journalView.updateViewportContent()

	viewBeforeUp := appModel.View()
	t.Logf("Before Up view:\n%s", viewBeforeUp)

	// User presses arrow up
	m2, _ := appModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	appModel2 := m2.(AppModel)

	if appModel2.journalView.entryCursor != 0 {
		t.Fatalf("expected entryCursor to be 0, got %d", appModel2.journalView.entryCursor)
	}

	viewAfterUp := appModel2.View()
	t.Logf("After Up view:\n%s", viewAfterUp)

	// Ensure line widths never exceed width - 1
	for idx, l := range strings.Split(viewAfterUp, "\n") {
		lw := lipgloss.Width(l)
		if lw > 80-1 {
			t.Errorf("Line %d width %d exceeds max allowed width 79:\n%s", idx, lw, l)
		}
	}

	// Ensure "cara install A" only appears ONCE
	countCaraInstall := strings.Count(viewAfterUp, "cara install A")
	if countCaraInstall != 1 {
		t.Errorf("expected 'cara install A' to appear exactly 1 time, but found %d times in view:\n%s", countCaraInstall, viewAfterUp)
	}

	// Ensure "+ [a] Tambah" only appears ONCE
	countTambah := strings.Count(viewAfterUp, "+ [a] Tambah")
	if countTambah != 1 {
		t.Errorf("expected '+ [a] Tambah' to appear exactly 1 time, but found %d times in view:\n%s", countTambah, viewAfterUp)
	}
}

func TestTodoInitialDataLoadRouting(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	app := New(cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(loginSuccessMsg{
		Token: "test-token",
		User:  models.User{ID: 1, Username: "akane"},
	})

	// App is initially on Tab 1 (tabJournal)
	appModel := m.(AppModel)
	if appModel.activeTab != tabJournal {
		t.Fatalf("expected activeTab to be tabJournal, got %v", appModel.activeTab)
	}

	// Simulated background todosLoadedMsg arrives while user is on tabJournal
	todayStr := time.Now().Format(time.DateOnly)
	prio := uint8(2)
	testTodos := []models.Todo{
		{ID: "todo-1", Title: "Task Penting 1", Done: false, Priority: prio},
		{ID: "todo-2", Title: "Task Penting 2", Done: true, Priority: prio},
	}
	m2, _ := appModel.Update(todosLoadedMsg{
		Date:  todayStr,
		Todos: testTodos,
	})
	appModel2 := m2.(AppModel)

	// Now user switches to Tab 2 (To-do)
	m3, _ := appModel2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	appModel3 := m3.(AppModel)

	// Verify that todos are present immediately without needing date change
	if len(appModel3.todoView.todos) != 2 {
		t.Fatalf("expected 2 todos in todoView, got %d", len(appModel3.todoView.todos))
	}
	view := appModel3.View()
	if !strings.Contains(view, "Task Penting 1") {
		t.Errorf("expected view to contain 'Task Penting 1', got:\n%s", view)
	}
}

func TestJournalNavigationAfterEditNeverFreezes(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	app := New(cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(loginSuccessMsg{
		Token: "test-token",
		User:  models.User{ID: 1, Username: "akane"},
	})

	appModel := m.(AppModel)
	appModel.journalView.entries = []models.JournalEntry{
		{ID: "note-1", Title: "Catatan Pertama", Body: "Isi catatan pertama", UpdatedAt: time.Now()},
		{ID: "note-2", Title: "Catatan Kedua", Body: "Isi catatan kedua", UpdatedAt: time.Now()},
		{ID: "note-3", Title: "Catatan Ketiga", Body: "Isi catatan ketiga", UpdatedAt: time.Now()},
	}
	appModel.journalView.entryCursor = 0
	appModel.journalView.updateViewportContent()

	// 1. User presses Enter on note 1 to open/edit it
	m2, _ := appModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	appModel2 := m2.(AppModel)
	if appModel2.journalView.subMode != journalModeEdit {
		t.Fatalf("expected subMode to be journalModeEdit, got %v", appModel2.journalView.subMode)
	}

	// 2. User presses Esc to finish viewing/editing
	m3, _ := appModel2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel3 := m3.(AppModel)
	if appModel3.journalView.subMode != journalModeView {
		t.Fatalf("expected subMode to be journalModeView, got %v", appModel3.journalView.subMode)
	}

	// 3. User immediately presses Down arrow -> should move to note 2 without freezing!
	m4, _ := appModel3.Update(tea.KeyMsg{Type: tea.KeyDown})
	appModel4 := m4.(AppModel)
	if appModel4.journalView.entryCursor != 1 {
		t.Fatalf("expected entryCursor to be 1 after Down arrow, got %d", appModel4.journalView.entryCursor)
	}

	// 4. User presses Down again -> should move to note 3
	m5, _ := appModel4.Update(tea.KeyMsg{Type: tea.KeyDown})
	appModel5 := m5.(AppModel)
	if appModel5.journalView.entryCursor != 2 {
		t.Fatalf("expected entryCursor to be 2 after Down arrow, got %d", appModel5.journalView.entryCursor)
	}

	// 5. User presses Up arrow -> should move back to note 2
	m6, _ := appModel5.Update(tea.KeyMsg{Type: tea.KeyUp})
	appModel6 := m6.(AppModel)
	if appModel6.journalView.entryCursor != 1 {
		t.Fatalf("expected entryCursor to be 1 after Up arrow, got %d", appModel6.journalView.entryCursor)
	}

	// 6. User presses Tab -> should switch to Tab 2 (tabTodo) without getting stuck
	m7, _ := appModel6.Update(tea.KeyMsg{Type: tea.KeyTab})
	appModel7 := m7.(AppModel)
	if appModel7.activeTab != tabTodo {
		t.Fatalf("expected activeTab to switch to tabTodo on Tab key, got %v", appModel7.activeTab)
	}
}

func TestJournalSearchModalAndSelection(t *testing.T) {
	cfg := config.Config{
		ServerURL: "http://127.0.0.1:8080",
		Token:     "test-token",
		Username:  "akane",
	}

	app := New(cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(loginSuccessMsg{
		Token: "test-token",
		User:  models.User{ID: 1, Username: "akane"},
	})

	appModel := m.(AppModel)

	// 1. Press '/' to open search modal
	m2, _ := appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	appModel2 := m2.(AppModel)
	if appModel2.journalView.subMode != journalModeSearch {
		t.Fatalf("expected subMode to be journalModeSearch, got %v", appModel2.journalView.subMode)
	}

	// 2. Type "kerja" (contains 'k' and 'j' - must not be swallowed!)
	for _, r := range "kerja" {
		m2, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	appModel3 := m2.(AppModel)
	if appModel3.journalView.searchInput.Value() != "kerja" {
		t.Fatalf("expected searchInput to be 'kerja', got %q", appModel3.journalView.searchInput.Value())
	}

	// 3. Simulate search results arriving
	matches := []models.JournalSearchMatch{
		{ID: "n-1", Date: "2026-10-18", Title: "Cara install A", Tags: "kerja", Snippet: "Panduan setup"},
		{ID: "n-2", Date: "2026-10-19", Title: "dd", Tags: "dasdp", Snippet: "catatan harian"},
	}
	m4, _ := appModel3.Update(journalSearchLoadedMsg{Matches: matches})
	appModel4 := m4.(AppModel)
	if len(appModel4.journalView.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(appModel4.journalView.searchMatches))
	}

	// Verify search modal view renders Title and Tags
	modalView := appModel4.View()
	if !strings.Contains(modalView, "Cara install A") || !strings.Contains(modalView, "🏷 kerja") {
		t.Errorf("expected search modal to show match title and tags, got:\n%s", modalView)
	}

	// 4. Down arrow moves cursor to match 2
	m5, _ := appModel4.Update(tea.KeyMsg{Type: tea.KeyDown})
	appModel5 := m5.(AppModel)
	if appModel5.journalView.searchCursor != 1 {
		t.Fatalf("expected searchCursor to be 1, got %d", appModel5.journalView.searchCursor)
	}

	// Set lastSearchQuery so Enter selects the match
	appModel5.journalView.lastSearchQuery = "kerja"

	// 5. Press Enter to select match 2
	m6, _ := appModel5.Update(tea.KeyMsg{Type: tea.KeyEnter})
	appModel6 := m6.(AppModel)
	if appModel6.journalView.subMode != journalModeView {
		t.Fatalf("expected subMode to return to journalModeView, got %v", appModel6.journalView.subMode)
	}
	if appModel6.journalView.date.Format(time.DateOnly) != "2026-10-19" {
		t.Fatalf("expected journal date to be 2026-10-19, got %s", appModel6.journalView.date.Format(time.DateOnly))
	}
	if appModel6.journalView.activeNoteID != "n-2" {
		t.Fatalf("expected activeNoteID to be 'n-2', got %s", appModel6.journalView.activeNoteID)
	}
}
