// Command client is the todoapp terminal UI.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"todoapp/internal/client/config"
	"todoapp/internal/client/tui"
)

func main() {
	serverFlag := flag.String("server", "", "server URL (e.g. http://192.168.96.215:8081)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config (%v), using defaults\n", err)
	}

	if *serverFlag != "" {
		cfg.ServerURL = *serverFlag
	}

	model := tui.New(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running TUI: %v\n", err)
		os.Exit(1)
	}
}
