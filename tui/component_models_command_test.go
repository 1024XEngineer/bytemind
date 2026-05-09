package tui

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

func TestFormatModelsStatusGroupsSortsLabelsAndWarnings(t *testing.T) {
	cfg := config.Config{
		ProviderRuntime: config.ProviderRuntimeConfig{
			DefaultProvider: "openai",
			DefaultModel:    "gpt-5.4",
		},
	}
	status := formatModelsStatus(cfg, []provider.ModelInfo{
		{ProviderID: "deepseek", ModelID: "deepseek-chat", Metadata: map[string]string{"family": "deepseek"}},
		{ProviderID: "openai", ModelID: "gpt-5.4-mini", Metadata: map[string]string{"family": "openai"}},
		{ProviderID: "openai", ModelID: "gpt-5.4", Metadata: map[string]string{"family": "openai"}},
	}, []provider.Warning{{ProviderID: "deepseek", Reason: "provider_list_models_failed"}})

	for _, want := range []string{
		"active: openai/gpt-5.4",
		"default provider: openai",
		"* openai",
		"  - gpt-5.4  [active, default, family=openai]",
		"  - gpt-5.4-mini  [family=openai]",
		"- deepseek",
		"  - deepseek-chat  [family=deepseek]",
		"warnings:",
		"- deepseek: provider_list_models_failed",
	} {
		if !strings.Contains(status, want) {
			t.Fatalf("expected status to contain %q, got:\n%s", want, status)
		}
	}
	if strings.Index(status, "- deepseek") > strings.Index(status, "* openai") {
		t.Fatalf("expected providers to be sorted, got:\n%s", status)
	}
}

func TestFormatModelsStatusFallbackLabelsWhenEmpty(t *testing.T) {
	status := formatModelsStatus(config.Config{}, nil, nil)
	for _, want := range []string{
		"active: unknown",
		"default provider: unknown",
		"No models discovered.",
	} {
		if !strings.Contains(status, want) {
			t.Fatalf("expected status to contain %q, got:\n%s", want, status)
		}
	}

	legacyStatus := formatModelsStatus(config.Config{
		Provider: config.ProviderConfig{Model: "legacy-model"},
	}, nil, nil)
	if !strings.Contains(legacyStatus, "active: legacy-model") {
		t.Fatalf("expected legacy model fallback, got:\n%s", legacyStatus)
	}
}

func TestRunModelsCommandOpensModelPickerAlias(t *testing.T) {
	m := &model{
		runner:     &subAgentCommandRunnerStub{},
		tokenUsage: newTokenUsageComponent(),
		cfg: config.Config{
			TokenQuota: 1000,
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", Model: "gpt-5.4"},
				},
			},
		},
	}

	if err := m.runModelsCommand("/models", []string{"/models"}); err != nil {
		t.Fatalf("expected /models to run, got %v", err)
	}
	if !m.modelsOpen {
		t.Fatal("expected /models to open the model picker")
	}
	if m.statusNote != "Opened model picker." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
	if len(m.chatItems) != 0 {
		t.Fatalf("expected alias not to append chat items, got %#v", m.chatItems)
	}
}

func TestRunModelsCommandRejectsInvalidState(t *testing.T) {
	if err := (&model{}).runModelsCommand("/models", []string{"/models"}); err == nil || !strings.Contains(err.Error(), "runner is unavailable") {
		t.Fatalf("expected missing runner error, got %v", err)
	}

	m := &model{runner: &subAgentCommandRunnerStub{}}
	if err := m.runModelsCommand("/models status", []string{"/models", "status"}); err != nil {
		t.Fatalf("expected /models status compatibility alias, got %v", err)
	}
	if err := m.runModelsCommand("/models delete", []string{"/models", "delete"}); err == nil || err.Error() != modelsCommandUsage {
		t.Fatalf("expected invalid /models subcommand to fail with %q, got %v", modelsCommandUsage, err)
	}
}
