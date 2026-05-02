package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type senderCall struct {
	Title string
	Body  string
}

type stubSender struct {
	mu      sync.Mutex
	calls   []senderCall
	err     error
	started chan struct{}
	block   chan struct{}
}

func (s *stubSender) Send(ctx context.Context, title, body string) error {
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	s.calls = append(s.calls, senderCall{Title: title, Body: body})
	s.mu.Unlock()
	return s.err
}

func (s *stubSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubSender) callTitles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.calls))
	for _, call := range s.calls {
		out = append(out, call.Title)
	}
	return out
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout %s", timeout)
}

func TestDesktopNotifierCooldownByKey(t *testing.T) {
	now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	sender := &stubSender{}
	notifier := newDesktopNotifierWithSender(
		DesktopConfig{Enabled: true, CooldownSeconds: 3, QueueSize: 8, SendTimeoutMs: 1000},
		sender,
		func() time.Time { return now },
	)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := notifier.Close(closeCtx); err != nil {
			t.Fatalf("close notifier: %v", err)
		}
	}()

	notifier.Notify(Message{Event: EventApprovalRequired, Title: "Approval", Body: "need approval", Key: "approval|go test"})
	notifier.Notify(Message{Event: EventApprovalRequired, Title: "Approval", Body: "need approval", Key: "approval|go test"})
	waitForCondition(t, time.Second, func() bool { return sender.callCount() == 1 })

	now = now.Add(4 * time.Second)
	notifier.Notify(Message{Event: EventApprovalRequired, Title: "Approval", Body: "need approval", Key: "approval|go test"})
	waitForCondition(t, time.Second, func() bool { return sender.callCount() == 2 })
}

func TestDesktopNotifierQueueFullDropsOldest(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 4)
	sender := &stubSender{block: block, started: started}
	notifier := newDesktopNotifierWithSender(
		DesktopConfig{Enabled: true, CooldownSeconds: 0, QueueSize: 1, SendTimeoutMs: 2000},
		sender,
		time.Now,
	)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := notifier.Close(closeCtx); err != nil {
			t.Fatalf("close notifier: %v", err)
		}
	}()

	notifier.Notify(Message{Event: EventApprovalRequired, Title: "first", Body: "first body", Key: "k1"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("expected first send to start")
	}

	notifier.Notify(Message{Event: EventApprovalRequired, Title: "second", Body: "second body", Key: "k2"})
	notifier.Notify(Message{Event: EventApprovalRequired, Title: "third", Body: "third body", Key: "k3"})

	if notifier.droppedCount() < 1 {
		t.Fatalf("expected dropped count >= 1, got %d", notifier.droppedCount())
	}

	close(block)
	waitForCondition(t, 2*time.Second, func() bool { return sender.callCount() >= 2 })
	titles := sender.callTitles()
	if len(titles) < 2 {
		t.Fatalf("expected at least two sends, got %v", titles)
	}
	if titles[0] != "first" {
		t.Fatalf("expected first message to be delivered first, got %v", titles)
	}
	foundThird := false
	for _, title := range titles {
		if title == "third" {
			foundThird = true
			break
		}
	}
	if !foundThird {
		t.Fatalf("expected newest message to survive queue overflow, got %v", titles)
	}
}

func TestDesktopNotifierSenderFailureDoesNotStopQueue(t *testing.T) {
	sender := &stubSender{err: errors.New("send failed")}
	notifier := newDesktopNotifierWithSender(
		DesktopConfig{Enabled: true, CooldownSeconds: 0, QueueSize: 4, SendTimeoutMs: 100},
		sender,
		time.Now,
	)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := notifier.Close(closeCtx); err != nil {
			t.Fatalf("close notifier: %v", err)
		}
	}()

	notifier.Notify(Message{Event: EventRunFailed, Title: "failed-1", Body: "first", Key: "run_failed|1"})
	notifier.Notify(Message{Event: EventRunFailed, Title: "failed-2", Body: "second", Key: "run_failed|2"})
	waitForCondition(t, time.Second, func() bool { return sender.callCount() == 2 })
}

func TestDesktopNotifierCloseHonorsContextTimeout(t *testing.T) {
	block := make(chan struct{})
	sender := &stubSender{block: block}
	notifier := newDesktopNotifierWithSender(
		DesktopConfig{Enabled: true, CooldownSeconds: 0, QueueSize: 2, SendTimeoutMs: 2000},
		sender,
		time.Now,
	)
	notifier.Notify(Message{Event: EventRunCompleted, Title: "done", Body: "done", Key: "run_completed|1"})

	closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := notifier.Close(closeCtx); err == nil {
		t.Fatalf("expected close timeout while sender is blocked")
	}

	close(block)
	finalCtx, finalCancel := context.WithTimeout(context.Background(), time.Second)
	defer finalCancel()
	if err := notifier.Close(finalCtx); err != nil {
		t.Fatalf("expected notifier to close after unblock, got %v", err)
	}
}
