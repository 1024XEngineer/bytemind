package agent

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestParseAssistantTurnIntentFromTag(t *testing.T) {
	reply := llm.NewAssistantTextMessage("<turn_intent>finalize</turn_intent>Done.")
	intent, cleaned, explicit := parseAssistantTurnIntent(reply)
	if !explicit {
		t.Fatal("expected turn intent tag to be explicit")
	}
	if intent != turnIntentFinalize {
		t.Fatalf("expected finalize intent, got %q", intent)
	}
	if cleaned.Content != "Done." {
		t.Fatalf("expected cleaned content without tag, got %q", cleaned.Content)
	}
}

func TestInferAssistantTurnIntent(t *testing.T) {
	if got := inferAssistantTurnIntent("I will inspect files now."); got != turnIntentContinueWork {
		t.Fatalf("expected continue_work, got %q", got)
	}
	if got := inferAssistantTurnIntent("I will continue shortly."); got != turnIntentUnknown {
		t.Fatalf("expected unknown for vague no-tag continue text, got %q", got)
	}
	if got := inferAssistantTurnIntent("Please confirm if you want me to continue."); got != turnIntentAskUser {
		t.Fatalf("expected ask_user, got %q", got)
	}
	if got := inferAssistantTurnIntent("All done. Final answer below."); got != turnIntentFinalize {
		t.Fatalf("expected finalize for explicit completion text, got %q", got)
	}
	if got := inferAssistantTurnIntent("Thanks for waiting."); got != turnIntentUnknown {
		t.Fatalf("expected unknown for neutral final text, got %q", got)
	}
}

func TestAdaptiveTurnStateLimits(t *testing.T) {
	state := newAdaptiveTurnState(1)
	if state.maxSemanticRepairs != 2 {
		t.Fatalf("expected max semantic repairs to be 2, got %d", state.maxSemanticRepairs)
	}
	state.recordNoProgressTurn()
	state.recordNoProgressTurn()
	state.recordNoProgressTurn()
	if !state.exceededNoProgressLimit() {
		t.Fatal("expected no-progress limit to be reached")
	}
}
