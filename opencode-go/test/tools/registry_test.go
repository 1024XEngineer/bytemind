package tools_test

import (
	"context"
	"github.com/opencode-go/internal/tools"
	"testing"
)

func TestRegistry_NewRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistry_Register(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &MockTool{name: "test"}

	err := reg.Register(tool)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := tools.NewRegistry()
	tool1 := &MockTool{name: "test"}
	tool2 := &MockTool{name: "test"}

	err := reg.Register(tool1)
	if err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	err = reg.Register(tool2)
	if err == nil {
		t.Fatal("Expected error for duplicate registration, got nil")
	}
	if err.Error() != "tool test already registered" {
		t.Fatalf("Unexpected error message: %v", err)
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &MockTool{name: "test"}

	_ = reg.Register(tool)

	got, exists := reg.Get("test")
	if !exists {
		t.Fatal("Tool should exist")
	}
	if got != tool {
		t.Fatal("Got wrong tool")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := tools.NewRegistry()

	_, exists := reg.Get("nonexistent")
	if exists {
		t.Fatal("Tool should not exist")
	}
}

func TestRegistry_Names(t *testing.T) {
	reg := tools.NewRegistry()

	// 注册多个工具
	toolsList := []*MockTool{
		{name: "tool1"},
		{name: "tool2"},
		{name: "tool3"},
	}

	for _, tool := range toolsList {
		_ = reg.Register(tool)
	}

	names := reg.Names()
	if len(names) != 3 {
		t.Fatalf("Expected 3 names, got %d", len(names))
	}

	// 检查是否包含所有名称
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	for _, tool := range toolsList {
		if !nameMap[tool.name] {
			t.Fatalf("Missing tool name: %s", tool.name)
		}
	}
}

func TestRegistry_Count(t *testing.T) {
	reg := tools.NewRegistry()

	if reg.Count() != 0 {
		t.Fatalf("Initial count should be 0, got %d", reg.Count())
	}

	_ = reg.Register(&MockTool{name: "tool1"})
	if reg.Count() != 1 {
		t.Fatalf("Count should be 1, got %d", reg.Count())
	}

	_ = reg.Register(&MockTool{name: "tool2"})
	if reg.Count() != 2 {
		t.Fatalf("Count should be 2, got %d", reg.Count())
	}
}

func TestRegistry_List(t *testing.T) {
	reg := tools.NewRegistry()

	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	tool := &MockTool{
		name:        "test",
		description: "test tool",
		schema:      schema,
	}

	_ = reg.Register(tool)

	definitions := reg.List()
	if len(definitions) != 1 {
		t.Fatalf("Expected 1 definition, got %d", len(definitions))
	}

	def := definitions[0]
	if def.Type != "function" {
		t.Fatalf("Expected type 'function', got %s", def.Type)
	}
	if def.Function.Name != "test" {
		t.Fatalf("Expected name 'test', got %s", def.Function.Name)
	}
	if def.Function.Description != "test tool" {
		t.Fatalf("Expected description 'test tool', got %s", def.Function.Description)
	}
	if def.Function.Parameters == nil {
		t.Fatal("Parameters should not be nil")
	}
	// 检查 parameters 是否包含预期的键（我们的schema包含"type"字段）
	if _, hasType := def.Function.Parameters["type"]; !hasType {
		t.Fatal("Parameters should have 'type' field")
	}
	// 检查 type 是否为 "object"
	if def.Function.Parameters["type"] != "object" {
		t.Fatalf("Expected type 'object', got %v", def.Function.Parameters["type"])
	}
}

func TestRegistry_Clear(t *testing.T) {
	reg := tools.NewRegistry()

	_ = reg.Register(&MockTool{name: "tool1"})
	_ = reg.Register(&MockTool{name: "tool2"})

	if reg.Count() != 2 {
		t.Fatalf("Count should be 2 before clear, got %d", reg.Count())
	}

	reg.Clear()

	if reg.Count() != 0 {
		t.Fatalf("Count should be 0 after clear, got %d", reg.Count())
	}

	_, exists := reg.Get("tool1")
	if exists {
		t.Fatal("Tool should not exist after clear")
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := tools.NewRegistry()

	executed := false
	tool := &MockTool{
		name: "test",
		executeFn: func(ctx context.Context, args map[string]interface{}) (*tools.ToolResult, error) {
			executed = true
			return tools.SuccessResult("test output"), nil
		},
	}

	_ = reg.Register(tool)

	result, err := reg.Execute(context.Background(), "test", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !executed {
		t.Fatal("Tool Execute was not called")
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if result.Output != "test output" {
		t.Fatalf("Expected output 'test output', got %s", result.Output)
	}
}

func TestRegistry_Execute_NotFound(t *testing.T) {
	reg := tools.NewRegistry()

	result, err := reg.Execute(context.Background(), "nonexistent", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute should not return error for non-existent tool, got: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for non-existent tool")
	}
	if result.Error != "tool nonexistent not found" {
		t.Fatalf("Unexpected error message: %s", result.Error)
	}
}

func TestRegistry_Execute_PropagatesError(t *testing.T) {
	reg := tools.NewRegistry()

	tool := &MockTool{
		name: "test",
		executeFn: func(ctx context.Context, args map[string]interface{}) (*tools.ToolResult, error) {
			return tools.ErrorResult("tool error"), nil
		},
	}

	_ = reg.Register(tool)

	result, err := reg.Execute(context.Background(), "test", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure")
	}
	if result.Error != "tool error" {
		t.Fatalf("Expected error 'tool error', got %s", result.Error)
	}
}
