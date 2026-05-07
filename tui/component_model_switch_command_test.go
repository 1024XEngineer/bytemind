package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
	"github.com/charmbracelet/bubbles/textarea"
)

func TestRunModelCommandOpensPickerAndRefreshesTargets(t *testing.T) {
	runner := &subAgentCommandRunnerStub{
		models: []provider.ModelInfo{
			{ProviderID: "deepseek", ModelID: "deepseek-chat"},
			{ProviderID: "openai", ModelID: "gpt-5.4"},
		},
	}
	m := &model{
		runner: runner,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:    "openai-compatible",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-5.4",
				APIKey:  "openai-key",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.4", APIKey: "openai-key"},
					"deepseek": {
						Type:    "openai-compatible",
						BaseURL: "https://api.deepseek.com",
						Model:   "deepseek-chat",
						APIKey:  "deepseek-key",
					},
				},
			},
		},
	}

	if err := m.runModelCommand("/model picker", []string{"/model", "picker"}); err != nil {
		t.Fatalf("expected /model picker to open picker, got %v", err)
	}
	if !m.modelsOpen {
		t.Fatal("expected model picker to open")
	}
	if m.statusNote != "Opened model picker." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
	if len(m.discoveredModels) != 2 {
		t.Fatalf("expected discovered models to refresh, got %#v", m.discoveredModels)
	}
	targets := m.sortedModelCommandTargets()
	if len(targets) != 2 {
		t.Fatalf("expected two sorted targets, got %#v", targets)
	}
	if got := modelTargetLabel(targets[m.commandCursor]); got != "openai/gpt-5.4" {
		t.Fatalf("expected active target to be selected, got %q", got)
	}
}

func TestRunModelCommandAllowsPickerWhenOnlyOneTargetExists(t *testing.T) {
	runner := &subAgentCommandRunnerStub{
		models: []provider.ModelInfo{
			{ProviderID: "openai", ModelID: "chatgpt-5.4"},
		},
	}
	m := &model{
		runner: runner,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:    "openai-compatible",
				BaseURL: "https://api.openai.com/v1",
				Model:   "chatgpt-5.4",
				APIKey:  "openai-key",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "chatgpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "chatgpt-5.4", APIKey: "openai-key"},
				},
			},
		},
	}

	if err := m.runModelCommand("/model picker", []string{"/model", "picker"}); err != nil {
		t.Fatalf("expected /model picker to open with one target, got %v", err)
	}
	if !m.modelsOpen {
		t.Fatal("expected single-target model picker to stay open")
	}
	if m.statusNote != "Opened model picker." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
}

func TestRunAddCommandOpensStartupGuide(t *testing.T) {
	workspace := t.TempDir()
	m := &model{
		workspace: workspace,
		input:     textarea.New(),
	}

	if err := m.runAddCommand("/add model", []string{"/add", "model"}); err != nil {
		t.Fatalf("expected /add model to open startup guide, got %v", err)
	}
	if !m.startupGuide.Active {
		t.Fatal("expected startup guide to be active")
	}
	if m.startupGuide.Title != "Add model" {
		t.Fatalf("unexpected startup guide title %q", m.startupGuide.Title)
	}
	if m.startupGuide.CurrentField != startupFieldType {
		t.Fatalf("expected startup guide to start at provider type, got %q", m.startupGuide.CurrentField)
	}
	if strings.TrimSpace(m.startupGuide.ConfigPath) == "" {
		t.Fatal("expected writable config path to be resolved")
	}
	if m.statusNote != "Opened add model guide." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
}

func TestRunDeleteCommandOpensDeletePicker(t *testing.T) {
	m := &model{
		runner: &subAgentCommandRunnerStub{},
		cfg: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai":   {Type: "openai-compatible", Model: "gpt-5.4"},
					"deepseek": {Type: "openai-compatible", Model: "deepseek-chat"},
				},
			},
		},
	}

	if err := m.runDeleteCommand("/delete model", []string{"/delete", "model"}); err != nil {
		t.Fatalf("expected /delete model to open picker, got %v", err)
	}
	if !m.modelsOpen {
		t.Fatal("expected delete picker to open")
	}
	if normalizeModelPickerMode(m.modelPickerMode) != modelPickerModeDelete {
		t.Fatalf("expected delete picker mode, got %q", m.modelPickerMode)
	}
	if m.statusNote != "Opened model delete picker." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
}

func TestActivateSelectedModelTargetSwitchesRuntimePersistsConfigAndRefreshesBudget(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "openai-key"
  },
  "provider_runtime": {
    "default_provider": "openai",
    "default_model": "gpt-5.4-mini",
    "providers": {
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key": "openai-key"
      },
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "model": "deepseek-chat",
        "api_key": "deepseek-key"
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &subAgentCommandRunnerStub{
		models: []provider.ModelInfo{
			{ProviderID: "openai", ModelID: "gpt-5.4-mini", Metadata: map[string]string{"context_window": "128000"}},
			{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Metadata: map[string]string{"context_window": "64000"}},
		},
	}
	m := &model{
		runner:     runner,
		tokenUsage: newTokenUsageComponent(),
		workspace:  workspace,
		cfg: config.Config{
			TokenQuota: 1000,
			Provider: config.ProviderConfig{
				Type:    "openai-compatible",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-5.4-mini",
				APIKey:  "openai-key",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4-mini",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.4-mini", APIKey: "openai-key"},
					"deepseek": {
						Type:    "openai-compatible",
						BaseURL: "https://api.deepseek.com",
						Model:   "deepseek-chat",
						APIKey:  "deepseek-key",
					},
				},
			},
		},
		startupGuide: StartupGuide{ConfigPath: configPath},
	}

	if err := m.openModelPicker(); err != nil {
		t.Fatalf("expected picker to open, got %v", err)
	}
	targets := m.sortedModelCommandTargets()
	selected := -1
	for i, target := range targets {
		if modelTargetLabel(target) == "deepseek/deepseek-reasoner" {
			selected = i
			break
		}
	}
	if selected < 0 {
		t.Fatalf("expected picker targets to include deepseek/deepseek-reasoner, got %#v", targets)
	}
	m.commandCursor = selected

	if err := m.activateSelectedModelTarget(); err != nil {
		t.Fatalf("expected picker activation to succeed, got %v", err)
	}
	if m.cfg.ProviderRuntime.DefaultProvider != "deepseek" || m.cfg.ProviderRuntime.DefaultModel != "deepseek-reasoner" {
		t.Fatalf("expected model switch to update runtime defaults, got %#v", m.cfg.ProviderRuntime)
	}
	if m.cfg.Provider.BaseURL != "https://api.deepseek.com" || m.cfg.Provider.Model != "deepseek-reasoner" {
		t.Fatalf("expected legacy provider config to switch, got %#v", m.cfg.Provider)
	}
	if runner.runtimeCfg.DefaultProvider != "deepseek" || runner.runtimeCfg.DefaultModel != "deepseek-reasoner" {
		t.Fatalf("expected runner runtime update, got %#v", runner.runtimeCfg)
	}
	if runner.providerCfg.Model != "deepseek-reasoner" || runner.client == nil {
		t.Fatalf("expected runner provider/client update, got provider=%#v client=%#v", runner.providerCfg, runner.client)
	}
	if m.tokenBudget != 64000 || m.tokenUsage.total != 64000 {
		t.Fatalf("expected switched model context window budget, got budget=%d total=%d", m.tokenBudget, m.tokenUsage.total)
	}
	if m.statusNote != "Model switched and saved." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
	if !strings.Contains(m.chatItems[1].Body, "Switched model to deepseek/deepseek-reasoner.") {
		t.Fatalf("expected switch response in chat, got %#v", m.chatItems)
	}

	cfg, err := config.Load(workspace, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderRuntime.DefaultProvider != "deepseek" || cfg.ProviderRuntime.DefaultModel != "deepseek-reasoner" {
		t.Fatalf("expected persisted runtime defaults, got %#v", cfg.ProviderRuntime)
	}
	if cfg.Provider.BaseURL != "https://api.deepseek.com" || cfg.Provider.Model != "deepseek-reasoner" {
		t.Fatalf("expected persisted provider section to switch, got %#v", cfg.Provider)
	}
}

func TestDeleteSelectedModelTargetRemovesConfiguredProviderAndPersistsConfig(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "openai-key"
  },
  "provider_runtime": {
    "default_provider": "openai",
    "default_model": "gpt-5.4-mini",
    "providers": {
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key": "openai-key"
      },
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "model": "deepseek-chat",
        "api_key": "deepseek-key"
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &subAgentCommandRunnerStub{
		models: []provider.ModelInfo{
			{ProviderID: "openai", ModelID: "gpt-5.4-mini", Metadata: map[string]string{"context_window": "128000"}},
			{ProviderID: "deepseek", ModelID: "deepseek-chat", Metadata: map[string]string{"context_window": "64000"}},
			{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Metadata: map[string]string{"context_window": "64000"}},
		},
	}
	m := &model{
		runner:     runner,
		tokenUsage: newTokenUsageComponent(),
		workspace:  workspace,
		cfg: config.Config{
			TokenQuota: 1000,
			Provider: config.ProviderConfig{
				Type:    "openai-compatible",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-5.4-mini",
				APIKey:  "openai-key",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4-mini",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.4-mini", APIKey: "openai-key"},
					"deepseek": {
						Type:    "openai-compatible",
						BaseURL: "https://api.deepseek.com",
						Model:   "deepseek-chat",
						APIKey:  "deepseek-key",
					},
				},
			},
		},
		startupGuide: StartupGuide{ConfigPath: configPath},
		discoveredModels: []provider.ModelInfo{
			{ProviderID: "openai", ModelID: "gpt-5.4-mini", Metadata: map[string]string{"context_window": "128000"}},
			{ProviderID: "deepseek", ModelID: "deepseek-chat", Metadata: map[string]string{"context_window": "64000"}},
			{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Metadata: map[string]string{"context_window": "64000"}},
		},
	}

	if err := m.openModelDeletePicker(); err != nil {
		t.Fatalf("expected delete picker to open, got %v", err)
	}
	targets := m.sortedConfiguredModelCommandTargets()
	selected := -1
	for i, target := range targets {
		if modelTargetLabel(target) == "deepseek/deepseek-chat" {
			selected = i
			break
		}
	}
	if selected < 0 {
		t.Fatalf("expected configured targets to include deepseek/deepseek-chat, got %#v", targets)
	}
	m.commandCursor = selected

	if err := m.deleteSelectedModelTarget(); err != nil {
		t.Fatalf("expected picker deletion to succeed, got %v", err)
	}
	if _, ok := m.cfg.ProviderRuntime.Providers["deepseek"]; ok {
		t.Fatalf("expected deepseek provider to be removed, got %#v", m.cfg.ProviderRuntime.Providers)
	}
	if m.cfg.ProviderRuntime.DefaultProvider != "openai" || m.cfg.ProviderRuntime.DefaultModel != "gpt-5.4-mini" {
		t.Fatalf("expected openai to remain active default, got %#v", m.cfg.ProviderRuntime)
	}
	if runner.runtimeCfg.DefaultProvider != "openai" || len(runner.runtimeCfg.Providers) != 1 {
		t.Fatalf("expected runner runtime update after delete, got %#v", runner.runtimeCfg)
	}
	if len(m.discoveredModels) != 1 || modelTargetLabel(m.discoveredModels[0]) != "openai/gpt-5.4-mini" {
		t.Fatalf("expected discovered models for deleted provider to be pruned, got %#v", m.discoveredModels)
	}
	if m.statusNote != "Model deleted and saved." {
		t.Fatalf("unexpected status note %q", m.statusNote)
	}
	if !strings.Contains(m.chatItems[1].Body, "Deleted model deepseek/deepseek-chat.") {
		t.Fatalf("expected delete response in chat, got %#v", m.chatItems)
	}

	cfg, err := config.Load(workspace, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.ProviderRuntime.Providers["deepseek"]; ok {
		t.Fatalf("expected persisted runtime provider removal, got %#v", cfg.ProviderRuntime.Providers)
	}
	if cfg.Provider.Model != "gpt-5.4-mini" || cfg.Provider.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected persisted provider section to remain on openai, got %#v", cfg.Provider)
	}
}

func TestDeleteSelectedModelTargetRejectsDeletingLastConfiguredTarget(t *testing.T) {
	m := &model{
		runner: &subAgentCommandRunnerStub{},
		cfg: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", Model: "gpt-5.4"},
				},
			},
		},
	}

	if err := m.openModelDeletePicker(); err != nil {
		t.Fatalf("expected delete picker to open, got %v", err)
	}
	err := m.deleteSelectedModelTarget()
	if err == nil || !strings.Contains(err.Error(), "cannot delete the last configured model") {
		t.Fatalf("expected last-target delete to fail, got %v", err)
	}
}

func TestRunModelCommandRejectsLegacyForms(t *testing.T) {
	m := &model{runner: &subAgentCommandRunnerStub{}}

	for _, fields := range [][]string{
		{"/model"},
		{"/model", "list"},
		{"/model", "set", "openai/gpt-5.4"},
		{"/model", "openai/gpt-5.4"},
	} {
		if err := m.runModelCommand(strings.Join(fields, " "), fields); err == nil || err.Error() != modelCommandUsage {
			t.Fatalf("expected legacy model form %v to fail with %q, got %v", fields, modelCommandUsage, err)
		}
	}
}

func TestRunModelCommandRejectsUnknownTarget(t *testing.T) {
	m := &model{
		runner: &subAgentCommandRunnerStub{
			models: []provider.ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}},
		},
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:    "openai-compatible",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-5.4",
				APIKey:  "openai-key",
			},
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.4", APIKey: "openai-key"},
				},
			},
		},
	}

	err := m.switchModelCommandTarget("/model openai/missing", "openai/missing")
	if err == nil || !strings.Contains(err.Error(), "unknown model target") {
		t.Fatalf("expected unknown target error, got %v", err)
	}
}
