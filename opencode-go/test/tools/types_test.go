package tools_test

import (
	"github.com/opencode-go/internal/tools"
	"testing"
)

func TestSuccessResult(t *testing.T) {
	result := tools.SuccessResult("test")
	if !result.Success {
		t.Error("Expected success")
	}
	if result.Output != "test" {
		t.Errorf("Expected 'test', got %s", result.Output)
	}
}
