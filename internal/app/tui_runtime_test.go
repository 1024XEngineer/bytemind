package app

import "testing"

func TestResolveTUIStartupPolicyInteractiveDisablesGuideAndAPIKeyRequirement(t *testing.T) {
	guide, requireAPIKey := resolveTUIStartupPolicy(true)
	if guide.Active {
		t.Fatal("expected startup guide to stay disabled for interactive tui")
	}
	if requireAPIKey {
		t.Fatal("expected interactive tui to allow startup without API key")
	}
}

func TestResolveTUIStartupPolicyNonInteractiveRequiresAPIKey(t *testing.T) {
	guide, requireAPIKey := resolveTUIStartupPolicy(false)
	if guide.Active {
		t.Fatal("expected startup guide to stay disabled for non-interactive tui")
	}
	if !requireAPIKey {
		t.Fatal("expected non-interactive tui to require API key")
	}
}
