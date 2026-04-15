package tui

import (
	"context"
	"testing"
	"time"
)

func TestDefaultClipboardTextWriterReturnsWhenContextCanceled(t *testing.T) {
	writer := defaultClipboardTextWriter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- writer.WriteText(ctx, "probe")
	}()

	select {
	case <-done:
		// We only require the call to return quickly once context is canceled.
	case <-time.After(2 * time.Second):
		t.Fatalf("expected clipboard writer to return after context cancellation")
	}
}
