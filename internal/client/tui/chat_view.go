package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/api"
	"todoapp/internal/models"
)

type chatFocusArea int

const (
	focusRooms chatFocusArea = iota
	focusMessages
	focusInput
)

type chatSubMode int

const (
	chatModeNormal chatSubMode = iota
	chatModeNewRoom
	chatModeNewDM
)

// Messages
type chatRoomsLoadedMsg struct {
	Rooms        []models.Room
	SelectRoomID int64
}

type chatMessagesLoadedMsg struct {
	RoomID   int64
	Messages []models.Message
	HasMore  bool
}

type chatOlderMessagesLoadedMsg struct {
	RoomID   int64
	Messages []models.Message
	HasMore  bool
}

type chatMessageSentMsg struct {
	Message models.Message
}

type chatErrMsg struct {
	Err error
}

type wsEventMsg struct {
	Event models.WSEvent
}

type wsStatusMsg struct {
	Online bool
}

type chatViewModel struct {
	apiClient       *api.Client
	currentUserID   int64
	currentUsername string
	rooms           []models.Room
	roomCursor      int
	activeRoomID    int64
	messages        map[int64][]models.Message // roomID -> messages
	hasMore         map[int64]bool             // roomID -> has more older messages
	loadingOlder    bool
	focus           chatFocusArea
	subMode         chatSubMode
	loading         bool
	wsOnline        bool
	onlineUsers     map[int64]bool
	spinner         spinner.Model
	viewport        viewport.Model
	msgInput        textinput.Model
	modalInput      textinput.Model
	errMsg          string
	statusMsg       string
	width           int
	height          int
}

func newChatViewModel(apiClient *api.Client, currentUserID int64, currentUsername string) chatViewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	vp := viewport.New(40, 15)
	vp.SetContent("Pilih channel atau percakapan di sebelah kiri...")

	mInput := textinput.New()
	mInput.Placeholder = "Ketik pesan chat lalu tekan [Enter]..."
	mInput.Prompt = "💬 "
	mInput.CharLimit = 1000

	modalIn := textinput.New()
	modalIn.Placeholder = "Nama channel baru atau username teman..."
	modalIn.Prompt = "> "
	modalIn.CharLimit = 64

	return chatViewModel{
		apiClient:       apiClient,
		currentUserID:   currentUserID,
		currentUsername: currentUsername,
		rooms:           []models.Room{},
		roomCursor:      0,
		activeRoomID:    0,
		messages:        make(map[int64][]models.Message),
		hasMore:         make(map[int64]bool),
		loadingOlder:    false,
		focus:           focusRooms,
		subMode:         chatModeNormal,
		loading:         false,
		wsOnline:        false,
		onlineUsers:     make(map[int64]bool),
		spinner:         s,
		viewport:        vp,
		msgInput:        mInput,
		modalInput:      modalIn,
	}
}

func (m chatViewModel) Init() tea.Cmd {
	return tea.Batch(m.loadRoomsCmd(), m.spinner.Tick)
}

func (m chatViewModel) loadRoomsCmd() tea.Cmd {
	client := m.apiClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		rooms, err := client.ListRooms(ctx)
		if err != nil {
			return chatErrMsg{Err: err}
		}
		return chatRoomsLoadedMsg{Rooms: rooms}
	}
}

func (m chatViewModel) loadMessagesCmd(roomID int64) tea.Cmd {
	client := m.apiClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.ListMessages(ctx, roomID, 0, 100)
		if err != nil {
			return chatErrMsg{Err: err}
		}
		return chatMessagesLoadedMsg{RoomID: roomID, Messages: resp.Messages, HasMore: resp.HasMore}
	}
}

func (m chatViewModel) loadOlderMessagesCmd(roomID int64, beforeID int64) tea.Cmd {
	client := m.apiClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.ListMessages(ctx, roomID, beforeID, 50)
		if err != nil {
			return chatErrMsg{Err: err}
		}
		return chatOlderMessagesLoadedMsg{RoomID: roomID, Messages: resp.Messages, HasMore: resp.HasMore}
	}
}

func (m *chatViewModel) checkLoadOlder() tea.Cmd {
	if m.activeRoomID <= 0 || m.loadingOlder || !m.hasMore[m.activeRoomID] {
		return nil
	}
	msgs := m.messages[m.activeRoomID]
	if len(msgs) == 0 {
		return nil
	}
	if m.viewport.YOffset <= 2 {
		m.loadingOlder = true
		oldestID := msgs[0].ID
		return m.loadOlderMessagesCmd(m.activeRoomID, oldestID)
	}
	return nil
}

func appendIfNotExists(list []models.Message, msg models.Message) []models.Message {
	for _, existing := range list {
		if existing.ID == msg.ID {
			return list
		}
	}
	return append(list, msg)
}

func (m *chatViewModel) SetCurrentUser(userID int64, username string) {
	m.currentUserID = userID
	m.currentUsername = username
}

func (m *chatViewModel) SetWSOnline(online bool) {
	m.wsOnline = online
}

func (m chatViewModel) Update(msg tea.Msg) (chatViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		sidebarWidth := 24
		vpWidth := max(28, msg.Width-sidebarWidth-12)
		vpHeight := max(6, msg.Height-14)
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		m.msgInput.Width = max(20, vpWidth-4)
		if m.activeRoomID > 0 {
			m.renderMessages(m.activeRoomID)
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case wsStatusMsg:
		m.wsOnline = msg.Online
		return m, nil

	case wsEventMsg:
		switch msg.Event.Type {
		case "message":
			msgBytes, _ := json.Marshal(msg.Event.Payload)
			var message models.Message
			if err := json.Unmarshal(msgBytes, &message); err == nil {
				m.messages[message.RoomID] = appendIfNotExists(m.messages[message.RoomID], message)
				if message.RoomID == m.activeRoomID {
					atBottom := m.viewport.AtBottom()
					m.renderMessages(message.RoomID)
					if atBottom || m.focus == focusInput {
						m.viewport.GotoBottom()
					}
					client := m.apiClient
					roomID := message.RoomID
					msgID := message.ID
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						_ = client.MarkRead(ctx, roomID, msgID)
					}()
				} else {
					for i := range m.rooms {
						if m.rooms[i].ID == message.RoomID {
							m.rooms[i].UnreadCount++
							break
						}
					}
				}
			}
		case "presence":
			var p models.WSPresence
			msgBytes, _ := json.Marshal(msg.Event.Payload)
			if err := json.Unmarshal(msgBytes, &p); err == nil {
				m.onlineUsers[p.UserID] = p.Online
			}
		}
		return m, nil

	case chatRoomsLoadedMsg:
		m.loading = false
		m.rooms = msg.Rooms
		if len(m.rooms) == 0 {
			m.activeRoomID = 0
			m.roomCursor = 0
			m.viewport.SetContent(lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).
				Render("Belum ada channel atau percakapan.\n\nTekan [n] untuk membuat channel baru, atau [u] untuk DM teman."))
			return m, nil
		}

		found := false
		for i, r := range m.rooms {
			targetID := m.activeRoomID
			if msg.SelectRoomID > 0 {
				targetID = msg.SelectRoomID
			}
			if r.ID == targetID {
				m.activeRoomID = r.ID
				m.roomCursor = i
				found = true
				break
			}
		}

		if !found {
			m.activeRoomID = m.rooms[0].ID
			m.roomCursor = 0
		}

		return m, m.loadMessagesCmd(m.activeRoomID)

	case chatMessagesLoadedMsg:
		m.loading = false
		m.hasMore[msg.RoomID] = msg.HasMore
		existing := m.messages[msg.RoomID]
		for _, incoming := range msg.Messages {
			existing = appendIfNotExists(existing, incoming)
		}
		m.messages[msg.RoomID] = existing
		m.renderMessages(msg.RoomID)
		m.viewport.GotoBottom()

		// Mark as read
		if len(msg.Messages) > 0 {
			lastID := msg.Messages[len(msg.Messages)-1].ID
			client := m.apiClient
			roomID := msg.RoomID
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = client.MarkRead(ctx, roomID, lastID)
			}()
			// Clear local unread count
			for i := range m.rooms {
				if m.rooms[i].ID == roomID {
					m.rooms[i].UnreadCount = 0
				}
			}
		}
		return m, nil

	case chatOlderMessagesLoadedMsg:
		m.loadingOlder = false
		m.hasMore[msg.RoomID] = msg.HasMore
		if len(msg.Messages) > 0 {
			existing := m.messages[msg.RoomID]
			combined := append(msg.Messages, existing...)
			dedup := make([]models.Message, 0, len(combined))
			seen := make(map[int64]bool)
			for _, item := range combined {
				if !seen[item.ID] {
					seen[item.ID] = true
					dedup = append(dedup, item)
				}
			}
			m.messages[msg.RoomID] = dedup
			oldOffset := m.viewport.YOffset
			m.renderMessages(msg.RoomID)
			m.viewport.SetYOffset(oldOffset + len(msg.Messages)*3)
		}
		return m, nil

	case chatMessageSentMsg:
		m.loading = false
		m.messages[msg.Message.RoomID] = appendIfNotExists(m.messages[msg.Message.RoomID], msg.Message)
		m.renderMessages(msg.Message.RoomID)
		m.viewport.GotoBottom()
		m.msgInput.SetValue("")
		return m, nil

	case chatErrMsg:
		m.loading = false
		m.errMsg = msg.Err.Error()
		return m, nil

	case tea.KeyMsg:
		if m.subMode != chatModeNormal {
			return m.handleModalKeys(msg)
		}
		return m.handleNormalKeys(msg)
	}

	var cmd tea.Cmd
	if m.focus == focusInput {
		m.msgInput, cmd = m.msgInput.Update(msg)
	}
	return m, cmd
}

func (m chatViewModel) handleNormalKeys(msg tea.KeyMsg) (chatViewModel, tea.Cmd) {
	// 1. Global scrolling shortcuts (work from anywhere, even inside text input)
	switch msg.String() {
	case "pgup", "pageup":
		m.viewport.ViewUp()
		return m, m.checkLoadOlder()
	case "pgdown", "pagedown":
		m.viewport.ViewDown()
		return m, nil
	case "shift+up", "alt+up":
		m.viewport.LineUp(2)
		return m, m.checkLoadOlder()
	case "shift+down", "alt+down":
		m.viewport.LineDown(2)
		return m, nil
	}

	// 2. Room/DM Modals & Switching
	switch msg.String() {
	case "ctrl+k":
		if m.focus == focusInput {
			m.focus = focusRooms
			m.msgInput.Blur()
			return m, nil
		}
		if len(m.rooms) > 0 {
			m.focus = focusInput
			m.msgInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case "n":
		if m.focus == focusRooms {
			m.subMode = chatModeNewRoom
			m.modalInput.SetValue("")
			m.modalInput.Prompt = "Nama Channel: "
			m.modalInput.Focus()
			return m, textinput.Blink
		}

	case "u":
		if m.focus == focusRooms {
			m.subMode = chatModeNewDM
			m.modalInput.SetValue("")
			m.modalInput.Prompt = "Kirim DM ke @username: "
			m.modalInput.Focus()
			return m, textinput.Blink
		}
	}

	// 3. Focus Area Routing
	switch m.focus {
	case focusRooms:
		switch msg.String() {
		case "j", "down":
			if len(m.rooms) > 0 && m.roomCursor < len(m.rooms)-1 {
				m.roomCursor++
				m.activeRoomID = m.rooms[m.roomCursor].ID
				return m, m.loadMessagesCmd(m.activeRoomID)
			}
		case "k", "up":
			if m.roomCursor > 0 {
				m.roomCursor--
				m.activeRoomID = m.rooms[m.roomCursor].ID
				return m, m.loadMessagesCmd(m.activeRoomID)
			}
		case "l", "right":
			if len(m.rooms) > 0 {
				m.focus = focusMessages
				return m, nil
			}
		case "enter", "i":
			if len(m.rooms) > 0 {
				m.focus = focusInput
				m.msgInput.Focus()
				return m, textinput.Blink
			}
		case "r":
			m.loading = true
			return m, m.loadRoomsCmd()
		}

	case focusMessages:
		switch msg.String() {
		case "k", "up":
			m.viewport.LineUp(1)
			return m, m.checkLoadOlder()
		case "j", "down":
			m.viewport.LineDown(1)
			return m, nil
		case "u", "ctrl+u":
			m.viewport.HalfViewUp()
			return m, m.checkLoadOlder()
		case "d", "ctrl+d":
			m.viewport.HalfViewDown()
			return m, nil
		case "b":
			m.viewport.ViewUp()
			return m, m.checkLoadOlder()
		case "f", "space":
			m.viewport.ViewDown()
			return m, nil
		case "g", "home":
			m.viewport.GotoTop()
			return m, m.checkLoadOlder()
		case "G", "end":
			m.viewport.GotoBottom()
			return m, nil
		case "h", "left", "esc":
			m.focus = focusRooms
			return m, nil
		case "c", "y":
			if m.activeRoomID > 0 {
				msgs := m.messages[m.activeRoomID]
				if len(msgs) > 0 {
					last := msgs[len(msgs)-1]
					_ = CopyToClipboard(fmt.Sprintf("%s: %s", last.Username, last.Body))
				}
			}
			return m, nil
		case "i", "enter":
			m.focus = focusInput
			m.msgInput.Focus()
			return m, textinput.Blink
		}

	case focusInput:
		switch msg.String() {
		case "ctrl+v":
			_, _ = PasteIntoTextInput(&m.msgInput)
			return m, nil

		case "ctrl+c":
			if m.msgInput.Value() != "" {
				_ = CopyToClipboard(m.msgInput.Value())
			}
			return m, nil

		case "esc":
			m.focus = focusMessages
			m.msgInput.Blur()
			return m, nil

		case "ctrl+u":
			m.viewport.HalfViewUp()
			return m, m.checkLoadOlder()

		case "ctrl+d":
			m.viewport.HalfViewDown()
			return m, nil

		case "up":
			// If input is blank, pressing up enters scroll mode smoothly
			if m.msgInput.Value() == "" {
				m.focus = focusMessages
				m.msgInput.Blur()
				m.viewport.LineUp(2)
				return m, m.checkLoadOlder()
			}

		case "enter":
			body := strings.TrimSpace(m.msgInput.Value())
			if body == "" || m.activeRoomID == 0 {
				return m, nil
			}
			m.loading = true
			client := m.apiClient
			roomID := m.activeRoomID
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				msg, err := client.SendMessage(ctx, roomID, body)
				if err != nil {
					return chatErrMsg{Err: err}
				}
				return chatMessageSentMsg{Message: msg}
			}

		default:
			var cmd tea.Cmd
			m.msgInput, cmd = m.msgInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m chatViewModel) handleModalKeys(msg tea.KeyMsg) (chatViewModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.subMode = chatModeNormal
		m.errMsg = ""
		m.modalInput.Blur()
		return m, nil

	case "ctrl+v":
		_, _ = PasteIntoTextInput(&m.modalInput)
		return m, nil

	case "ctrl+c":
		if m.modalInput.Value() != "" {
			_ = CopyToClipboard(m.modalInput.Value())
		}
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.modalInput.Value())
		if val == "" {
			return m, nil
		}
		client := m.apiClient
		mode := m.subMode
		m.subMode = chatModeNormal
		m.errMsg = ""
		m.modalInput.Blur()
		m.loading = true

		if mode == chatModeNewRoom {
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				room, err := client.CreateRoom(ctx, val)
				if err != nil {
					return chatErrMsg{Err: err}
				}
				rooms, err := client.ListRooms(ctx)
				if err != nil {
					return chatErrMsg{Err: err}
				}
				return chatRoomsLoadedMsg{Rooms: rooms, SelectRoomID: room.ID}
			}
		} else if mode == chatModeNewDM {
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				room, err := client.CreateDM(ctx, val)
				if err != nil {
					return chatErrMsg{Err: err}
				}
				rooms, err := client.ListRooms(ctx)
				if err != nil {
					return chatErrMsg{Err: err}
				}
				return chatRoomsLoadedMsg{Rooms: rooms, SelectRoomID: room.ID}
			}
		}
	}

	var cmd tea.Cmd
	m.modalInput, cmd = m.modalInput.Update(msg)
	return m, cmd
}

func formatChatDateDivider(t time.Time) string {
	now := time.Now().Local()
	msgLocal := t.Local()

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	msgDay := time.Date(msgLocal.Year(), msgLocal.Month(), msgLocal.Day(), 0, 0, 0, 0, msgLocal.Location())

	daysDiff := int(today.Sub(msgDay).Hours() / 24)

	indonesianMonths := []string{
		"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	indonesianDays := []string{
		"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu",
	}

	dayName := indonesianDays[msgLocal.Weekday()]
	monthName := indonesianMonths[msgLocal.Month()]

	var label string
	switch daysDiff {
	case 0:
		label = fmt.Sprintf("Hari ini • %d %s %d", msgLocal.Day(), monthName, msgLocal.Year())
	case 1:
		label = fmt.Sprintf("Kemarin • %d %s %d", msgLocal.Day(), monthName, msgLocal.Year())
	default:
		if now.Year() == msgLocal.Year() {
			label = fmt.Sprintf("%s, %d %s", dayName, msgLocal.Day(), monthName)
		} else {
			label = fmt.Sprintf("%s, %d %s %d", dayName, msgLocal.Day(), monthName, msgLocal.Year())
		}
	}

	return fmt.Sprintf("─── %s ───", label)
}

func formatMsgTime(t time.Time) string {
	now := time.Now().Local()
	msgLocal := t.Local()

	isToday := now.Year() == msgLocal.Year() && now.YearDay() == msgLocal.YearDay()
	if isToday {
		return msgLocal.Format("15:04")
	}

	indonesianShortMonths := []string{
		"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
		"Jul", "Agu", "Sep", "Okt", "Nov", "Des",
	}
	return fmt.Sprintf("%d %s, %s", msgLocal.Day(), indonesianShortMonths[msgLocal.Month()], msgLocal.Format("15:04"))
}

func (m *chatViewModel) renderMessages(roomID int64) {
	msgs := m.messages[roomID]
	if len(msgs) == 0 {
		m.viewport.SetContent(lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).
			Render("\n  Belum ada pesan di percakapan ini.\n  Ketik pesan di bawah lalu tekan [Enter]!"))
		return
	}

	vpW := m.viewport.Width
	if vpW <= 0 {
		vpW = 60
	}
	// Usable width with safe margin to avoid any terminal edge wrapping
	availW := max(24, vpW-2)
	maxBubbleOuterW := max(20, min(56, int(float64(availW)*0.78)))
	maxInnerW := max(12, maxBubbleOuterW-4) // minus 2 border + 2 padding

	var b strings.Builder
	var lastDateStr string

	for idx, msg := range msgs {
		msgLocal := msg.CreatedAt.Local()
		dateKey := msgLocal.Format("2006-01-02")

		// 1. Render Date Divider if date changed
		if dateKey != lastDateStr {
			lastDateStr = dateKey
			dividerText := formatChatDateDivider(msgLocal)
			dividerStyle := lipgloss.NewStyle().
				Width(availW).
				Align(lipgloss.Center).
				Foreground(ColorSubtle).
				Bold(true)

			if idx > 0 {
				b.WriteString("\n")
			}
			b.WriteString(dividerStyle.Render(dividerText) + "\n\n")
		}

		timeStr := formatMsgTime(msg.CreatedAt)
		isMe := (m.currentUserID > 0 && msg.UserID == m.currentUserID) ||
			(m.currentUsername != "" && strings.EqualFold(msg.Username, m.currentUsername))

		// Calculate appropriate inner width based on raw content lines
		rawLines := strings.Split(msg.Body, "\n")
		longestLine := 0
		for _, l := range rawLines {
			visLen := lipgloss.Width(l)
			if visLen > longestLine {
				longestLine = visLen
			}
		}
		innerW := min(maxInnerW, max(14, longestLine))

		if isMe {
			bubbleStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSecondary).
				Background(ColorDarkBg).
				Foreground(ColorText).
				Padding(0, 1).
				Width(innerW)

			bubbleText := bubbleStyle.Render(msg.Body)
			bubbleW := lipgloss.Width(bubbleText)

			headerText := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Anda") +
				" " + lipgloss.NewStyle().Foreground(ColorSubtle).Render("• "+timeStr)
			headerW := lipgloss.Width(headerText)

			blockW := max(bubbleW, headerW)
			leftPad := availW - blockW
			if leftPad < 0 {
				leftPad = 0
			}
			padStr := strings.Repeat(" ", leftPad)

			// Header aligned with right edge of block
			headerPadStr := ""
			if blockW > headerW {
				headerPadStr = strings.Repeat(" ", blockW-headerW)
			}
			b.WriteString(padStr + headerPadStr + headerText + "\n")

			// Bubble lines with padStr
			bubbleLines := strings.Split(bubbleText, "\n")
			bubblePadStr := ""
			if blockW > bubbleW {
				bubblePadStr = strings.Repeat(" ", blockW-bubbleW)
			}
			for _, bl := range bubbleLines {
				b.WriteString(padStr + bubblePadStr + bl + "\n")
			}
		} else {
			bubbleStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorAccent).
				Background(ColorDarkBg).
				Foreground(ColorText).
				Padding(0, 1).
				Width(innerW)

			bubbleText := bubbleStyle.Render(msg.Body)
			bubbleLines := strings.Split(bubbleText, "\n")

			headerText := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(msg.Username) +
				" " + lipgloss.NewStyle().Foreground(ColorSubtle).Render("• "+timeStr)

			indentStr := " "
			b.WriteString(indentStr + headerText + "\n")
			for _, bl := range bubbleLines {
				b.WriteString(indentStr + bl + "\n")
			}
		}

		if idx < len(msgs)-1 {
			b.WriteString("\n")
		}
	}
	m.viewport.SetContent(b.String())
}

// OnWSEvent processes incoming WebSocket events from background client
func (m *chatViewModel) OnWSEvent(evt models.WSEvent) {
	switch evt.Type {
	case "message":
		// Payload is Message
		msgBytes, _ := json.Marshal(evt.Payload)
		var msg models.Message
		if err := json.Unmarshal(msgBytes, &msg); err == nil {
			m.messages[msg.RoomID] = appendIfNotExists(m.messages[msg.RoomID], msg)
			if msg.RoomID == m.activeRoomID {
				atBottom := m.viewport.AtBottom()
				m.renderMessages(msg.RoomID)
				if atBottom || m.focus == focusInput {
					m.viewport.GotoBottom()
				}
				// Mark as read
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = m.apiClient.MarkRead(ctx, msg.RoomID, msg.ID)
				}()
			} else {
				// Increment unread count for that room
				for i := range m.rooms {
					if m.rooms[i].ID == msg.RoomID {
						m.rooms[i].UnreadCount++
					}
				}
			}
		}
	case "presence":
		var p models.WSPresence
		msgBytes, _ := json.Marshal(evt.Payload)
		if err := json.Unmarshal(msgBytes, &p); err == nil {
			m.onlineUsers[p.UserID] = p.Online
		}
	}
}

func (m chatViewModel) View() string {
	if m.subMode != chatModeNormal {
		title := "BUAT CHANNEL BARU"
		if m.subMode == chatModeNewDM {
			title = "MULAI PERCAKAPAN PRIVAT (DM)"
		}
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render(title) + "\n\n")
		modal.WriteString(m.modalInput.View() + "\n\n")
		modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Enter] Simpan  •  [Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	// 1. Sidebar (Rooms & DMs)
	var sb strings.Builder
	sidebarTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("CHANNELS & DMS")
	sb.WriteString(sidebarTitle + "\n\n")

	if len(m.rooms) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).Render("Belum ada room.\n[n] Channel baru\n[u] Mulai DM") + "\n")
	} else {
		for i, r := range m.rooms {
			isCursor := (i == m.roomCursor)
			style := NormalItemStyle
			cursorPrefix := "  "
			if isCursor {
				cursorPrefix = "▶ "
				style = SelectedItemStyle
			}

			roomLabel := r.Name
			if r.UnreadCount > 0 {
				roomLabel += fmt.Sprintf(" (%d)", r.UnreadCount)
				style = style.Foreground(ColorHighlight).Bold(true)
			}

			sb.WriteString(style.Render(cursorPrefix+roomLabel) + "\n")
		}
	}

	sidebarBox := lipgloss.NewStyle().
		Width(24).
		Height(max(8, m.height-14)).
		Border(lipgloss.RoundedBorder(), false, true, false, false).
		BorderForeground(ColorBorder).
		PaddingRight(1).
		Render(sb.String())

	// 2. Chat Viewport & Message Input
	var mainPanel strings.Builder

	// Header for active room
	activeRoomName := "Pilih Room"
	for _, r := range m.rooms {
		if r.ID == m.activeRoomID {
			activeRoomName = r.Name
			break
		}
	}

	wsStatus := lipgloss.NewStyle().Foreground(ColorSuccess).Render("● Online")
	if !m.wsOnline {
		wsStatus = lipgloss.NewStyle().Foreground(ColorSubtle).Render("○ Offline")
	}

	// Scroll position badge
	scrollBadge := ""
	if len(m.messages[m.activeRoomID]) > 0 {
		if m.viewport.AtBottom() {
			scrollBadge = lipgloss.NewStyle().Foreground(ColorSubtle).Render("▼ Terbaru")
		} else if m.viewport.AtTop() {
			scrollBadge = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("▲ Paling Atas")
		} else {
			scrollBadge = lipgloss.NewStyle().Foreground(ColorHighlight).Render(fmt.Sprintf("↕ %.0f%%", m.viewport.ScrollPercent()*100))
		}
	}

	focusHint := ""
	if m.focus == focusMessages {
		focusHint = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSelectFg).
			Background(ColorPrimary).
			Padding(0, 1).
			Render("MODE BACA [↑/k: Atas  ↓/j: Bawah  PgUp/PgDn  i: Balas  h/Esc: Room]")
	} else if m.focus == focusRooms {
		focusHint = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Render("[l/→: Baca Chat  •  i: Ketik]")
	} else {
		focusHint = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Render("[Esc: Mode Baca  •  PgUp/PgDn: Scroll]")
	}

	header := fmt.Sprintf("%s   %s   %s   %s",
		lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(activeRoomName),
		wsStatus,
		scrollBadge,
		focusHint,
	)
	mainPanel.WriteString(header + "\n\n")

	// Viewport
	mainPanel.WriteString(m.viewport.View() + "\n\n")

	// Message Input
	inputBorderColor := ColorBorder
	if m.focus == focusInput {
		inputBorderColor = ColorPrimary
	}
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorderColor).
		Padding(0, 1).
		Render(m.msgInput.View())

	mainPanel.WriteString(inputBox)

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, "  ", mainPanel.String())
	return content
}
