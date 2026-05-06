package agent

import (
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/tools"
)

func TestNonInteractiveApprovalAlwaysApproves(t *testing.T) {
	handler := nonInteractiveApproval()
	approved, err := handler(tools.ApprovalRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !approved.Approved() {
		t.Fatal("expected auto-approval")
	}
}

func TestThreadSafeObserverNilInner(t *testing.T) {
	o := &threadSafeObserver{inner: nil}
	// Should not panic
	o.HandleEvent(Event{})
}

func TestThreadSafeObserverConcurrentAccess(t *testing.T) {
	var count int
	var mu sync.Mutex
	o := &threadSafeObserver{
		inner: ObserverFunc(func(Event) {
			mu.Lock()
			count++
			mu.Unlock()
		}),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.HandleEvent(Event{})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != 100 {
		t.Fatalf("expected 100 events handled, got %d", count)
	}
}

func TestNoOpObserverDiscardsEvents(t *testing.T) {
	o := &noOpObserver{}
	// Should not panic
	o.HandleEvent(Event{})
	o.HandleEvent(Event{Type: "test"})
}

func TestSubAgentStdoutReturnsDiscard(t *testing.T) {
	w := subAgentStdout()
	if w == nil {
		t.Fatal("expected non-nil writer")
	}
	n, err := w.Write([]byte("test"))
	if err != nil {
		t.Fatalf("expected no error writing to discard, got %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes written, got %d", n)
	}
}
