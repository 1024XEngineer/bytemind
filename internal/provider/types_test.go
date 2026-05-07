package provider

import (
	"errors"
	"testing"
)

func TestProviderErrorStringAndUnwrap(t *testing.T) {
	base := errors.New("base error")

	if (*Error)(nil).Error() != "" {
		t.Fatal("expected nil error string to be empty")
	}
	if (*Error)(nil).Unwrap() != nil {
		t.Fatal("expected nil unwrap to be nil")
	}

	withMessage := &Error{Code: ErrCodeUnavailable, Message: "trimmed message", Err: base}
	if withMessage.Error() != "trimmed message" {
		t.Fatalf("unexpected message: %q", withMessage.Error())
	}

	withWrapped := &Error{Code: ErrCodeUnavailable, Err: base}
	if withWrapped.Error() != "base error" {
		t.Fatalf("unexpected wrapped message: %q", withWrapped.Error())
	}
	if !errors.Is(withWrapped, base) {
		t.Fatal("expected unwrap to expose base error")
	}

	withCodeOnly := &Error{Code: ErrCodeBadRequest}
	if withCodeOnly.Error() != string(ErrCodeBadRequest) {
		t.Fatalf("unexpected code-only message: %q", withCodeOnly.Error())
	}
}

func TestModelInfoMetadataParsing(t *testing.T) {
	info := ModelInfo{
		ProviderID: "openai",
		ModelID:    "gpt-5.4",
		Metadata: map[string]string{
			"provider_family":   "openai-compatible",
			"context_window":    "128000",
			"max_output_tokens": "4096",
			"supports_tools":    "true",
			"usage_source":      "discovered",
		},
	}
	metadata := info.ModelMetadata()
	if metadata.Family != "openai-compatible" {
		t.Fatalf("unexpected family %q", metadata.Family)
	}
	if metadata.ContextWindow != 128000 || metadata.MaxOutputTokens != 4096 {
		t.Fatalf("unexpected token metadata %+v", metadata)
	}
	if !metadata.SupportsTools {
		t.Fatal("expected supports_tools to parse true")
	}
	if metadata.UsageSource != "discovered" {
		t.Fatalf("unexpected usage source %q", metadata.UsageSource)
	}

	metadata.Metadata["context_window"] = "mutated"
	if info.Metadata["context_window"] != "128000" {
		t.Fatalf("expected metadata clone to protect original, got %#v", info.Metadata)
	}
}

func TestModelInfoMetadataParsingFallbacks(t *testing.T) {
	info := ModelInfo{
		ProviderID: "local",
		ModelID:    "model",
		Metadata: map[string]string{
			"family":            "custom",
			"context_window":    "-1",
			"max_output_tokens": "not-a-number",
			"supports_tools":    "not-bool",
			"source":            "config",
		},
	}
	metadata := info.ModelMetadata()
	if metadata.Family != "custom" {
		t.Fatalf("expected family fallback, got %q", metadata.Family)
	}
	if metadata.ContextWindow != 0 || metadata.MaxOutputTokens != 0 {
		t.Fatalf("expected invalid ints to parse as zero, got %+v", metadata)
	}
	if metadata.SupportsTools {
		t.Fatal("expected invalid bool to parse false")
	}
	if metadata.UsageSource != "config" {
		t.Fatalf("expected source fallback, got %q", metadata.UsageSource)
	}

	empty := (ModelInfo{}).ModelMetadata()
	if len(empty.Metadata) != 0 {
		t.Fatalf("expected empty metadata map, got %#v", empty.Metadata)
	}
}
