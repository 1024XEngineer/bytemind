package tui

import (
	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"os"
	"runtime"
	"strconv"
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
	ensureZoneManager()
	applyAutoMouseYOffset()
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if shouldUseInputTTY() {
		// Keep direct console input opt-in on Windows.
		// It can help mouse reporting in some terminals, but it may break CJK/IME input.
		programOptions = append(programOptions, tea.WithInputTTY())
	}
	if shouldEnableMouseCapture() {
		programOptions = append(programOptions, tea.WithMouseAllMotion())
	}
	program := tea.NewProgram(newModel(opts), programOptions...)
	_, err := program.Run()
	return err
}

func shouldEnableMouseCapture() bool {
	return parseMouseCaptureEnv(os.Getenv("BYTEMIND_ENABLE_MOUSE"))
}

func shouldUseInputTTY() bool {
	return runtime.GOOS == "windows" && parseInputTTYEnv(os.Getenv("BYTEMIND_WINDOWS_INPUT_TTY"))
}

func parseMouseCaptureEnv(value string) bool {
	// Default to enabled so wheel/drag/select behavior works out of the box.
	// Most users expect mouse capture unless explicitly disabled.
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func parseInputTTYEnv(value string) bool {
	// Keep this opt-in by default. On Windows terminals, input TTY mode can
	// improve mouse reporting but may disrupt IME/CJK typing behavior.
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "", "0", "false", "no", "off":
		return false
	default:
		return false
	}
}

func applyAutoMouseYOffset() {
	if offset, ok := defaultAutoMouseYOffset(
		runtime.GOOS,
		os.Getenv("BYTEMIND_WINDOWS_INPUT_TTY"),
		os.Getenv("WT_SESSION"),
		os.Getenv("TERM_PROGRAM"),
		os.Getenv("BYTEMIND_MOUSE_Y_OFFSET"),
	); ok {
		if err := os.Setenv("BYTEMIND_MOUSE_Y_OFFSET", strconv.Itoa(offset)); err != nil {
			warnSetenv("BYTEMIND_MOUSE_Y_OFFSET", err)
		}
	}
}

func defaultAutoMouseYOffset(goos, inputTTY, wtSession, termProgram, existing string) (int, bool) {
	// Respect explicit user/project override first.
	if strings.TrimSpace(existing) != "" {
		return 0, false
	}
	// In some Windows terminal hosts (notably Windows Terminal and VSCode
	// integrated terminal), mouse Y can be reported a couple rows above visual
	// position when running Bubble Tea fullscreen UIs without input TTY mode.
	isWindowsTerminalHost := strings.TrimSpace(wtSession) != "" || strings.EqualFold(strings.TrimSpace(termProgram), "vscode")
	if goos == "windows" && isWindowsTerminalHost && !parseInputTTYEnv(inputTTY) {
		return 2, true
	}
	return 0, false
}
