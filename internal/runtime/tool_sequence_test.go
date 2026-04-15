package runtime

import (
	"reflect"
	"testing"

	"bytemind/internal/llm"
)

func TestNormalizeToolArguments(t *testing.T) {
	if got := NormalizeToolArguments(" "); got != "{}" {
		t.Fatalf("expected empty args to normalize to {}, got %q", got)
	}
	if got := NormalizeToolArguments("{bad"); got != "{bad" {
		t.Fatalf("expected invalid json passthrough, got %q", got)
	}
	if got := NormalizeToolArguments("{\"b\":2,\"a\":1}"); got != `{"a":1,"b":2}` {
		t.Fatalf("expected stable json normalization, got %q", got)
	}
}

func TestSignatureToolCalls(t *testing.T) {
	calls := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file", Arguments: `{"path":"a.go","line":1}`}},
		{Function: llm.ToolFunctionCall{Name: "search_text", Arguments: `{"query":"todo"}`}},
	}
	got := SignatureToolCalls(calls)
	want := `read_file:{"line":1,"path":"a.go"}|search_text:{"query":"todo"}`
	if got != want {
		t.Fatalf("unexpected signature: got=%q want=%q", got, want)
	}
}

func TestUniqueToolCallNames(t *testing.T) {
	calls := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file"}},
		{Function: llm.ToolFunctionCall{Name: "read_file"}},
		{Function: llm.ToolFunctionCall{Name: "search_text"}},
	}
	got := UniqueToolCallNames(calls)
	want := []string{"read_file", "search_text"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected unique names: got=%v want=%v", got, want)
	}
}

func TestToolSequenceTrackerObserve(t *testing.T) {
	tracker := NewToolSequenceTracker(2)
	calls := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file", Arguments: `{}`}},
	}

	first := tracker.Observe(calls)
	if first.RepeatCount != 1 || first.ReachedThreshold {
		t.Fatalf("unexpected first observation: %#v", first)
	}

	second := tracker.Observe(calls)
	if second.RepeatCount != 2 || !second.ReachedThreshold {
		t.Fatalf("unexpected second observation: %#v", second)
	}
}
