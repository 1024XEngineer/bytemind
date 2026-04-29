package runtime

import (
	"testing"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestValidateTaskTransitionAllowsKnownTransitions(t *testing.T) {
	tests := []struct {
		name string
		from corepkg.TaskStatus
		to   corepkg.TaskStatus
		opts TransitionOptions
	}{
		{
			name: "pending to running",
			from: corepkg.TaskPending,
			to:   corepkg.TaskRunning,
		},
		{
			name: "pending to killed",
			from: corepkg.TaskPending,
			to:   corepkg.TaskKilled,
		},
		{
			name: "pending to failed",
			from: corepkg.TaskPending,
			to:   corepkg.TaskFailed,
		},
		{
			name: "running to completed",
			from: corepkg.TaskRunning,
			to:   corepkg.TaskCompleted,
		},
		{
			name: "running to failed",
			from: corepkg.TaskRunning,
			to:   corepkg.TaskFailed,
		},
		{
			name: "running to killed",
			from: corepkg.TaskRunning,
			to:   corepkg.TaskKilled,
		},
		{
			name: "failed to pending through retry",
			from: corepkg.TaskFailed,
			to:   corepkg.TaskPending,
			opts: TransitionOptions{AllowRetryTransition: true},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTaskTransition(tt.from, tt.to, tt.opts); err != nil {
				t.Fatalf("expected transition %s -> %s to be valid: %v", tt.from, tt.to, err)
			}
		})
	}
}

func TestValidateTaskTransitionRejectsIllegalTransitions(t *testing.T) {
	tests := []struct {
		name string
		from corepkg.TaskStatus
		to   corepkg.TaskStatus
		opts TransitionOptions
	}{
		{
			name: "completed to running",
			from: corepkg.TaskCompleted,
			to:   corepkg.TaskRunning,
		},
		{
			name: "killed to running",
			from: corepkg.TaskKilled,
			to:   corepkg.TaskRunning,
		},
		{
			name: "running to pending without retry path",
			from: corepkg.TaskRunning,
			to:   corepkg.TaskPending,
		},
		{
			name: "failed to pending without retry flag",
			from: corepkg.TaskFailed,
			to:   corepkg.TaskPending,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTaskTransition(tt.from, tt.to, tt.opts)
			if err == nil {
				t.Fatalf("expected transition %s -> %s to fail", tt.from, tt.to)
			}
			if !hasErrorCode(err, ErrorCodeInvalidTransition) {
				t.Fatalf("expected error code %q, got %q", ErrorCodeInvalidTransition, errorCode(err))
			}
		})
	}
}

func TestIsTerminalTaskStatus(t *testing.T) {
	if !IsTerminalTaskStatus(corepkg.TaskCompleted) {
		t.Fatal("expected completed to be terminal")
	}
	if !IsTerminalTaskStatus(corepkg.TaskFailed) {
		t.Fatal("expected failed to be terminal")
	}
	if !IsTerminalTaskStatus(corepkg.TaskKilled) {
		t.Fatal("expected killed to be terminal")
	}
	if IsTerminalTaskStatus(corepkg.TaskRunning) {
		t.Fatal("expected running to be non-terminal")
	}
}
