package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/api"
	"todoapp/internal/client/config"
	"todoapp/internal/client/notify"
	"todoapp/internal/client/ws"
	"todoapp/internal/models"
)

type appState int

const (
	stateLogin appState = iota
	stateMain
)

type tabIndex int

const (
	tabJournal tabIndex = iota
	tabTodo
	tabChat
)

// AppModel is the root Bubble Tea model.
type AppModel struct {
	state          appState
	activeTab      tabIndex
	apiClient      *api.Client
	cfg            config.Config
	user           models.User
	loginView      loginModel
	todoView       todoViewModel
	journalView    journalViewModel
	chatView       chatViewModel
	wsClient       *ws.Client
	wsMsgCh        chan tea.Msg
	width          int
	height         int
	commandPalette bool
	paletteInput   textinput.Model
	globalToast    string
}

func waitForWSEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// New creates and initializes the root TUI model.
func New(cfg config.Config) AppModel {
	if cfg.Theme != "" {
		ApplyTheme(cfg.Theme)
	}
	notify.SetEnabled(cfg.IsNotificationsEnabled())
	client := api.New(cfg.ServerURL, cfg.Token)
	loginM := newLoginModel(client, cfg)
	todoM := newTodoViewModel(client)
	journalM := newJournalViewModel(client)
	chatM := newChatViewModel(client, 0, cfg.Username)

	wsMsgCh := make(chan tea.Msg, 256)
	wsClient := ws.NewClient(
		cfg.ServerURL,
		cfg.Token,
		func(evt models.WSEvent) {
			select {
			case wsMsgCh <- wsEventMsg{Event: evt}:
			default:
			}
		},
		func(online bool) {
			select {
			case wsMsgCh <- wsStatusMsg{Online: online}:
			default:
			}
		},
	)

	pInput := textinput.New()
	pInput.Prompt = ": "
	pInput.Placeholder = "command (e.g. :today, :add <title>, :logout, :q)"

	state := stateLogin
	// If token exists, default to stateMain (validated in Init).
	if cfg.Token != "" {
		state = stateMain
	}

	return AppModel{
		state:        state,
		activeTab:    tabJournal, // default to Journal tab
		apiClient:    client,
		cfg:          cfg,
		loginView:    loginM,
		todoView:     todoM,
		journalView:  journalM,
		chatView:     chatM,
		wsClient:     wsClient,
		wsMsgCh:      wsMsgCh,
		paletteInput: pInput,
	}
}

type authCheckMsg struct {
	User models.User
	Err  error
}

func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.loginView.Init(),
		waitForWSEvent(m.wsMsgCh),
	}

	if m.cfg.Token != "" {
		client := m.apiClient
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			u, err := client.Me(ctx)
			return authCheckMsg{User: u, Err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmdLogin, cmdTodo, cmdJournal, cmdChat tea.Cmd
		m.loginView, cmdLogin = m.loginView.Update(msg)
		m.todoView, cmdTodo = m.todoView.Update(msg)
		m.journalView, cmdJournal = m.journalView.Update(msg)
		m.chatView, cmdChat = m.chatView.Update(msg)
		return m, tea.Batch(cmdLogin, cmdTodo, cmdJournal, cmdChat)

	case authCheckMsg:
		if msg.Err != nil {
			// Token expired or invalid, switch to login view
			m.state = stateLogin
			m.cfg.Token = ""
			m.cfg.Username = ""
			m.user = models.User{}
			m.apiClient.SetToken("")
			if m.wsClient != nil {
				m.wsClient.UpdateToken("")
			}
			m.loginView.Reset()
			m.chatView = newChatViewModel(m.apiClient, 0, "")
			m.todoView = newTodoViewModel(m.apiClient)
			m.journalView = newJournalViewModel(m.apiClient)
			m.journalView.width = m.width
			m.journalView.height = m.height
			m.journalView.ensureViewportDimensions()
			_ = config.Save(m.cfg)
			m.globalToast = "Sesi telah berakhir, silakan login kembali."
			return m, nil
		}
		m.user = msg.User
		m.state = stateMain
		m.chatView.SetCurrentUser(msg.User.ID, msg.User.Username)
		if m.wsClient != nil {
			m.wsClient.UpdateToken(m.cfg.Token)
		}
		return m, tea.Batch(m.todoView.Init(), m.journalView.Init(), m.chatView.Init())

	case loginSuccessMsg:
		m.user = msg.User
		m.state = stateMain
		m.cfg.Token = msg.Token
		m.cfg.Username = msg.User.Username
		m.cfg.ServerURL = m.apiClient.BaseURL()
		_ = config.Save(m.cfg)
		m.apiClient.SetToken(msg.Token)

		// Reset login loading and clear password
		m.loginView.loading = false
		m.loginView.errText = ""
		m.loginView.inputs[1].SetValue("")

		// Reinitialize views with clean state for the new user
		m.chatView = newChatViewModel(m.apiClient, msg.User.ID, msg.User.Username)
		m.chatView.width = m.width
		m.chatView.height = m.height
		sidebarWidth := 24
		m.chatView.viewport.Width = max(28, m.width-sidebarWidth-12)
		m.chatView.viewport.Height = max(6, m.height-14)
		m.chatView.msgInput.Width = max(20, m.chatView.viewport.Width-4)

		m.todoView = newTodoViewModel(m.apiClient)
		m.todoView.width = m.width
		m.todoView.height = m.height

		m.journalView = newJournalViewModel(m.apiClient)
		m.journalView.width = m.width
		m.journalView.height = m.height
		m.journalView.ensureViewportDimensions()

		if m.wsClient != nil {
			m.wsClient.UpdateToken(msg.Token)
		}
		m.globalToast = fmt.Sprintf("Selamat datang, %s!", msg.User.Username)
		return m, tea.Batch(m.todoView.Init(), m.journalView.Init(), m.chatView.Init())

	case wsStatusMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, tea.Batch(cmd, waitForWSEvent(m.wsMsgCh))

	case wsEventMsg:
		switch msg.Event.Type {
		case "reminder":
			var rem models.WSReminder
			b, _ := json.Marshal(msg.Event.Payload)
			if err := json.Unmarshal(b, &rem); err == nil {
				notify.NotifyReminder(rem.Title, rem.Priority)
				m.globalToast = fmt.Sprintf("⏰ PENGINGAT: %s", rem.Title)
				todayStr := time.Now().Format(time.DateOnly)
				if rem.Date == todayStr && m.activeTab == tabTodo {
					return m, tea.Batch(waitForWSEvent(m.wsMsgCh), m.todoView.loadTodosCmd(m.todoView.date))
				}
			}
			return m, waitForWSEvent(m.wsMsgCh)

		case "message":
			var chatMsg models.Message
			b, _ := json.Marshal(msg.Event.Payload)
			if err := json.Unmarshal(b, &chatMsg); err == nil {
				if chatMsg.UserID != m.user.ID {
					if m.activeTab != tabChat || m.chatView.activeRoomID != chatMsg.RoomID {
						notify.NotifyChatMessage(chatMsg.Username, chatMsg.Body)
					}
				}
			}
		}
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, tea.Batch(cmd, waitForWSEvent(m.wsMsgCh))

	case todosLoadedMsg, todoOpSuccessMsg, todoOpErrMsg:
		var cmd tea.Cmd
		m.todoView, cmd = m.todoView.Update(msg)
		return m, cmd

	case journalLoadedMsg, journalSavedMsg, journalDeletedMsg, journalErrMsg, journalSearchLoadedMsg, editorFinishedMsg:
		var cmd tea.Cmd
		m.journalView, cmd = m.journalView.Update(msg)
		return m, cmd

	case chatRoomsLoadedMsg, chatMessagesLoadedMsg, chatOlderMessagesLoadedMsg, chatMessageSentMsg, chatErrMsg:
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// If user is inside an edit/input modal, let child view copy text or cancel without killing app
			if m.state == stateMain && (m.journalView.subMode == journalModeEdit ||
				m.todoView.subMode == subModeAdd || m.todoView.subMode == subModeEdit ||
				m.todoView.subMode == subModeBatch ||
				(m.activeTab == tabChat && m.chatView.focus == focusInput)) {
				// Route to child view
				break
			}
			if m.wsClient != nil {
				m.wsClient.Close()
			}
			return m, tea.Quit
		}

		// Handle Command Palette
		if m.commandPalette {
			switch msg.String() {
			case "esc":
				m.commandPalette = false
				m.paletteInput.Blur()
				return m, nil
			case "ctrl+v":
				_, _ = PasteIntoTextInput(&m.paletteInput)
				return m, nil
			case "ctrl+c":
				if m.paletteInput.Value() != "" {
					_ = CopyToClipboard(m.paletteInput.Value())
				}
				return m, nil
			case "enter":
				cmdStr := strings.TrimSpace(m.paletteInput.Value())
				m.commandPalette = false
				m.paletteInput.Blur()
				return m.execCommand(cmdStr)
			}
			var cmd tea.Cmd
			m.paletteInput, cmd = m.paletteInput.Update(msg)
			return m, cmd
		}

		canSwitchTabs := (m.todoView.subMode == subModeNormal &&
			m.journalView.subMode == journalModeView &&
			(m.activeTab != tabChat || (m.chatView.subMode == chatModeNormal && m.chatView.focus != focusInput)))

		// Global command palette trigger
		if m.state == stateMain && (msg.String() == ":" || msg.String() == "ctrl+p") {
			if canSwitchTabs {
				m.commandPalette = true
				m.paletteInput.SetValue("")
				m.paletteInput.Focus()
				return m, textinput.Blink
			}
		}

		// Tab Switching (when not in text input modal)
		if m.state == stateMain && canSwitchTabs {
			switch msg.String() {
			case "1":
				m.activeTab = tabJournal
				return m, m.journalView.loadJournalCmd(m.journalView.date)
			case "2":
				m.activeTab = tabTodo
				return m, m.todoView.loadTodosCmd(m.todoView.date)
			case "3":
				m.activeTab = tabChat
				if len(m.chatView.rooms) == 0 {
					return m, m.chatView.loadRoomsCmd()
				}
				return m, nil
			case "tab":
				m.activeTab = (m.activeTab + 1) % 3
				switch m.activeTab {
				case tabJournal:
					return m, m.journalView.loadJournalCmd(m.journalView.date)
				case tabTodo:
					return m, m.todoView.loadTodosCmd(m.todoView.date)
				case tabChat:
					if len(m.chatView.rooms) == 0 {
						return m, m.chatView.loadRoomsCmd()
					}
					return m, nil
				}
				return m, nil
			case "q":
				if m.wsClient != nil {
					m.wsClient.Close()
				}
				return m, tea.Quit
			}
		}
	}

	// Route updates to active sub-model
	if m.state == stateLogin {
		var cmd tea.Cmd
		m.loginView, cmd = m.loginView.Update(msg)
		return m, cmd
	}

	if m.activeTab == tabJournal {
		var cmd tea.Cmd
		m.journalView, cmd = m.journalView.Update(msg)
		return m, cmd
	}

	if m.activeTab == tabTodo {
		var cmd tea.Cmd
		m.todoView, cmd = m.todoView.Update(msg)
		return m, cmd
	}

	if m.activeTab == tabChat {
		var cmd tea.Cmd
		m.chatView, cmd = m.chatView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m AppModel) execCommand(cmdStr string) (tea.Model, tea.Cmd) {
	cmdStr = strings.TrimPrefix(cmdStr, ":")
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "q", "quit", "exit":
		if m.wsClient != nil {
			m.wsClient.Close()
		}
		return m, tea.Quit

	case "logout":
		m.state = stateLogin
		m.cfg.Token = ""
		m.cfg.Username = ""
		m.user = models.User{}
		m.apiClient.SetToken("")
		if m.wsClient != nil {
			m.wsClient.UpdateToken("")
		}
		_ = config.Save(m.cfg)

		m.loginView.Reset()

		// Reinitialize views with clean state
		m.chatView = newChatViewModel(m.apiClient, 0, "")
		m.chatView.width = m.width
		m.chatView.height = m.height
		sidebarWidth := 26
		m.chatView.viewport.Width = max(30, m.width-sidebarWidth-8)
		m.chatView.viewport.Height = max(6, m.height-12)
		m.chatView.msgInput.Width = max(25, m.chatView.viewport.Width-4)

		m.todoView = newTodoViewModel(m.apiClient)
		m.todoView.width = m.width
		m.todoView.height = m.height

		m.journalView = newJournalViewModel(m.apiClient)
		m.journalView.width = m.width
		m.journalView.height = m.height

		m.globalToast = "Anda telah logout."
		return m, nil

	case "today":
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		if m.activeTab == tabJournal {
			m.journalView.date = today
			return m, m.journalView.loadJournalCmd(today)
		}
		m.todoView.date = today
		return m, m.todoView.loadTodosCmd(today)

	case "add":
		if len(parts) > 1 {
			m.activeTab = tabTodo
			title := strings.Join(parts[1:], " ")
			client := m.apiClient
			dateStr := m.todoView.date.Format(time.DateOnly)
			prio := uint8(2)
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := client.CreateTodo(ctx, models.CreateTodoRequest{
					Date:     dateStr,
					Title:    title,
					Priority: &prio,
				})
				if err != nil {
					return todoOpErrMsg{Err: err}
				}
				return todoOpSuccessMsg{Message: "Task berhasil ditambahkan!"}
			}
		}

	case "batch":
		m.activeTab = tabTodo
		var cmd tea.Cmd
		m.todoView, cmd = m.todoView.openBatchModal()
		return m, cmd

	case "search", "find":
		m.activeTab = tabJournal
		m.journalView.subMode = journalModeSearch
		if len(parts) > 1 {
			query := strings.Join(parts[1:], " ")
			m.journalView.searchInput.SetValue(query)
			client := m.apiClient
			m.journalView.loading = true
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
		m.journalView.searchInput.SetValue("")
		m.journalView.searchMatches = nil
		m.journalView.searchInput.Focus()
		return m, textinput.Blink

	case "refresh", "r":
		if m.activeTab == tabJournal {
			return m, m.journalView.loadJournalCmd(m.journalView.date)
		} else if m.activeTab == tabChat {
			return m, m.chatView.loadRoomsCmd()
		}
		return m, m.todoView.loadTodosCmd(m.todoView.date)

	case "notify":
		if len(parts) > 1 {
			switch parts[1] {
			case "on", "enable", "1":
				notify.SetEnabled(true)
				en := true
				m.cfg.NotificationsEnabled = &en
				_ = config.Save(m.cfg)
				m.globalToast = "Notifikasi desktop diaktifkan."
				return m, nil
			case "off", "disable", "0":
				notify.SetEnabled(false)
				en := false
				m.cfg.NotificationsEnabled = &en
				_ = config.Save(m.cfg)
				m.globalToast = "Notifikasi desktop dinonaktifkan."
				return m, nil
			}
		}
		newVal := !notify.IsEnabled()
		notify.SetEnabled(newVal)
		m.cfg.NotificationsEnabled = &newVal
		_ = config.Save(m.cfg)
		if newVal {
			m.globalToast = "Notifikasi desktop diaktifkan."
		} else {
			m.globalToast = "Notifikasi desktop dinonaktifkan."
		}
		return m, nil

	case "theme":
		if len(parts) > 1 {
			themeName := strings.ToLower(parts[1])
			if ApplyTheme(themeName) {
				m.cfg.Theme = CurrentTheme()
				_ = config.Save(m.cfg)
				if m.chatView.activeRoomID > 0 {
					m.chatView.renderMessages(m.chatView.activeRoomID)
				}
				m.globalToast = fmt.Sprintf("Tema berhasil diubah ke '%s'.", CurrentTheme())
				return m, nil
			}
			m.globalToast = fmt.Sprintf("Tema '%s' tidak ditemukan. Pilihan: %s", parts[1], strings.Join(ListThemes(), ", "))
			return m, nil
		}
		m.globalToast = fmt.Sprintf("Tema aktif: '%s'. Pilihan: %s (gunakan :theme <nama>)", CurrentTheme(), strings.Join(ListThemes(), ", "))
		return m, nil

	case "copy", "c", "yank":
		if m.activeTab == tabJournal {
			if len(m.journalView.entries) > 0 && m.journalView.entryCursor < len(m.journalView.entries) {
				entry := m.journalView.entries[m.journalView.entryCursor]
				text := entry.Body
				if entry.Title != "" {
					text = entry.Title + "\n\n" + text
				}
				_ = CopyToClipboard(text)
				m.globalToast = "📋 Catatan jurnal disalin ke clipboard!"
			} else {
				m.globalToast = "Tidak ada catatan jurnal untuk disalin."
			}
			return m, nil
		} else if m.activeTab == tabTodo {
			if len(m.todoView.todos) > 0 && m.todoView.cursor < len(m.todoView.todos) {
				item := m.todoView.todos[m.todoView.cursor]
				status := "[ ]"
				if item.Done {
					status = "[x]"
				}
				text := fmt.Sprintf("%s %s (Prioritas: %d)", status, item.Title, item.Priority)
				if item.Description != nil && *item.Description != "" {
					text += "\nCatatan: " + *item.Description
				}
				_ = CopyToClipboard(text)
				m.globalToast = "📋 Task disalin ke clipboard!"
			} else {
				m.globalToast = "Tidak ada task untuk disalin."
			}
			return m, nil
		} else if m.activeTab == tabChat {
			if m.chatView.activeRoomID > 0 {
				msgs := m.chatView.messages[m.chatView.activeRoomID]
				if len(msgs) > 0 {
					last := msgs[len(msgs)-1]
					_ = CopyToClipboard(fmt.Sprintf("%s: %s", last.Username, last.Body))
					m.globalToast = "📋 Pesan chat disalin ke clipboard!"
				}
			}
			return m, nil
		}

	case "paste", "p":
		text, err := ReadFromClipboard()
		if err != nil || text == "" {
			m.globalToast = "Clipboard kosong atau tidak dapat diakses."
			return m, nil
		}
		if m.activeTab == tabTodo {
			m.activeTab = tabTodo
			client := m.apiClient
			dateStr := m.todoView.date.Format(time.DateOnly)
			prio := uint8(2)
			firstLine := strings.Split(text, "\n")[0]
			var desc *string
			if strings.Contains(text, "\n") {
				lines := strings.SplitN(text, "\n", 2)
				firstLine = strings.TrimSpace(lines[0])
				rest := strings.TrimSpace(lines[1])
				if rest != "" {
					desc = &rest
				}
			}
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := client.CreateTodo(ctx, models.CreateTodoRequest{
					Date:        dateStr,
					Title:       firstLine,
					Description: desc,
					Priority:    &prio,
				})
				if err != nil {
					return todoOpErrMsg{Err: err}
				}
				return todoOpSuccessMsg{Message: "Task dari clipboard berhasil ditambahkan!"}
			}
		}

	case "help":
		m.globalToast = "Shortcut: [1/2/3] Tabs, [Enter/i] Chat, [n] Channel, [u] DM, [Space] Todo, [c/y] Copy, [Ctrl+V] Paste, [:] Cmd (:copy, :paste, :theme, :notify, :today, :add, :logout)"
		return m, nil
	}

	m.globalToast = fmt.Sprintf("Perintah tidak dikenal: %s", parts[0])
	return m, nil
}

func (m AppModel) View() string {
	if m.state == stateLogin {
		return m.loginView.View()
	}

	// 1. Header & Tabs
	userBadge := lipgloss.NewStyle().Foreground(ColorSecondary).Render("👤 " + m.user.Username)
	netStatus := lipgloss.NewStyle().Foreground(ColorSuccess).Render("● Online")
	if !m.chatView.wsOnline {
		netStatus = lipgloss.NewStyle().Foreground(ColorOrange).Render("○ Offline")
	}
	appTitle := AppTitleStyle.Render("todoapp") + "  " + userBadge + " " + netStatus

	tab1 := InactiveTabStyle.Render("1. Jurnal")
	tab2 := InactiveTabStyle.Render("2. To-do")
	tab3 := InactiveTabStyle.Render("3. Chat")

	switch m.activeTab {
	case tabJournal:
		tab1 = ActiveTabStyle.Render("1. Jurnal")
	case tabTodo:
		tab2 = ActiveTabStyle.Render("2. To-do")
	case tabChat:
		tab3 = ActiveTabStyle.Render("3. Chat")
	}

	tabsRow := lipgloss.JoinHorizontal(lipgloss.Bottom, tab1, tab2, tab3)
	headerRow := lipgloss.JoinHorizontal(lipgloss.Center, appTitle, "   ", tabsRow)

	// 2. Main Content
	var content string
	switch m.activeTab {
	case tabJournal:
		content = m.journalView.View()
	case tabTodo:
		content = m.todoView.View()
	case tabChat:
		content = m.chatView.View()
	}

	footerLines := 1
	if m.globalToast != "" {
		footerLines = 2
	}
	innerBoxHeight := max(5, m.height-2-footerLines-2)
	innerBoxWidth := max(26, m.width-6)

	contentBox := ContentBoxStyle.
		Width(innerBoxWidth).
		Height(innerBoxHeight).
		MaxHeight(innerBoxHeight).
		Render(content)

	// 3. Command Palette Modal (if open)
	if m.commandPalette {
		paletteBox := ModalBoxStyle.Width(m.width - 6).Render(
			lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("COMMAND PALETTE") + "\n\n" +
				m.paletteInput.View() + "\n\n" +
				lipgloss.NewStyle().Foreground(ColorSubtle).Render("Contoh: :copy • :paste • :theme <nama> • :today • :search <kata> • :add <task> • :logout • :quit • [Esc] Batal"),
		)
		contentBox = lipgloss.Place(m.width, m.height-6, lipgloss.Center, lipgloss.Center, paletteBox)
	}

	// 4. Footer & Shortcut Status Bar
	var shortcuts string
	if m.activeTab == tabJournal {
		switch m.journalView.subMode {
		case journalModeEdit:
			shortcuts = "[Ctrl+S] Simpan  •  [Ctrl+V] Tempel  •  [Tab] Pindah Field  •  [Esc] Batal"
		case journalModeSearch:
			shortcuts = "[Enter] Cari/Buka  •  [Ctrl+V] Tempel  •  [↑/↓ atau Tab] Pilih Hasil  •  [Esc] Kembali"
		case journalModeConfirmDelete:
			shortcuts = "[y] Ya, Hapus  •  [n/Esc] Batal"
		default:
			shortcuts = "[Enter/i] Edit  •  [a] Tambah  •  [c/y] Salin  •  [j/k atau ↑/↓] Pilih  •  [PgUp/PgDn] Scroll  •  [h/l] Tanggal  •  [d] Hapus  •  [/] Cari"
		}
	} else if m.activeTab == tabTodo {
		if m.todoView.subMode == subModeDetail {
			shortcuts = "[c/y] Salin  •  [Space] Ubah Status  •  [e] Edit  •  [d] Hapus  •  [j/k] Lainnya  •  [h/l] Tanggal  •  [Esc] Tutup"
		} else if m.todoView.subMode != subModeNormal {
			shortcuts = "[Tab] Pindah Field  •  [Ctrl+V] Tempel  •  [Enter] Simpan  •  [Esc] Batal"
		} else {
			shortcuts = "[Space] Selesai  •  [Enter] Detail  •  [c/y] Salin  •  [j/k] Pilih  •  [h/l] Tanggal  •  [a] Tambah  •  [b] Batch  •  [e] Edit  •  [d] Hapus"
		}
	} else if m.activeTab == tabChat {
		if m.chatView.subMode != chatModeNormal {
			shortcuts = "[Ctrl+V] Tempel  •  [Enter] Simpan  •  [Esc] Batal"
		} else if m.chatView.focus == focusInput {
			shortcuts = "[Enter] Kirim  •  [Ctrl+V] Tempel  •  [PgUp/PgDn] Scroll  •  [Esc] Mode Baca  •  [Ctrl+K] Sidebar"
		} else if m.chatView.focus == focusMessages {
			shortcuts = "[c/y] Salin Pesan  •  [j/k atau ↑/↓] Scroll  •  [PgUp/PgDn] Layar  •  [i/Enter] Ketik  •  [h/Esc] Sidebar"
		} else {
			shortcuts = "[Tab/1-2-3] Pindah Tab  •  [j/k] Pilih Room  •  [l/→] Chat  •  [n] Channel  •  [u] DM  •  [:] Menu  •  [q] Keluar"
		}
	} else {
		shortcuts = "[Tab/1-2-3] Pindah Tab  •  [:] Menu  •  [q] Keluar"
	}

	footer := StatusBarStyle.Width(m.width).MaxHeight(1).Render(shortcuts)
	if m.globalToast != "" {
		toastLine := lipgloss.NewStyle().Foreground(ColorHighlight).Render("📢 " + m.globalToast)
		footer = lipgloss.JoinVertical(lipgloss.Left, toastLine, footer)
	}

	finalView := lipgloss.JoinVertical(lipgloss.Left, headerRow, contentBox, footer)
	finalLines := strings.Split(finalView, "\n")
	if len(finalLines) > m.height && m.height > 0 {
		finalLines = finalLines[:m.height]
	}
	maxAllowedWidth := m.width - 1
	if maxAllowedWidth < 10 {
		maxAllowedWidth = m.width
	}
	for i, l := range finalLines {
		if lipgloss.Width(l) > maxAllowedWidth && maxAllowedWidth > 0 {
			finalLines[i] = lipgloss.NewStyle().MaxWidth(maxAllowedWidth).Render(l)
		}
	}
	return strings.Join(finalLines, "\n")
}
