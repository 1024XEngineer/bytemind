package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type contractTestTool struct {
	name string
}

func (t contractTestTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "contract test tool",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (contractTestTool) Run(context.Context, json.RawMessage, *ExecutionContext) (string, error) {
	return "ok", nil
}

type contractInvalidSpecTool struct {
	contractTestTool
	spec ToolSpec
}

func (t contractInvalidSpecTool) Spec() ToolSpec {
	return t.spec
}

func TestRegistryContractRegisterRejectsNilTool(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(nil, RegisterOptions{Source: RegistrationSourceBuiltin})
	if err == nil {
		t.Fatal("expected invalid tool error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidTool {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryContractBuiltinRejectsExtensionID(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected invalid source error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidSource {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryContractInvalidSource(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{})
	if err == nil {
		t.Fatal("expected invalid source error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidSource {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.Message == "" {
		t.Fatal("expected message")
	}
}

func TestRegistryContractExtensionRequiresID(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceExtension})
	if err == nil {
		t.Fatal("expected invalid source error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidSource {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.Source != RegistrationSourceExtension {
		t.Fatalf("unexpected source: %q", regErr.Source)
	}
	if regErr.Message == "" {
		t.Fatal("expected message")
	}
}

func TestRegistryContractDuplicateIncludesConflictContext(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.ToolKey != "contract_tool" {
		t.Fatalf("unexpected tool key: %q", regErr.ToolKey)
	}
	if regErr.Source != RegistrationSourceExtension {
		t.Fatalf("unexpected source: %q", regErr.Source)
	}
	if regErr.ExtensionID != "skill.demo" {
		t.Fatalf("unexpected extension id: %q", regErr.ExtensionID)
	}
	if regErr.ConflictWith.Source != RegistrationSourceBuiltin {
		t.Fatalf("unexpected conflict source: %+v", regErr.ConflictWith)
	}
	if regErr.ConflictWith.ToolKey != "contract_tool" {
		t.Fatalf("unexpected conflict tool key: %q", regErr.ConflictWith.ToolKey)
	}
	if regErr.Message == "" {
		t.Fatal("expected message")
	}
}

func TestRegistryContractDuplicateOriginalNameIncludesConflictContext(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(contractTestTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(contractTestTool{name: "skill:skill_demo:read_file"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "read_file",
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.OriginalToolName != "read_file" {
		t.Fatalf("unexpected original tool name: %q", regErr.OriginalToolName)
	}
	if regErr.ConflictWith.Source != RegistrationSourceBuiltin {
		t.Fatalf("unexpected conflict source: %+v", regErr.ConflictWith)
	}
	if regErr.ConflictWith.OriginalToolName != "read_file" {
		t.Fatalf("unexpected conflict original tool name: %q", regErr.ConflictWith.OriginalToolName)
	}
}

func TestRegistryContractDuplicateOriginalNameIsCaseInsensitive(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(contractTestTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(contractTestTool{name: "skill:skill_demo:read_file"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "Read_File",
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.OriginalToolName != "read_file" {
		t.Fatalf("expected normalized original tool name, got %q", regErr.OriginalToolName)
	}
}

func TestRegistryContractInvalidSchemaIncludesCause(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractInvalidSpecTool{
		contractTestTool: contractTestTool{name: "bad_schema_tool"},
		spec:             ToolSpec{Name: "bad_schema_tool", SafetyClass: "invalid"},
	}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected invalid schema error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidSchema {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.ToolKey != "bad_schema_tool" {
		t.Fatalf("unexpected tool key: %q", regErr.ToolKey)
	}
	if regErr.Source != RegistrationSourceExtension {
		t.Fatalf("unexpected source: %q", regErr.Source)
	}
	if regErr.ExtensionID != "skill.demo" {
		t.Fatalf("unexpected extension id: %q", regErr.ExtensionID)
	}
	if regErr.Cause == nil {
		t.Fatal("expected cause")
	}
	if regErr.Message == "" {
		t.Fatal("expected message")
	}
}

func TestRegistryContractRejectsEmptyToolKey(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractTestTool{name: ""}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected invalid tool key error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidToolKey {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryContractRejectsSpecNameMismatch(t *testing.T) {
	registry := &Registry{}
	err := registry.Register(contractInvalidSpecTool{
		contractTestTool: contractTestTool{name: "real_tool"},
		spec:             ToolSpec{Name: "spoofed_tool"},
	}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected invalid tool key error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorInvalidToolKey {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryContractRegisterRejectsDuplicateBuiltin(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(contractTestTool{name: "contract_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryContractNotFound(t *testing.T) {
	registry := &Registry{}
	err := registry.Unregister("missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorNotFound {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.ToolKey != "missing" {
		t.Fatalf("unexpected tool key: %q", regErr.ToolKey)
	}
	if regErr.Message == "" {
		t.Fatal("expected message")
	}
}
