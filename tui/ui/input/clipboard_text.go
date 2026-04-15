package tui

import (
	"context"

	"github.com/atotto/clipboard"
)

type clipboardTextWriter interface {
	// WriteText copies text into system clipboard storage.
	// Callers should pass a timeout-bounded context to avoid hanging forever
	// on clipboard backends that can block.
	WriteText(ctx context.Context, text string) error
}

type defaultClipboardTextWriter struct{}

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
