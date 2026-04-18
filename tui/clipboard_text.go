package tui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
)

type clipboardTextWriter interface {
	// WriteText copies text into system clipboard storage.
	// Callers should pass a timeout-bounded context to avoid hanging forever
	// on clipboard backends that can block.
	WriteText(ctx context.Context, text string) error
}

type clipboardTextReader interface {
	// ReadText reads plain text from system clipboard storage.
	// Callers should pass a timeout-bounded context to avoid hanging forever
	// on clipboard backends that can block.
	ReadText(ctx context.Context) (string, error)
}

type defaultClipboardTextWriter struct{}
type defaultClipboardTextReader struct{}

func (defaultClipboardTextWriter) WriteText(ctx context.Context, text string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan error, 1)
	go func() {
		done <- clipboard.WriteAll(text)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (defaultClipboardTextReader) ReadText(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := clipboard.ReadAll()
		done <- result{text: text, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case out := <-done:
		if out.err == nil && strings.TrimSpace(out.text) != "" {
			return out.text, nil
		}
		// On some Windows terminals/environments, the generic clipboard backend
		// can fail while Ctrl+V still pastes text in the terminal. Retry via
		// PowerShell to preserve a deterministic paste boundary.
		if runtime.GOOS == "windows" {
			if fallback, fallbackErr := readClipboardTextViaPowerShell(ctx); fallbackErr == nil && strings.TrimSpace(fallback) != "" {
				return fallback, nil
			}
		}
		return out.text, out.err
	}
}

func readClipboardTextViaPowerShell(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"Get-Clipboard -Raw",
	)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
