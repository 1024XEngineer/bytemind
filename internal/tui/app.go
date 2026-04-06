package tui

import (
	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	Runner       *agent.Runner
	Store        *session.Store
	Session      *session.Session
	ImageStore   assets.ImageStore
	Config       config.Config
	Workspace    string
	StartupGuide StartupGuide
}

type StartupGuide struct {
	Active       bool
	Title        string
	Status       string
	Lines        []string
	ConfigPath   string
	CurrentField string
}

func Run(opts Options) error {
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if shouldEnableMouseCapture() {
		programOptions = append(programOptions, tea.WithMouseCellMotion())
	}
	program := tea.NewProgram(newModel(opts), programOptions...)
	_, err := program.Run()
	return err
}

func shouldEnableMouseCapture() bool {
	return parseMouseCaptureEnv(os.Getenv("BYTEMIND_ENABLE_MOUSE"))
}

func parseMouseCaptureEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
