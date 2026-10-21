package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/api"
	"todoapp/internal/models"
)

type journalSubMode int

const (
	journalModeView journalSubMode = iota
	journalModeEdit
	journalModeSearch
	journalModeConfirmDelete
)

type journalLoadedMsg struct {
	Date    string
	Entries []models.JournalEntry
}

type journalSavedMsg struct {
	Entry *models.JournalEntry
}

type journalDeletedMsg struct{}

type journalErrMsg struct {
	Err error
}

type journalSearchLoadedMsg struct {
	Matches []models.JournalSearchMatch
}

type editorFinishedMsg struct {
	err      error
	tempFile string
}

type journalViewModel struct {
	apiClient     *api.Client
	date          time.Time
	entries       []models.JournalEntry
	entryCursor   int
	activeNoteID  string
	subMode       journalSubMode
	loading       bool
	spinner       spinner.Model
	viewport      viewport.Model
	titleInput    textinput.Model
	textarea      textarea.Model
	tagsInput     textinput.Model
	editFocus     int // 0: title, 1: textarea, 2: tags
	statusMsg     string
	errMsg        string
	width         int
	height        int

	// Search
	searchInput     textinput.Model
	searchMatches   []models.JournalSearchMatch
	searchCursor    int
	lastSearchQuery string
}

func newJournalViewModel(apiClient *api.Client) journalViewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	vp := viewport.New(60, 15)
	vp.SetContent("Memuat catatan jurnal...")

	titleIn := textinput.New()
	titleIn.Placeholder = "Judul catatan jurnal (opsional)"
	titleIn.Prompt = "Judul: "
	titleIn.CharLimit = 255

	ta := textarea.New()
	ta.Placeholder = "Tulis jurnal harian dalam format Markdown di sini..."
	ta.ShowLineNumbers = true
	ta.SetWidth(60)
	ta.SetHeight(12)

	tagsIn := textinput.New()
	tagsIn.Placeholder = "tag1, tag2, ide (dipisahkan koma)"
	tagsIn.Prompt = "Tags: "
	tagsIn.CharLimit = 512

	sIn := textinput.New()
	sIn.Placeholder = "Cari kata kunci, tag (mis. #dada, Go), atau judul..."
	sIn.Prompt = "Cari: "
	sIn.CharLimit = 255

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	return journalViewModel{
		apiClient:   apiClient,
		date:        today,
		entries:     []models.JournalEntry{},
		entryCursor: 0,
		subMode:     journalModeView,
		spinner:     s,
		viewport:    vp,
		titleInput:  titleIn,
		textarea:    ta,
		tagsInput:   tagsIn,
		searchInput: sIn,
	}
}

func (m journalViewModel) Init() tea.Cmd {
	return tea.Batch(m.loadJournalCmd(m.date), m.spinner.Tick)
}

func (m journalViewModel) loadJournalCmd(date time.Time) tea.Cmd {
	client := m.apiClient
	dateStr := date.Format(time.DateOnly)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.GetJournal(ctx, dateStr)
		if err != nil {
			return journalErrMsg{Err: err}
		}
		var entries []models.JournalEntry
		if resp != nil {
			entries = resp.Entries
			if len(entries) == 0 && resp.Entry != nil && (resp.Entry.Title != "" || resp.Entry.Body != "") {
				entries = []models.JournalEntry{*resp.Entry}
			}
		}
		return journalLoadedMsg{Date: dateStr, Entries: entries}
	}
}

func (m *journalViewModel) ensureViewportDimensions() {
	listWidth := 26
	if m.width > 90 {
		listWidth = 30
	}
	vpWidth := max(20, m.width-listWidth-10)
	vpHeight := max(5, m.height-8)
	m.viewport.Width = vpWidth
	m.viewport.Height = vpHeight
	m.textarea.SetWidth(max(30, m.width-12))
	m.textarea.SetHeight(max(5, vpHeight-4))
}

func padLine(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		s = lipgloss.NewStyle().MaxWidth(width).Render(s)
		w = lipgloss.Width(s)
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

func padBlock(s string, width, height int) string {
	s = strings.ReplaceAll(s, "\r", "")
	rawLines := strings.Split(s, "\n")
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(rawLines) {
			line := rawLines[i]
			lines = append(lines, padLine(line, width))
		} else {
			lines = append(lines, strings.Repeat(" ", width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *journalViewModel) updateViewportContent() {
	m.ensureViewportDimensions()
	if len(m.entries) == 0 {
		m.viewport.SetContent(lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).
			Render("Belum ada catatan jurnal untuk tanggal ini.\n\nTekan [a] untuk membuat catatan baru atau [e] untuk membuka editor eksternal ($EDITOR)."))
		m.viewport.GotoTop()
		return
	}
	if m.entryCursor < 0 {
		m.entryCursor = 0
	}
	if m.entryCursor >= len(m.entries) {
		m.entryCursor = max(0, len(m.entries)-1)
	}
	entry := m.entries[m.entryCursor]
	var content strings.Builder
	title := strings.TrimSpace(entry.Title)
	if title == "" {
		title = fmt.Sprintf("Catatan #%d", m.entryCursor+1)
	}

	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("📌 "+title) + "\n")

	metaStyle := lipgloss.NewStyle().Foreground(ColorSubtle)
	timeStr := entry.UpdatedAt.Local().Format("02 Jan 2006 15:04")
	if entry.Tags != "" {
		tagBadge := lipgloss.NewStyle().Foreground(ColorPrimary).Render("🏷 " + entry.Tags)
		content.WriteString(tagBadge + "  " + metaStyle.Render("🕒 "+timeStr) + "\n")
	} else {
		content.WriteString(metaStyle.Render("🕒 Disimpan: "+timeStr) + "\n")
	}

	dividerWidth := max(18, m.viewport.Width-2)
	content.WriteString(lipgloss.NewStyle().Foreground(ColorBorder).Render(strings.Repeat("─", dividerWidth)) + "\n")

	bodyClean := strings.ReplaceAll(entry.Body, "\r", "")
	content.WriteString(bodyClean)

	m.viewport.SetContent(content.String())
	m.viewport.GotoTop()
}

func (m journalViewModel) Update(msg tea.Msg) (journalViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureViewportDimensions()
		m.updateViewportContent()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case journalLoadedMsg:
		m.loading = false
		m.entries = msg.Entries
		if m.activeNoteID != "" {
			for idx, ent := range m.entries {
				if ent.ID == m.activeNoteID {
					m.entryCursor = idx
					break
				}
			}
		}
		if m.entryCursor >= len(m.entries) {
			m.entryCursor = max(0, len(m.entries)-1)
		}
		m.updateViewportContent()
		return m, nil

	case journalSavedMsg:
		m.loading = false
		m.statusMsg = "Catatan jurnal berhasil disimpan!"
		m.errMsg = ""
		m.subMode = journalModeView
		if msg.Entry != nil && msg.Entry.ID != "" {
			m.activeNoteID = msg.Entry.ID
		}
		return m, m.loadJournalCmd(m.date)

	case journalDeletedMsg:
		m.loading = false
		m.statusMsg = "Catatan jurnal berhasil dihapus."
		m.errMsg = ""
		m.subMode = journalModeView
		if m.entryCursor > 0 {
			m.entryCursor--
		}
		return m, m.loadJournalCmd(m.date)

	case journalErrMsg:
		m.loading = false
		m.errMsg = msg.Err.Error()
		m.statusMsg = ""
		return m, nil

	case journalSearchLoadedMsg:
		m.loading = false
		m.searchMatches = msg.Matches
		m.searchCursor = 0
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Editor error: %v", msg.err)
			_ = os.Remove(msg.tempFile)
			return m, nil
		}
		content, err := os.ReadFile(msg.tempFile)
		_ = os.Remove(msg.tempFile)
		if err != nil {
			m.errMsg = fmt.Sprintf("Gagal membaca hasil editor: %v", err)
			return m, nil
		}
		newTitle, newBody := parseEditorContent(string(content))
		var currentEntry *models.JournalEntry
		if len(m.entries) > 0 && m.entryCursor < len(m.entries) {
			currentEntry = &m.entries[m.entryCursor]
		}
		if currentEntry != nil && newTitle == currentEntry.Title && newBody == currentEntry.Body {
			m.statusMsg = "Tidak ada perubahan."
			return m, nil
		}
		tags := ""
		if currentEntry != nil {
			tags = currentEntry.Tags
		}
		return m, m.saveJournalCmd(newTitle, newBody, tags, m.activeNoteID)

	case tea.KeyMsg:
		switch m.subMode {
		case journalModeView:
			return m.handleViewKeys(msg)
		case journalModeEdit:
			return m.handleEditKeys(msg)
		case journalModeSearch:
			return m.handleSearchKeys(msg)
		case journalModeConfirmDelete:
			return m.handleDeleteConfirmKeys(msg)
		}
	}

	return m, nil
}

func (m journalViewModel) handleViewKeys(msg tea.KeyMsg) (journalViewModel, tea.Cmd) {
	key := strings.ToLower(msg.String())
	switch key {
	case "h", "[", "left":
		m.date = m.date.AddDate(0, 0, -1)
		m.loading = true
		m.entryCursor = 0
		m.activeNoteID = ""
		m.statusMsg = ""
		m.errMsg = ""
		return m, m.loadJournalCmd(m.date)

	case "l", "]", "right":
		m.date = m.date.AddDate(0, 0, 1)
		m.loading = true
		m.entryCursor = 0
		m.activeNoteID = ""
		m.statusMsg = ""
		m.errMsg = ""
		return m, m.loadJournalCmd(m.date)

	case "t":
		now := time.Now()
		m.date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		m.loading = true
		m.entryCursor = 0
		m.activeNoteID = ""
		m.statusMsg = ""
		m.errMsg = ""
		return m, m.loadJournalCmd(m.date)

	case "k", "up":
		m.statusMsg = ""
		m.errMsg = ""
		if len(m.entries) > 0 {
			if m.entryCursor > 0 {
				m.entryCursor--
				m.activeNoteID = m.entries[m.entryCursor].ID
				m.updateViewportContent()
			}
			return m, nil
		}
		m.viewport.LineUp(1)
		return m, nil

	case "j", "down":
		m.statusMsg = ""
		m.errMsg = ""
		if len(m.entries) > 0 {
			if m.entryCursor < len(m.entries)-1 {
				m.entryCursor++
				m.activeNoteID = m.entries[m.entryCursor].ID
				m.updateViewportContent()
			}
			return m, nil
		}
		m.viewport.LineDown(1)
		return m, nil

	case "ctrl+k", "ctrl+p":
		m.viewport.LineUp(1)
		return m, nil

	case "ctrl+j", "ctrl+n":
		m.viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.viewport.GotoTop()
		return m, nil

	case "end":
		m.viewport.GotoBottom()
		return m, nil

	case "esc":
		m.statusMsg = ""
		m.errMsg = ""
		return m, nil

	case "c", "y":
		if len(m.entries) > 0 && m.entryCursor < len(m.entries) {
			entry := m.entries[m.entryCursor]
			var toCopy strings.Builder
			if strings.TrimSpace(entry.Title) != "" {
				toCopy.WriteString(entry.Title + "\n\n")
			}
			if strings.TrimSpace(entry.Tags) != "" {
				toCopy.WriteString("Tags: " + entry.Tags + "\n\n")
			}
			toCopy.WriteString(entry.Body)
			if err := CopyToClipboard(toCopy.String()); err != nil {
				m.errMsg = "Gagal menyalin: " + err.Error()
			} else {
				m.statusMsg = "📋 Catatan disalin ke clipboard!"
				m.errMsg = ""
			}
		} else {
			m.statusMsg = "Belum ada catatan untuk disalin."
		}
		return m, nil

	case "a", "n":
		// Tambah catatan baru
		m.subMode = journalModeEdit
		m.activeNoteID = ""
		m.editFocus = 0
		m.titleInput.SetValue("")
		m.textarea.SetValue("")
		m.tagsInput.SetValue("")
		m.titleInput.Focus()
		m.textarea.Blur()
		m.tagsInput.Blur()
		m.statusMsg = ""
		m.errMsg = ""
		return m, textinput.Blink

	case "i", "enter":
		// Edit catatan yang sedang dipilih
		if len(m.entries) == 0 {
			m.subMode = journalModeEdit
			m.activeNoteID = ""
			m.editFocus = 0
			m.titleInput.SetValue("")
			m.textarea.SetValue("")
			m.tagsInput.SetValue("")
			m.titleInput.Focus()
			m.textarea.Blur()
			m.tagsInput.Blur()
			m.statusMsg = ""
			m.errMsg = ""
			return m, textinput.Blink
		}
		entry := m.entries[m.entryCursor]
		m.subMode = journalModeEdit
		m.activeNoteID = entry.ID
		m.editFocus = 0
		m.titleInput.SetValue(entry.Title)
		m.textarea.SetValue(entry.Body)
		m.tagsInput.SetValue(entry.Tags)
		m.titleInput.Focus()
		m.textarea.Blur()
		m.tagsInput.Blur()
		m.statusMsg = ""
		m.errMsg = ""
		return m, textinput.Blink

	case "e":
		// Buka di external editor ($EDITOR / notepad)
		return m.openExternalEditor()

	case "/":
		// Buka Pencarian
		m.subMode = journalModeSearch
		m.searchInput.SetValue("")
		m.searchMatches = nil
		m.searchCursor = 0
		m.lastSearchQuery = ""
		m.searchInput.Focus()
		return m, textinput.Blink

	case "d":
		// Konfirmasi Hapus catatan terpilih
		if len(m.entries) > 0 {
			m.subMode = journalModeConfirmDelete
		}
		return m, nil

	case "r":
		m.loading = true
		return m, m.loadJournalCmd(m.date)

	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m journalViewModel) handleEditKeys(msg tea.KeyMsg) (journalViewModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		// Save
		title := strings.TrimSpace(m.titleInput.Value())
		body := m.textarea.Value()
		tags := m.tagsInput.Value()
		return m, m.saveJournalCmd(title, body, tags, m.activeNoteID)

	case "ctrl+v":
		var err error
		var pasted string
		switch m.editFocus {
		case 0:
			pasted, err = PasteIntoTextInput(&m.titleInput)
		case 1:
			pasted, err = PasteIntoTextarea(&m.textarea)
		case 2:
			pasted, err = PasteIntoTextInput(&m.tagsInput)
		}
		if err != nil {
			m.errMsg = "Gagal menempel: " + err.Error()
		} else if pasted != "" {
			m.statusMsg = "📋 Berhasil menempel dari clipboard!"
			m.errMsg = ""
		}
		return m, nil

	case "ctrl+c":
		var toCopy string
		switch m.editFocus {
		case 0:
			toCopy = m.titleInput.Value()
		case 1:
			toCopy = m.textarea.Value()
		case 2:
			toCopy = m.tagsInput.Value()
		}
		if toCopy != "" {
			if err := CopyToClipboard(toCopy); err == nil {
				m.statusMsg = "📋 Teks field disalin ke clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "esc":
		m.subMode = journalModeView
		return m, nil

	case "tab":
		m.editFocus = (m.editFocus + 1) % 3
		return m.updateEditFocus()

	case "shift+tab":
		m.editFocus--
		if m.editFocus < 0 {
			m.editFocus = 2
		}
		return m.updateEditFocus()

	case "up":
		if m.editFocus == 2 {
			m.editFocus = 1
			return m.updateEditFocus()
		}

	case "down":
		if m.editFocus == 0 {
			m.editFocus = 1
			return m.updateEditFocus()
		}

	case "enter":
		if m.editFocus == 0 {
			m.editFocus = 1
			return m.updateEditFocus()
		}
	}

	var cmd tea.Cmd
	switch m.editFocus {
	case 0:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case 1:
		m.textarea, cmd = m.textarea.Update(msg)
	case 2:
		m.tagsInput, cmd = m.tagsInput.Update(msg)
	}
	return m, cmd
}

func (m journalViewModel) updateEditFocus() (journalViewModel, tea.Cmd) {
	var cmds []tea.Cmd
	if m.editFocus == 0 {
		cmds = append(cmds, m.titleInput.Focus())
		m.textarea.Blur()
		m.tagsInput.Blur()
	} else if m.editFocus == 1 {
		m.titleInput.Blur()
		cmds = append(cmds, m.textarea.Focus())
		m.tagsInput.Blur()
	} else {
		m.titleInput.Blur()
		m.textarea.Blur()
		cmds = append(cmds, m.tagsInput.Focus())
	}
	return m, tea.Batch(cmds...)
}

func (m journalViewModel) handleSearchKeys(msg tea.KeyMsg) (journalViewModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.subMode = journalModeView
		m.searchMatches = nil
		m.searchCursor = 0
		m.lastSearchQuery = ""
		return m, nil

	case "ctrl+v":
		pasted, err := PasteIntoTextInput(&m.searchInput)
		if err == nil && pasted != "" {
			q := strings.TrimSpace(m.searchInput.Value())
			if q != m.lastSearchQuery && q != "" {
				m.loading = true
				m.lastSearchQuery = q
				m.searchCursor = 0
				client := m.apiClient
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					resp, err := client.SearchJournal(ctx, q)
					if err != nil {
						return journalErrMsg{Err: err}
					}
					return journalSearchLoadedMsg{Matches: resp.Matches}
				}
			}
		}
		return m, nil

	case "ctrl+c":
		if len(m.searchMatches) > 0 && m.searchCursor >= 0 && m.searchCursor < len(m.searchMatches) {
			match := m.searchMatches[m.searchCursor]
			text := match.Title
			if match.Snippet != "" {
				if text != "" {
					text += "\n"
				}
				text += match.Snippet
			}
			_ = CopyToClipboard(text)
		} else if m.searchInput.Value() != "" {
			_ = CopyToClipboard(m.searchInput.Value())
		}
		return m, nil

	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil

	case "down":
		if m.searchCursor < len(m.searchMatches)-1 {
			m.searchCursor++
		}
		return m, nil

	case "tab":
		if len(m.searchMatches) > 0 {
			m.searchCursor = (m.searchCursor + 1) % len(m.searchMatches)
		}
		return m, nil

	case "enter":
		query := strings.TrimSpace(m.searchInput.Value())

		// If user has already searched and presses Enter on the selected match:
		if len(m.searchMatches) > 0 && query == m.lastSearchQuery && m.searchCursor >= 0 && m.searchCursor < len(m.searchMatches) {
			match := m.searchMatches[m.searchCursor]
			parsedDate, err := time.Parse(time.DateOnly, match.Date)
			if err == nil {
				m.date = parsedDate
				m.activeNoteID = match.ID
				m.subMode = journalModeView
				m.loading = true
				m.searchMatches = nil
				m.searchCursor = 0
				m.lastSearchQuery = ""
				return m, m.loadJournalCmd(m.date)
			}
		}

		// Otherwise, run a new search
		if query != "" {
			m.loading = true
			m.lastSearchQuery = query
			m.searchCursor = 0
			client := m.apiClient
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				resp, err := client.SearchJournal(ctx, query)
				if err != nil {
					return journalErrMsg{Err: err}
				}
				return journalSearchLoadedMsg{Matches: resp.Matches}
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m journalViewModel) handleDeleteConfirmKeys(msg tea.KeyMsg) (journalViewModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if len(m.entries) == 0 {
			m.subMode = journalModeView
			return m, nil
		}
		entryID := m.entries[m.entryCursor].ID
		client := m.apiClient
		m.loading = true
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := client.DeleteJournalEntry(ctx, entryID)
			if err != nil {
				return journalErrMsg{Err: err}
			}
			return journalDeletedMsg{}
		}
	case "n", "N", "esc":
		m.subMode = journalModeView
		return m, nil
	}
	return m, nil
}

func (m journalViewModel) saveJournalCmd(title, body, tags string, noteID string) tea.Cmd {
	client := m.apiClient
	dateStr := m.date.Format(time.DateOnly)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		entry, err := client.SaveJournal(ctx, models.SaveJournalRequest{
			ID:    noteID,
			Date:  dateStr,
			Title: title,
			Body:  body,
			Tags:  tags,
		})
		if err != nil {
			return journalErrMsg{Err: err}
		}
		return journalSavedMsg{Entry: entry}
	}
}

func (m journalViewModel) openExternalEditor() (journalViewModel, tea.Cmd) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "notepad"
		if _, err := exec.LookPath("nano"); err == nil {
			editor = "nano"
		} else if _, err := exec.LookPath("vim"); err == nil {
			editor = "vim"
		}
	}

	tmpDir := os.TempDir()
	fileName := fmt.Sprintf("journal-%s-%d.md", m.date.Format(time.DateOnly), time.Now().Unix())
	tmpFile := filepath.Join(tmpDir, fileName)

	initialContent := ""
	if len(m.entries) > 0 && m.entryCursor < len(m.entries) {
		entry := m.entries[m.entryCursor]
		m.activeNoteID = entry.ID
		if entry.Title != "" {
			initialContent = fmt.Sprintf("# %s\n\n%s", entry.Title, entry.Body)
		} else {
			initialContent = entry.Body
		}
	} else {
		m.activeNoteID = ""
	}

	if err := os.WriteFile(tmpFile, []byte(initialContent), 0600); err != nil {
		m.errMsg = fmt.Sprintf("Gagal membuat file sementara: %v", err)
		return m, nil
	}

	c := exec.Command(editor, tmpFile)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err, tempFile: tmpFile}
	})
}

func parseEditorContent(content string) (title string, body string) {
	lines := strings.Split(content, "\n")
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	if idx < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[idx]), "# ") {
		title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[idx]), "# "))
		idx++
		if idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
			idx++
		}
		body = strings.Join(lines[idx:], "\n")
		return title, body
	}
	return "", content
}

func (m journalViewModel) View() string {
	var b strings.Builder

	// 1. Date Navigation Bar
	todayStr := time.Now().Format(time.DateOnly)
	dateLabel := m.date.Format("Monday, 02 Jan 2006")
	if m.date.Format(time.DateOnly) == todayStr {
		dateLabel += " (Hari Ini)"
	}
	navText := fmt.Sprintf("◀ [h]  %s  [l] ▶   •   [t] Hari Ini", dateLabel)
	if m.errMsg != "" {
		navText += "   " + ErrorToastStyle.Render("! "+m.errMsg)
	} else if m.statusMsg != "" {
		navText += "   " + SuccessToastStyle.Render("✓ "+m.statusMsg)
	}
	b.WriteString(DateNavStyle.Render(navText) + "\n")

	// 2. Modals and Sub-modes
	if m.subMode == journalModeEdit {
		var modal strings.Builder
		modalTitle := "TAMBAH CATATAN JURNAL (" + dateLabel + ")"
		if m.activeNoteID != "" {
			modalTitle = "EDIT CATATAN JURNAL (" + dateLabel + ")"
		}
		modal.WriteString(ModalTitleStyle.Render(modalTitle) + "\n\n")
		modal.WriteString(m.titleInput.View() + "\n\n")
		modal.WriteString(m.textarea.View() + "\n\n")
		modal.WriteString(m.tagsInput.View() + "\n\n")
		modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Tab] Pindah Field  •  [Ctrl+V] Tempel  •  [Ctrl+S] Simpan  •  [Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	if m.subMode == journalModeSearch {
		var modal strings.Builder
		modalTitle := "PENCARIAN JURNAL (JUDUL, TAGS, & ISI)"
		modal.WriteString(ModalTitleStyle.Render(modalTitle) + "\n")
		modal.WriteString(m.searchInput.View() + "\n")

		if m.loading {
			modal.WriteString(m.spinner.View() + " Mencari catatan...\n")
		} else if len(m.searchMatches) > 0 {
			modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).
				Render(fmt.Sprintf("Ditemukan %d entri (Gunakan [↑/↓ atau Tab] lalu [Enter] untuk buka):", len(m.searchMatches))) + "\n")
			for i, match := range m.searchMatches {
				cursor := "  "
				style := NormalItemStyle
				if i == m.searchCursor {
					cursor = "▶ "
					style = SelectedItemStyle
				}
				var itemParts []string
				itemParts = append(itemParts, fmt.Sprintf("[%s]", match.Date))
				if match.Title != "" {
					itemParts = append(itemParts, "📌 "+match.Title)
				}
				if match.Tags != "" {
					itemParts = append(itemParts, "🏷 "+match.Tags)
				}
				if match.Snippet != "" && match.Snippet != match.Title {
					snippetClean := strings.ReplaceAll(match.Snippet, "\n", " ")
					maxSnippetLen := max(20, m.width-50)
					if len(snippetClean) > maxSnippetLen {
						snippetClean = snippetClean[:maxSnippetLen-3] + "..."
					}
					itemParts = append(itemParts, "— "+snippetClean)
				}
				matchLine := fmt.Sprintf("%s%s", cursor, strings.Join(itemParts, "  "))
				modal.WriteString(style.Render(matchLine) + "\n")
			}
		} else if m.searchInput.Value() != "" && m.lastSearchQuery != "" {
			modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("Tidak ada catatan yang cocok.") + "\n")
		}
		modal.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Enter] Cari/Buka  •  [↑/↓ atau Tab] Pilih Hasil  •  [Esc] Kembali"))
		return ModalBoxStyle.Width(max(50, m.width-8)).Render(modal.String())
	}

	if m.subMode == journalModeConfirmDelete {
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render("HAPUS CATATAN JURNAL?") + "\n\n")
		noteName := fmt.Sprintf("Catatan #%d", m.entryCursor+1)
		if len(m.entries) > 0 && m.entries[m.entryCursor].Title != "" {
			noteName = fmt.Sprintf("%q", m.entries[m.entryCursor].Title)
		}
		modal.WriteString(fmt.Sprintf("Yakin ingin menghapus catatan %s untuk tanggal %s?\n\n", noteName, dateLabel))
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("[y] Ya, Hapus  •  [n/Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	// 3. Viewport Display (Side-by-side or empty state)
	listWidth := 26
	if m.width > 90 {
		listWidth = 30
	}
	vpWidth := max(20, m.width-listWidth-10)
	vpHeight := max(5, m.height-8)

	if m.loading && len(m.entries) == 0 {
		b.WriteString(padBlock(m.spinner.View()+" Memuat jurnal...", max(40, m.width-8), vpHeight))
	} else if len(m.entries) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).
			Render("Belum ada catatan jurnal untuk tanggal ini.\n\nTekan [a] untuk membuat catatan baru atau [e] untuk membuka editor eksternal ($EDITOR).")
		b.WriteString(padBlock(emptyMsg, max(40, m.width-8), vpHeight))
	} else {
		var listBuilder strings.Builder
		listHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Background(ColorDarkBg).
			Padding(0, 1).
			Render(fmt.Sprintf("📖 CATATAN (%d)", len(m.entries)))
		listBuilder.WriteString(listHeader + "\n")

		for i, entry := range m.entries {
			isCursor := (i == m.entryCursor)
			title := strings.TrimSpace(entry.Title)
			if title == "" {
				firstLine := strings.Split(strings.TrimSpace(entry.Body), "\n")[0]
				if len(firstLine) > listWidth-8 {
					firstLine = firstLine[:listWidth-8] + "..."
				}
				if firstLine != "" {
					title = firstLine
				} else {
					title = fmt.Sprintf("Catatan #%d", i+1)
				}
			}
			if len(title) > listWidth-8 {
				title = title[:listWidth-11] + "..."
			}

			if isCursor {
				itemText := fmt.Sprintf("▶ %d. %s", i+1, title)
				listBuilder.WriteString(SelectedItemStyle.Width(listWidth - 2).Render(itemText) + "\n")
			} else {
				itemText := fmt.Sprintf("  %d. %s", i+1, title)
				listBuilder.WriteString(NormalItemStyle.Width(listWidth - 2).Render(itemText) + "\n")
			}
		}
		listBuilder.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("+ [a] Tambah"))

		borderColor := ColorPrimary

		leftStyle := lipgloss.NewStyle().
			Width(listWidth).
			Height(vpHeight).
			MaxHeight(vpHeight).
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(borderColor).
			PaddingRight(1)

		leftView := leftStyle.Render(listBuilder.String())

		vpView := m.viewport.View()
		rightClean := padBlock(vpView, vpWidth, vpHeight)

		layout := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightClean)
		b.WriteString(layout)
	}

	return b.String()
}
