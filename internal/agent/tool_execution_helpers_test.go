package agent

import (
	"reflect"
	"testing"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
)

func TestToSandboxExecRulesCopiesConfigRules(t *testing.T) {
	if got := toSandboxExecRules(nil); got != nil {
		t.Fatalf("expected nil sandbox exec rules for nil input, got %#v", got)
	}

	input := []configpkg.ExecAllowRule{
		{Command: "go", ArgsPattern: []string{"test", "./..."}},
	}
	got := toSandboxExecRules(input)
	if len(got) != 1 {
		t.Fatalf("expected one sandbox exec rule, got %#v", got)
	}
	if got[0].Command != "go" || !reflect.DeepEqual(got[0].ArgsPattern, []string{"test", "./..."}) {
		t.Fatalf("unexpected converted exec rule: %#v", got[0])
	}

	got[0].ArgsPattern[0] = "run"
	if input[0].ArgsPattern[0] != "test" {
		t.Fatalf("expected args pattern copy isolation, got input %#v", input[0].ArgsPattern)
	}
}

func TestToSandboxNetworkRulesCopiesConfigRules(t *testing.T) {
	if got := toSandboxNetworkRules(nil); got != nil {
		t.Fatalf("expected nil sandbox network rules for nil input, got %#v", got)
	}

	input := []configpkg.NetworkAllowRule{
		{Host: "api.openai.com", Port: 443, Scheme: "https"},
	}
	got := toSandboxNetworkRules(input)
	if len(got) != 1 {
		t.Fatalf("expected one sandbox network rule, got %#v", got)
	}
	if got[0].Host != "api.openai.com" || got[0].Port != 443 || got[0].Scheme != "https" {
		t.Fatalf("unexpected converted network rule: %#v", got[0])
	}
}
