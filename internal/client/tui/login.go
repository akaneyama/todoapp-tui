package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"todoapp/internal/client/api"
	"todoapp/internal/client/config"
	"todoapp/internal/models"
)

type loginSuccessMsg struct {
	User  models.User
	Token string
}

type loginErrMsg struct {
	Err error
}

type loginMode int

const (
	modeLogin loginMode = iota
	modeRegister
)

type loginModel struct {
	apiClient *api.Client
	cfg       config.Config
	mode      loginMode
	focusIdx  int
	inputs    []textinput.Model
	spinner   spinner.Model
	loading   bool
	errText   string
	width     int
	height    int
}

func newLoginModel(apiClient *api.Client, cfg config.Config) loginModel {
	inputs := make([]textinput.Model, 3)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "username (3-32 chars)"
	inputs[0].Focus()
	inputs[0].Prompt = "  Username: "
	inputs[0].CharLimit = 32
	if cfg.Username != "" {
		inputs[0].SetValue(cfg.Username)
	}

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "password (min 8 chars)"
	inputs[1].Prompt = "  Password: "
	inputs[1].EchoMode = textinput.EchoPassword
	inputs[1].EchoCharacter = '•'
	inputs[1].CharLimit = 128

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "http://192.168.96.215:8081"
	inputs[2].Prompt = "Server URL: "
	inputs[2].CharLimit = 256
	if cfg.ServerURL != "" {
		inputs[2].SetValue(cfg.ServerURL)
	} else {
		inputs[2].SetValue("http://192.168.96.215:8081")
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	return loginModel{
		apiClient: apiClient,
		cfg:       cfg,
		mode:      modeLogin,
		focusIdx:  0,
		inputs:    inputs,
		spinner:   s,
	}
}

func (m loginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m loginModel) Update(msg tea.Msg) (loginModel, tea.Cmd) {
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

	case loginErrMsg:
		m.loading = false
		m.errText = msg.Err.Error()
		return m, nil

	case loginSuccessMsg:
		m.loading = false
		m.errText = ""
		m.inputs[1].SetValue("")
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.String() {
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "up" || msg.String() == "shift+tab" {
				m.focusIdx--
				if m.focusIdx < 0 {
					m.focusIdx = len(m.inputs) - 1
				}
			} else {
				m.focusIdx++
				if m.focusIdx >= len(m.inputs) {
					m.focusIdx = 0
				}
			}
			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i < len(m.inputs); i++ {
				if i == m.focusIdx {
					cmds[i] = m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, tea.Batch(cmds...)

		case "ctrl+v":
			_, _ = PasteIntoTextInput(&m.inputs[m.focusIdx])
			return m, nil

		case "ctrl+c":
			if m.inputs[m.focusIdx].Value() != "" {
				_ = CopyToClipboard(m.inputs[m.focusIdx].Value())
			}
			return m, nil

		case "ctrl+t", "f2":
			if m.mode == modeLogin {
				m.mode = modeRegister
			} else {
				m.mode = modeLogin
			}
			m.errText = ""
			return m, nil

		case "enter":
			return m.submit()
		}
	}

	// Update active textinput
	var cmd tea.Cmd
	m.inputs[m.focusIdx], cmd = m.inputs[m.focusIdx].Update(msg)
	return m, cmd
}

func (m *loginModel) submit() (loginModel, tea.Cmd) {
	username := strings.TrimSpace(m.inputs[0].Value())
	password := m.inputs[1].Value()
	serverURL := strings.TrimSpace(m.inputs[2].Value())

	if username == "" || password == "" {
		m.errText = "Username and password cannot be empty"
		return *m, nil
	}
	if serverURL == "" {
		serverURL = "http://192.168.96.215:8081"
	}

	m.loading = true
	m.errText = ""

	// Update base URL on API client
	m.apiClient = api.New(serverURL, "")
	m.cfg.ServerURL = serverURL
	m.cfg.Username = username

	mode := m.mode
	client := m.apiClient

	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if mode == modeRegister {
			// Register then immediately login
			u, err := client.Register(ctx, username, password, "UTC")
			if err != nil {
				return loginErrMsg{Err: err}
			}
			loginResp, err := client.Login(ctx, username, password, "UTC")
			if err != nil {
				return loginErrMsg{Err: err}
			}
			return loginSuccessMsg{User: u, Token: loginResp.Token}
		}

		loginResp, err := client.Login(ctx, username, password, "UTC")
		if err != nil {
			return loginErrMsg{Err: err}
		}
		return loginSuccessMsg{User: loginResp.User, Token: loginResp.Token}
	}

	return *m, tea.Batch(cmd, m.spinner.Tick)
}

func (m loginModel) View() string {
	modeTitle := "LOGIN"
	toggleHint := "Belum punya akun? Tekan [Ctrl+T] atau [F2] untuk Register"
	if m.mode == modeRegister {
		modeTitle = "REGISTER"
		toggleHint = "Sudah punya akun? Tekan [Ctrl+T] atau [F2] untuk Login"
	}

	header := AppTitleStyle.Render("todoapp") + " " + lipgloss.NewStyle().Bold(true).Foreground(ColorHighlight).Render("— "+modeTitle)

	var b strings.Builder
	b.WriteString(header + "\n\n")

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View() + "\n\n")
	}

	if m.loading {
		b.WriteString(m.spinner.View() + " Menghubungkan ke server...\n\n")
	} else if m.errText != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render("Error: "+m.errText) + "\n\n")
	}

	b.WriteString(lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Tab/Shift+Tab] Pindah field  •  [Ctrl+V] Tempel  •  [Enter] Submit\n"+toggleHint) + "\n")

	box := ContentBoxStyle.Width(54).Render(b.String())

	// Center in terminal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *loginModel) Reset() {
	m.loading = false
	m.errText = ""
	m.inputs[0].SetValue("")
	m.inputs[1].SetValue("")
	m.focusIdx = 0
	m.inputs[0].Focus()
	m.inputs[1].Blur()
	m.inputs[2].Blur()
}
