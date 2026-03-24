package policy

import (
	"testing"

	"forgecli/internal/config"
)

func TestValidateCommandBlocksDangerousCommands(t *testing.T) {
	t.Parallel()

	p := New(config.Default())
	if err := p.ValidateCommand("rm -rf /"); err == nil {
		t.Fatal("expected dangerous command to be blocked")
	}
}

func TestValidateCommandAllowsSimpleGoTest(t *testing.T) {
	t.Parallel()

	p := New(config.Default())
	if err := p.ValidateCommand("go test ./..."); err != nil {
		t.Fatalf("expected go test to be allowed, got %v", err)
	}
}
