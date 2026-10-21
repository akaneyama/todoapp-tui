package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/api"
	"todoapp/internal/models"
)

type todoSubMode int

const (
	subModeNormal todoSubMode = iota
	subModeAdd
	subModeEdit
	subModeBatch
	subModeDetail
	subModeConfirmDelete
	subModeConfirmRollover
)

// Messages
type todosLoadedMsg struct {
	Date  string
	Todos []models.Todo
}

type todoOpSuccessMsg struct {
	Message string
}

type todoOpErrMsg struct {
	Err error
}

type todoViewModel struct {
	apiClient *api.Client
	date      time.Time // current selected date
	todos     []models.Todo
	cursor    int
	subMode   todoSubMode
	loading   bool
	spinner   spinner.Model
	statusMsg string
	errMsg    string
	width     int
	height    int

	// Modal inputs for Add / Edit
	titleInput    textinput.Model
	descInput     textinput.Model
	priorityInput textinput.Model
	repeatInput   textinput.Model
	inputFocus    int

	// Modal inputs for Batch Creation
	batchTitleInput      textinput.Model
	batchDescInput       textinput.Model
	batchStartInput      textinput.Model
	batchEndInput        textinput.Model
	batchDaysInput       textinput.Model
	batchPriorityInput   textinput.Model
	batchRemindTimeInput textinput.Model
	batchFocus           int
}

func newTodoViewModel(apiClient *api.Client) todoViewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	tInput := textinput.New()
	tInput.Placeholder = "Judul task to-do"
	tInput.Prompt = "  Judul: "
	tInput.CharLimit = 500

	dInput := textinput.New()
	dInput.Placeholder = "Catatan / deskripsi tambahan (opsional)"
	dInput.Prompt = "  Catatan: "
	dInput.CharLimit = 1000

	pInput := textinput.New()
	pInput.Placeholder = "1 (Tinggi), 2 (Sedang), 3 (Rendah)"
	pInput.Prompt = "  Prioritas [1-3]: "
	pInput.CharLimit = 1

	rInput := textinput.New()
	rInput.Placeholder = "none, daily, weekly"
	rInput.Prompt = "  Pengulangan: "
	rInput.CharLimit = 10

	bTitle := textinput.New()
	bTitle.Placeholder = "Contoh: Posting Konten Medsos"
	bTitle.Prompt = "  Judul: "
	bTitle.CharLimit = 500

	bDesc := textinput.New()
	bDesc.Placeholder = "Catatan / deskripsi (opsional)"
	bDesc.Prompt = "  Catatan: "
	bDesc.CharLimit = 1000

	bStart := textinput.New()
	bStart.Placeholder = "YYYY-MM-DD"
	bStart.Prompt = "  Dari Tanggal: "
	bStart.CharLimit = 10

	bEnd := textinput.New()
	bEnd.Placeholder = "YYYY-MM-DD"
	bEnd.Prompt = "  Sampai Tanggal: "
	bEnd.CharLimit = 10

	bDays := textinput.New()
	bDays.Placeholder = "y / n (y = Senin-Jumat saja)"
	bDays.Prompt = "  Senin-Jumat Saja [y/n]: "
	bDays.CharLimit = 5

	bPrio := textinput.New()
	bPrio.Placeholder = "1 (Tinggi), 2 (Sedang), 3 (Rendah)"
	bPrio.Prompt = "  Prioritas [1-3]: "
	bPrio.CharLimit = 1

	bRemind := textinput.New()
	bRemind.Placeholder = "HH:MM (mis. 09:00, opsional)"
	bRemind.Prompt = "  Jam Pengingat: "
	bRemind.CharLimit = 5

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	return todoViewModel{
		apiClient:            apiClient,
		date:                 today,
		todos:                []models.Todo{},
		cursor:               0,
		subMode:              subModeNormal,
		loading:              false,
		spinner:              s,
		titleInput:           tInput,
		descInput:            dInput,
		priorityInput:        pInput,
		repeatInput:          rInput,
		inputFocus:           0,
		batchTitleInput:      bTitle,
		batchDescInput:       bDesc,
		batchStartInput:      bStart,
		batchEndInput:        bEnd,
		batchDaysInput:       bDays,
		batchPriorityInput:   bPrio,
		batchRemindTimeInput: bRemind,
		batchFocus:           0,
	}
}

func (m todoViewModel) Init() tea.Cmd {
	return tea.Batch(m.loadTodosCmd(m.date), m.spinner.Tick)
}

func (m todoViewModel) loadTodosCmd(date time.Time) tea.Cmd {
	client := m.apiClient
	dateStr := date.Format(time.DateOnly)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.ListTodos(ctx, dateStr, nil)
		if err != nil {
			return todoOpErrMsg{Err: err}
		}
		return todosLoadedMsg{Date: dateStr, Todos: resp.Todos}
	}
}

func (m todoViewModel) Update(msg tea.Msg) (todoViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case todosLoadedMsg:
		m.loading = false
		m.todos = msg.Todos
		if m.cursor >= len(m.todos) {
			m.cursor = max(0, len(m.todos)-1)
		}
		return m, nil

	case todoOpSuccessMsg:
		m.loading = false
		m.statusMsg = msg.Message
		m.errMsg = ""
		if m.subMode != subModeDetail {
			m.subMode = subModeNormal
		}
		return m, m.loadTodosCmd(m.date)

	case todoOpErrMsg:
		m.loading = false
		m.errMsg = msg.Err.Error()
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		switch m.subMode {
		case subModeNormal:
			return m.handleNormalKeys(msg)
		case subModeDetail:
			return m.handleDetailKeys(msg)
		case subModeAdd, subModeEdit:
			return m.handleInputKeys(msg)
		case subModeBatch:
			return m.handleBatchKeys(msg)
		case subModeConfirmDelete:
			return m.handleDeleteConfirmKeys(msg)
		case subModeConfirmRollover:
			return m.handleRolloverConfirmKeys(msg)
		}
	}

	return m, nil
}

func (m todoViewModel) handleNormalKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	key := strings.ToLower(msg.String())
	switch key {
	case "j", "down":
		if len(m.todos) > 0 && m.cursor < len(m.todos)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "g", "home":
		m.cursor = 0
		return m, nil

	case "end":
		if len(m.todos) > 0 {
			m.cursor = len(m.todos) - 1
		}
		return m, nil

	case "h", "[", "left":
		m.date = m.date.AddDate(0, 0, -1)
		m.loading = true
		m.cursor = 0
		return m, m.loadTodosCmd(m.date)

	case "l", "]", "right":
		m.date = m.date.AddDate(0, 0, 1)
		m.loading = true
		m.cursor = 0
		return m, m.loadTodosCmd(m.date)

	case "t":
		now := time.Now()
		m.date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		m.loading = true
		m.cursor = 0
		return m, m.loadTodosCmd(m.date)

	case "r":
		m.loading = true
		return m, m.loadTodosCmd(m.date)

	case " ":
		// Toggle Done
		if len(m.todos) == 0 {
			return m, nil
		}
		item := m.todos[m.cursor]
		newDone := !item.Done
		m.loading = true
		client := m.apiClient
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := client.UpdateTodo(ctx, item.ID, models.UpdateTodoRequest{Done: &newDone})
			if err != nil {
				return todoOpErrMsg{Err: err}
			}
			msg := "Task selesai!"
			if !newDone {
				msg = "Task dibuka kembali."
			}
			return todoOpSuccessMsg{Message: msg}
		}

	case "a":
		// Open Add Modal
		m.subMode = subModeAdd
		m.titleInput.SetValue("")
		m.descInput.SetValue("")
		m.priorityInput.SetValue("2")
		m.repeatInput.SetValue("none")
		m.inputFocus = 0
		m.titleInput.Focus()
		m.descInput.Blur()
		m.priorityInput.Blur()
		m.repeatInput.Blur()
		return m, textinput.Blink

	case "b":
		// Open Batch Modal
		return m.openBatchModal()

	case "e":
		// Open Edit Modal
		if len(m.todos) == 0 {
			return m, nil
		}
		item := m.todos[m.cursor]
		m.subMode = subModeEdit
		m.titleInput.SetValue(item.Title)
		if item.Description != nil {
			m.descInput.SetValue(*item.Description)
		} else {
			m.descInput.SetValue("")
		}
		m.priorityInput.SetValue(strconv.Itoa(int(item.Priority)))
		m.repeatInput.SetValue(item.RepeatRule)
		m.inputFocus = 0
		m.titleInput.Focus()
		m.descInput.Blur()
		m.priorityInput.Blur()
		m.repeatInput.Blur()
		return m, textinput.Blink

	case "d":
		// Open Delete Confirmation
		if len(m.todos) == 0 {
			return m, nil
		}
		m.subMode = subModeConfirmDelete
		return m, nil

	case "m":
		// Open Rollover Confirmation
		m.subMode = subModeConfirmRollover
		return m, nil

	case "c", "y":
		if len(m.todos) > 0 && m.cursor >= 0 && m.cursor < len(m.todos) {
			item := m.todos[m.cursor]
			status := "[ ]"
			if item.Done {
				status = "[x]"
			}
			text := fmt.Sprintf("%s %s (Prioritas: %d)", status, item.Title, item.Priority)
			if item.Description != nil && *item.Description != "" {
				text += "\nCatatan: " + *item.Description
			}
			if item.Date != "" {
				text += "\nTanggal: " + item.Date
			}
			if err := CopyToClipboard(text); err != nil {
				m.errMsg = "Gagal menyalin: " + err.Error()
			} else {
				m.statusMsg = "📋 Task disalin ke clipboard!"
				m.errMsg = ""
			}
		} else {
			m.statusMsg = "Belum ada task untuk disalin."
		}
		return m, nil

	case "enter", "v", "o":
		// Open Detail View modal
		if len(m.todos) > 0 {
			m.subMode = subModeDetail
			return m, nil
		}
	}

	return m, nil
}

func (m todoViewModel) handleDetailKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	key := strings.ToLower(msg.String())
	switch key {
	case "esc", "q", "enter", "v", "o":
		m.subMode = subModeNormal
		return m, nil

	case "c", "y":
		if len(m.todos) > 0 && m.cursor >= 0 && m.cursor < len(m.todos) {
			item := m.todos[m.cursor]
			status := "[ ]"
			if item.Done {
				status = "[x]"
			}
			text := fmt.Sprintf("%s %s (Prioritas: %d)", status, item.Title, item.Priority)
			if item.Description != nil && *item.Description != "" {
				text += "\nCatatan: " + *item.Description
			}
			if item.Date != "" {
				text += "\nTanggal: " + item.Date
			}
			if err := CopyToClipboard(text); err != nil {
				m.errMsg = "Gagal menyalin: " + err.Error()
			} else {
				m.statusMsg = "📋 Task disalin ke clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "h", "[", "left":
		// Previous day
		m.date = m.date.AddDate(0, 0, -1)
		m.cursor = 0
		m.loading = true
		return m, m.loadTodosCmd(m.date)

	case "l", "]", "right":
		// Next day
		m.date = m.date.AddDate(0, 0, 1)
		m.cursor = 0
		m.loading = true
		return m, m.loadTodosCmd(m.date)

	case "t":
		// Today
		now := time.Now()
		m.date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		m.cursor = 0
		m.loading = true
		return m, m.loadTodosCmd(m.date)

	case "j", "down":
		// Inspect next todo
		if len(m.todos) > 0 && m.cursor < len(m.todos)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		// Inspect previous todo
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case " ":
		// Toggle Done directly from Detail View
		if len(m.todos) == 0 {
			m.subMode = subModeNormal
			return m, nil
		}
		item := m.todos[m.cursor]
		newDone := !item.Done
		m.loading = true
		client := m.apiClient
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := client.UpdateTodo(ctx, item.ID, models.UpdateTodoRequest{Done: &newDone})
			if err != nil {
				return todoOpErrMsg{Err: err}
			}
			msg := "Task selesai!"
			if !newDone {
				msg = "Task dibuka kembali."
			}
			return todoOpSuccessMsg{Message: msg}
		}

	case "e":
		// Switch to Edit modal
		if len(m.todos) == 0 {
			m.subMode = subModeNormal
			return m, nil
		}
		item := m.todos[m.cursor]
		m.subMode = subModeEdit
		m.titleInput.SetValue(item.Title)
		if item.Description != nil {
			m.descInput.SetValue(*item.Description)
		} else {
			m.descInput.SetValue("")
		}
		m.priorityInput.SetValue(strconv.Itoa(int(item.Priority)))
		m.repeatInput.SetValue(item.RepeatRule)
		m.inputFocus = 0
		m.titleInput.Focus()
		m.descInput.Blur()
		m.priorityInput.Blur()
		m.repeatInput.Blur()
		return m, textinput.Blink

	case "d":
		// Switch to Delete confirmation
		if len(m.todos) == 0 {
			m.subMode = subModeNormal
			return m, nil
		}
		m.subMode = subModeConfirmDelete
		return m, nil
	}
	return m, nil
}

func (m todoViewModel) handleInputKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.subMode = subModeNormal
		m.errMsg = ""
		return m, nil

	case "ctrl+v":
		inputs := []*textinput.Model{&m.titleInput, &m.descInput, &m.priorityInput, &m.repeatInput}
		if m.inputFocus >= 0 && m.inputFocus < len(inputs) {
			pasted, err := PasteIntoTextInput(inputs[m.inputFocus])
			if err != nil {
				m.errMsg = "Gagal menempel: " + err.Error()
			} else if pasted != "" {
				m.statusMsg = "📋 Berhasil menempel dari clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "ctrl+c":
		inputs := []*textinput.Model{&m.titleInput, &m.descInput, &m.priorityInput, &m.repeatInput}
		if m.inputFocus >= 0 && m.inputFocus < len(inputs) {
			val := inputs[m.inputFocus].Value()
			if val != "" {
				_ = CopyToClipboard(val)
				m.statusMsg = "📋 Teks field disalin ke clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "tab", "shift+tab", "up", "down":
		if msg.String() == "up" || msg.String() == "shift+tab" {
			m.inputFocus--
			if m.inputFocus < 0 {
				m.inputFocus = 3
			}
		} else {
			m.inputFocus++
			if m.inputFocus > 3 {
				m.inputFocus = 0
			}
		}
		var cmds []tea.Cmd
		inputs := []*textinput.Model{&m.titleInput, &m.descInput, &m.priorityInput, &m.repeatInput}
		for i, in := range inputs {
			if i == m.inputFocus {
				cmds = append(cmds, in.Focus())
			} else {
				in.Blur()
			}
		}
		return m, tea.Batch(cmds...)

	case "enter":
		title := strings.TrimSpace(m.titleInput.Value())
		if title == "" {
			m.errMsg = "Judul task tidak boleh kosong"
			return m, nil
		}
		desc := strings.TrimSpace(m.descInput.Value())
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}
		prioVal, _ := strconv.Atoi(strings.TrimSpace(m.priorityInput.Value()))
		if prioVal < 1 || prioVal > 3 {
			prioVal = 2
		}
		priority := uint8(prioVal)
		repeat := strings.ToLower(strings.TrimSpace(m.repeatInput.Value()))
		if repeat != "daily" && repeat != "weekly" {
			repeat = "none"
		}

		client := m.apiClient
		dateStr := m.date.Format(time.DateOnly)

		if m.subMode == subModeAdd {
			m.loading = true
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := client.CreateTodo(ctx, models.CreateTodoRequest{
					Date:        dateStr,
					Title:       title,
					Description: descPtr,
					Priority:    &priority,
					RepeatRule:  repeat,
				})
				if err != nil {
					return todoOpErrMsg{Err: err}
				}
				return todoOpSuccessMsg{Message: "Task berhasil ditambahkan!"}
			}
		} else if m.subMode == subModeEdit {
			if len(m.todos) == 0 {
				m.subMode = subModeNormal
				return m, nil
			}
			todoID := m.todos[m.cursor].ID
			m.loading = true
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := client.UpdateTodo(ctx, todoID, models.UpdateTodoRequest{
					Title:       &title,
					Description: descPtr,
					Priority:    &priority,
					RepeatRule:  &repeat,
				})
				if err != nil {
					return todoOpErrMsg{Err: err}
				}
				return todoOpSuccessMsg{Message: "Task berhasil diperbarui!"}
			}
		}
	}

	var cmd tea.Cmd
	switch m.inputFocus {
	case 0:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case 1:
		m.descInput, cmd = m.descInput.Update(msg)
	case 2:
		m.priorityInput, cmd = m.priorityInput.Update(msg)
	default:
		m.repeatInput, cmd = m.repeatInput.Update(msg)
	}
	return m, cmd
}

func (m todoViewModel) openBatchModal() (todoViewModel, tea.Cmd) {
	m.subMode = subModeBatch
	m.batchFocus = 0
	m.batchTitleInput.SetValue("")
	m.batchDescInput.SetValue("")
	m.batchStartInput.SetValue(m.date.Format(time.DateOnly))
	m.batchEndInput.SetValue(m.date.AddDate(0, 0, 7).Format(time.DateOnly))
	m.batchDaysInput.SetValue("y")
	m.batchPriorityInput.SetValue("2")
	m.batchRemindTimeInput.SetValue("")

	m.batchTitleInput.Focus()
	m.batchDescInput.Blur()
	m.batchStartInput.Blur()
	m.batchEndInput.Blur()
	m.batchDaysInput.Blur()
	m.batchPriorityInput.Blur()
	m.batchRemindTimeInput.Blur()

	return m, textinput.Blink
}

func (m todoViewModel) handleBatchKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.subMode = subModeNormal
		m.errMsg = ""
		return m, nil

	case "ctrl+v":
		inputs := []*textinput.Model{
			&m.batchTitleInput, &m.batchDescInput, &m.batchStartInput,
			&m.batchEndInput, &m.batchDaysInput, &m.batchPriorityInput,
			&m.batchRemindTimeInput,
		}
		if m.batchFocus >= 0 && m.batchFocus < len(inputs) {
			pasted, err := PasteIntoTextInput(inputs[m.batchFocus])
			if err != nil {
				m.errMsg = "Gagal menempel: " + err.Error()
			} else if pasted != "" {
				m.statusMsg = "📋 Berhasil menempel dari clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "ctrl+c":
		inputs := []*textinput.Model{
			&m.batchTitleInput, &m.batchDescInput, &m.batchStartInput,
			&m.batchEndInput, &m.batchDaysInput, &m.batchPriorityInput,
			&m.batchRemindTimeInput,
		}
		if m.batchFocus >= 0 && m.batchFocus < len(inputs) {
			val := inputs[m.batchFocus].Value()
			if val != "" {
				_ = CopyToClipboard(val)
				m.statusMsg = "📋 Teks disalin ke clipboard!"
				m.errMsg = ""
			}
		}
		return m, nil

	case "tab", "shift+tab", "up", "down":
		if msg.String() == "up" || msg.String() == "shift+tab" {
			m.batchFocus--
			if m.batchFocus < 0 {
				m.batchFocus = 6
			}
		} else {
			m.batchFocus++
			if m.batchFocus > 6 {
				m.batchFocus = 0
			}
		}
		return m.updateBatchFocus()

	case "enter":
		title := strings.TrimSpace(m.batchTitleInput.Value())
		if title == "" {
			m.errMsg = "Judul task batch tidak boleh kosong"
			return m, nil
		}

		desc := strings.TrimSpace(m.batchDescInput.Value())
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}

		startStr := strings.TrimSpace(m.batchStartInput.Value())
		endStr := strings.TrimSpace(m.batchEndInput.Value())
		if startStr == "" {
			startStr = m.date.Format(time.DateOnly)
		}
		if endStr == "" {
			endStr = startStr
		}

		daysVal := strings.ToLower(strings.TrimSpace(m.batchDaysInput.Value()))
		weekdaysOnly := daysVal != "n" && daysVal != "tidak" && daysVal != "no"

		prioVal, _ := strconv.Atoi(strings.TrimSpace(m.batchPriorityInput.Value()))
		if prioVal < 1 || prioVal > 3 {
			prioVal = 2
		}
		priority := uint8(prioVal)

		remindTime := strings.TrimSpace(m.batchRemindTimeInput.Value())
		var remindTimePtr *string
		if remindTime != "" {
			remindTimePtr = &remindTime
		}

		client := m.apiClient
		m.loading = true
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			resp, err := client.CreateBatchTodos(ctx, models.BatchCreateTodoRequest{
				Title:        title,
				Description:  descPtr,
				StartDate:    startStr,
				EndDate:      endStr,
				WeekdaysOnly: weekdaysOnly,
				Priority:     &priority,
				RemindTime:   remindTimePtr,
			})
			if err != nil {
				return todoOpErrMsg{Err: err}
			}
			return todoOpSuccessMsg{Message: fmt.Sprintf("%d task batch berhasil dibuat!", resp.CreatedCount)}
		}
	}

	var cmd tea.Cmd
	switch m.batchFocus {
	case 0:
		m.batchTitleInput, cmd = m.batchTitleInput.Update(msg)
	case 1:
		m.batchDescInput, cmd = m.batchDescInput.Update(msg)
	case 2:
		m.batchStartInput, cmd = m.batchStartInput.Update(msg)
	case 3:
		m.batchEndInput, cmd = m.batchEndInput.Update(msg)
	case 4:
		m.batchDaysInput, cmd = m.batchDaysInput.Update(msg)
	case 5:
		m.batchPriorityInput, cmd = m.batchPriorityInput.Update(msg)
	case 6:
		m.batchRemindTimeInput, cmd = m.batchRemindTimeInput.Update(msg)
	}
	return m, cmd
}

func (m todoViewModel) updateBatchFocus() (todoViewModel, tea.Cmd) {
	inputs := []*textinput.Model{
		&m.batchTitleInput,
		&m.batchDescInput,
		&m.batchStartInput,
		&m.batchEndInput,
		&m.batchDaysInput,
		&m.batchPriorityInput,
		&m.batchRemindTimeInput,
	}
	var cmds []tea.Cmd
	for i, in := range inputs {
		if i == m.batchFocus {
			cmds = append(cmds, in.Focus())
		} else {
			in.Blur()
		}
	}
	return m, tea.Batch(cmds...)
}

func (m todoViewModel) handleDeleteConfirmKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if len(m.todos) == 0 {
			m.subMode = subModeNormal
			return m, nil
		}
		todoID := m.todos[m.cursor].ID
		m.loading = true
		client := m.apiClient
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := client.DeleteTodo(ctx, todoID)
			if err != nil {
				return todoOpErrMsg{Err: err}
			}
			return todoOpSuccessMsg{Message: "Task berhasil dihapus."}
		}
	case "n", "N", "esc":
		m.subMode = subModeNormal
		return m, nil
	}
	return m, nil
}

func (m todoViewModel) handleRolloverConfirmKeys(msg tea.KeyMsg) (todoViewModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		client := m.apiClient
		fromDate := m.date.Format(time.DateOnly)
		toDate := m.date.AddDate(0, 0, 1).Format(time.DateOnly)
		m.loading = true
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			resp, err := client.RolloverTodos(ctx, fromDate, toDate)
			if err != nil {
				return todoOpErrMsg{Err: err}
			}
			return todoOpSuccessMsg{Message: fmt.Sprintf("%d task belum selesai berhasil dipindahkan ke besok (%s)!", resp.MovedCount, toDate)}
		}
	case "n", "N", "esc":
		m.subMode = subModeNormal
		return m, nil
	}
	return m, nil
}

func (m todoViewModel) View() string {
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

	// 2. Modals (if active)
	if m.subMode == subModeDetail {
		if len(m.todos) == 0 {
			m.subMode = subModeNormal
			return m.View()
		}
		modalWidth := max(58, min(84, m.width-6))
		todo := m.todos[m.cursor]
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render("📋 DETAIL TO-DO") + "\n\n")

		// Status Badge & Priority Badge row
		var statusBadge string
		if todo.Done {
			statusBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorDarkBg).
				Background(ColorSuccess).
				Padding(0, 1).
				Render("✓ SELESAI")
		} else {
			statusBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorDarkBg).
				Background(ColorOrange).
				Padding(0, 1).
				Render("⏳ BELUM SELESAI")
		}

		var prioBadge string
		switch todo.Priority {
		case 1:
			prioBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorSelectFg).
				Background(ColorRed).
				Padding(0, 1).
				Render("P1 • TINGGI")
		case 3:
			prioBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorSelectFg).
				Background(ColorBlue).
				Padding(0, 1).
				Render("P3 • RENDAH")
		default:
			prioBadge = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorDarkBg).
				Background(ColorAccent).
				Padding(0, 1).
				Render("P2 • SEDANG")
		}

		modal.WriteString(fmt.Sprintf("%s   %s\n\n", statusBadge, prioBadge))

		// Title Section
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("📌 Judul Task:") + "\n")
		titleStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Width(modalWidth - 6).Render(todo.Title)
		modal.WriteString("  " + titleStyled + "\n\n")

		// Schedule Card
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("📅 Jadwal & Pengingat:") + "\n")
		modal.WriteString(fmt.Sprintf("  • Tanggal: %s\n", todo.Date))
		if todo.RepeatRule != "" && todo.RepeatRule != "none" {
			repeatLabel := todo.RepeatRule
			if repeatLabel == "daily" {
				repeatLabel = "Setiap Hari (Daily)"
			} else if repeatLabel == "weekly" {
				repeatLabel = "Setiap Minggu (Weekly)"
			}
			modal.WriteString(fmt.Sprintf("  • Pengulangan: ↻ %s\n", repeatLabel))
		}
		if todo.RemindAt != nil {
			modal.WriteString(fmt.Sprintf("  • Jam Pengingat: ⏰ %s\n", todo.RemindAt.Local().Format("15:04 (02 Jan 2006)")))
		}
		modal.WriteString("\n")

		// Notes / Description Box
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("📝 Catatan / Deskripsi:") + "\n")
		if todo.Description != nil && strings.TrimSpace(*todo.Description) != "" {
			descBox := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Background(ColorDarkBg).
				Padding(0, 1).
				Width(modalWidth - 6).
				Render(*todo.Description)
			modal.WriteString(descBox + "\n\n")
		} else {
			modal.WriteString("  " + lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).Render("(Tidak ada catatan tambahan)") + "\n\n")
		}

		// Metadata Timestamps
		createdStr := todo.CreatedAt.Local().Format("02 Jan 2006 15:04")
		updatedStr := todo.UpdatedAt.Local().Format("02 Jan 2006 15:04")
		modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render(fmt.Sprintf("🕒 Dibuat: %s   •   Diperbarui: %s", createdStr, updatedStr)) + "\n\n")

		// Action Shortcuts Footer
		shortcuts := lipgloss.NewStyle().Foreground(ColorSubtle).Render(
			"[Space] Ubah Status  •  [e] Edit  •  [d] Hapus  •  [j/k] Task Lain  •  [h/l] Ganti Hari  •  [Esc] Tutup",
		)
		modal.WriteString(shortcuts)

		return ModalBoxStyle.Width(modalWidth).Render(modal.String())
	}
	if m.subMode == subModeAdd || m.subMode == subModeEdit {
		title := "TAMBAH TO-DO BARU"
		if m.subMode == subModeEdit {
			title = "EDIT TO-DO"
		}
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render(title) + "\n\n")
		modal.WriteString(m.titleInput.View() + "\n\n")
		modal.WriteString(m.descInput.View() + "\n\n")
		modal.WriteString(m.priorityInput.View() + "\n\n")
		modal.WriteString(m.repeatInput.View() + "\n\n")
		modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Tab] Pindah field  •  [Ctrl+V] Tempel  •  [Enter] Simpan  •  [Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	if m.subMode == subModeBatch {
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render("BATCH CREATION TO-DO (SENIN - JUMAT / RANGE)") + "\n\n")
		modal.WriteString(m.batchTitleInput.View() + "\n\n")
		modal.WriteString(m.batchDescInput.View() + "\n\n")
		modal.WriteString(m.batchStartInput.View() + "\n\n")
		modal.WriteString(m.batchEndInput.View() + "\n\n")
		modal.WriteString(m.batchDaysInput.View() + "\n\n")
		modal.WriteString(m.batchPriorityInput.View() + "\n\n")
		modal.WriteString(m.batchRemindTimeInput.View() + "\n\n")
		modal.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Tab] Pindah field  •  [Ctrl+V] Tempel  •  [Enter] Generate Batch  •  [Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	if m.subMode == subModeConfirmDelete {
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render("HAPUS TO-DO?") + "\n\n")
		if len(m.todos) > 0 {
			modal.WriteString(fmt.Sprintf("Yakin ingin menghapus task: %q?\n\n", m.todos[m.cursor].Title))
		}
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorRed).Render("[y] Ya, Hapus  •  [n/Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	if m.subMode == subModeConfirmRollover {
		var modal strings.Builder
		modal.WriteString(ModalTitleStyle.Render("PINDAHKAN KE BESOK (ROLLOVER)") + "\n\n")
		modal.WriteString("Pindahkan semua task yang belum selesai dari tanggal ini ke besok?\n\n")
		modal.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("[y] Ya, Pindahkan  •  [n/Esc] Batal"))
		return ModalBoxStyle.Render(modal.String())
	}

	// 3. Todo List Items
	if m.loading && len(m.todos) == 0 {
		b.WriteString(m.spinner.View() + " Memuat daftar to-do...\n")
	} else if len(m.todos) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true).
			Render("Belum ada to-do untuk tanggal ini.\nTekan [a] untuk menambah task baru, atau [b] untuk batch creation (Senin-Jumat).")
		b.WriteString(emptyMsg + "\n")
	} else {
		for i, item := range m.todos {
			isCursor := (i == m.cursor)
			cursorPrefix := "  "
			if isCursor {
				cursorPrefix = lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render("▶ ")
			}

			// Checkbox
			check := "[ ]"
			if item.Done {
				check = lipgloss.NewStyle().Foreground(ColorSuccess).Render("[✓]")
			}

			// Priority Badge
			pBadge := P2BadgeStyle.String()
			switch item.Priority {
			case 1:
				pBadge = P1BadgeStyle.String()
			case 3:
				pBadge = P3BadgeStyle.String()
			}

			// Repeat Badge
			repeatBadge := ""
			if item.RepeatRule != "" && item.RepeatRule != "none" {
				repeatBadge = lipgloss.NewStyle().Foreground(ColorAccent).Render(" ↻" + item.RepeatRule)
			}

			// Title
			titleStyled := item.Title
			if item.Done {
				titleStyled = DoneItemStyle.Render(titleStyled)
			} else if isCursor {
				titleStyled = SelectedItemStyle.Render(titleStyled)
			} else {
				titleStyled = NormalItemStyle.Render(titleStyled)
			}

			line := fmt.Sprintf("%s%s %s %s%s", cursorPrefix, check, pBadge, titleStyled, repeatBadge)
			b.WriteString(line + "\n")
			if item.Description != nil && *item.Description != "" {
				descStyle := lipgloss.NewStyle().Foreground(ColorSubtle).Italic(true)
				if isCursor {
					descStyle = lipgloss.NewStyle().Foreground(ColorHighlight).Italic(true)
				}
				b.WriteString(fmt.Sprintf("       ↳ 📝 %s\n", descStyle.Render(*item.Description)))
			}
		}
	}

	return b.String()
}
