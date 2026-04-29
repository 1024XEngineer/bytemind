package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
)

type stubRegistryClient struct {
	providerID ProviderID
	models     []ModelInfo
	err        error
}

type stubListRegistry struct {
	listErr error
}

func (s stubListRegistry) Register(context.Context, Client) error         { return nil }
func (s stubListRegistry) Get(context.Context, ProviderID) (Client, bool) { return nil, false }
func (s stubListRegistry) List(context.Context) ([]ProviderID, error)     { return nil, s.listErr }

func (s stubRegistryClient) ProviderID() ProviderID                                { return s.providerID }
func (s stubRegistryClient) ListModels(context.Context) ([]ModelInfo, error)       { return s.models, s.err }
func (s stubRegistryClient) Stream(context.Context, Request) (<-chan Event, error) { return nil, nil }

func TestNewRegistryFromProviderConfigSupportsLegacyMode(t *testing.T) {
	reg, err := NewRegistryFromProviderConfig(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	ids, err := reg.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids) != 1 || ids[0] != ProviderOpenAI {
		t.Fatalf("unexpected ids %#v", ids)
	}
}

func TestNewRegistrySupportsMultipleProvidersWithSameType(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{
		DefaultProvider: " OpenAI-Primary ",
		Providers: map[string]config.ProviderConfig{
			" OpenAI-Primary ": {Type: "openai-compatible", BaseURL: "https://api.openai.com/v1", APIKey: "key-1", Model: "gpt-5.4"},
			"OPENAI-SECONDARY": {Type: "openai-compatible", BaseURL: "https://example.com/v1", APIKey: "key-2", Model: "gpt-4.1"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	ids, err := reg.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids) != 2 || ids[0] != "openai-primary" || ids[1] != "openai-secondary" {
		t.Fatalf("unexpected ids %#v", ids)
	}
	primary, ok := reg.Get(context.Background(), "openai-primary")
	if !ok || primary.ProviderID() != "openai-primary" {
		t.Fatalf("unexpected primary provider %#v ok=%v", primary, ok)
	}
	secondary, ok := reg.Get(context.Background(), "openai-secondary")
	if !ok || secondary.ProviderID() != "openai-secondary" {
		t.Fatalf("unexpected secondary provider %#v ok=%v", secondary, ok)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(models) != 2 || len(warnings) != 0 {
		t.Fatalf("unexpected models=%#v warnings=%#v", models, warnings)
	}
}

func TestRegistryRejectsDuplicateProvider(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI}); err != nil {
		t.Fatalf("unexpected first register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI}); err == nil {
		t.Fatal("expected duplicate provider error")
	} else {
		var providerErr *Error
		if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeDuplicateProvider {
			t.Fatalf("unexpected error %#v", err)
		}
	}
}

func TestListModelsAggregatesWarningsAndDeduplicates(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI, models: []ModelInfo{{ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}, {ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}, {ProviderID: "", ModelID: "gpt-4.1"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderAnthropic, err: errors.New("list failed")}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(models) != 2 || models[0].ProviderID != ProviderOpenAI || models[0].ModelID != "gpt-4.1" || models[1].ProviderID != ProviderOpenAI || models[1].ModelID != "gpt-5.4" {
		t.Fatalf("unexpected models %#v", models)
	}
	if len(warnings) != 1 || warnings[0].ProviderID != ProviderAnthropic || warnings[0].Reason != listModelsWarningReason {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
}

func TestListModelsKeepsInstanceIDsDistinctEvenWhenAliasesOverlap(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: "openai", models: []ModelInfo{{ProviderID: "openai-compatible", ModelID: "gpt-5.4"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: "openai-compatible", models: []ModelInfo{{ProviderID: "openai", ModelID: "gpt-5.4"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
	if len(models) != 2 || models[0].ProviderID != "openai" || models[1].ProviderID != "openai-compatible" {
		t.Fatalf("unexpected models %#v", models)
	}
}

func TestListModelsReturnsRegistryError(t *testing.T) {
	reg := stubListRegistry{listErr: errors.New("boom")}
	if _, _, err := ListModels(context.Background(), reg); err == nil {
		t.Fatal("expected registry list error")
	}
}

func TestListModelsReturnsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, _, err := ListModels(ctx, reg); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestRegistryCoversLookupAndNormalizationBranches(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: "openai-primary"}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	client, ok := reg.Get(context.Background(), "openai-primary")
	if !ok || client.ProviderID() != "openai-primary" {
		t.Fatalf("unexpected client lookup result ok=%v client=%#v", ok, client)
	}
	if _, ok := reg.Get(context.Background(), "missing"); ok {
		t.Fatal("expected missing provider lookup to fail")
	}
	ids, err := reg.List(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "openai-primary" {
		t.Fatalf("unexpected ids %#v err=%v", ids, err)
	}
}

func TestRegistryHandlesProviderNotFoundAndConfigErrors(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), nil); err != nil {
		t.Fatalf("expected nil client register to be ignored, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ""}); err == nil {
		t.Fatal("expected provider not found error")
	} else {
		var providerErr *Error
		if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeProviderNotFound {
			t.Fatalf("unexpected error %#v", err)
		}
	}
	if normalizeRegistryProviderID(" OpenAI-Primary ") != "openai-primary" {
		t.Fatal("expected registry provider id normalization to trim and lowercase")
	}
	if _, err := NewRegistry(config.ProviderRuntimeConfig{DefaultProvider: "missing", Providers: map[string]config.ProviderConfig{"openai-primary": {Type: "openai-compatible", BaseURL: "https://example.com", APIKey: "key", Model: "m"}}}); err == nil {
		t.Fatal("expected missing default provider error")
	}
	if _, err := NewRegistry(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{" OpenAI-Primary ": {Type: "openai-compatible", BaseURL: "https://example.com", APIKey: "key", Model: "m"}, "openai-primary": {Type: "openai-compatible", BaseURL: "https://example2.com", APIKey: "key", Model: "m"}}}); err == nil {
		t.Fatal("expected duplicate normalized provider error")
	}
	if _, err := NewRegistry(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{"   ": {Type: "openai-compatible", BaseURL: "https://example.com", APIKey: "key", Model: "m"}}}); err == nil {
		t.Fatal("expected blank provider id error")
	}
	if _, err := NewRegistry(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{"broken": {BaseURL: "https://example.com", APIKey: "key", Model: "m"}}}); err == nil {
		t.Fatal("expected invalid provider type error")
	}
}
