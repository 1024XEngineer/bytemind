package agent

import (
	"reflect"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
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

func TestSignatureToolCallNames(t *testing.T) {
	calls := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`}},
		{Function: llm.ToolFunctionCall{Name: "search_text", Arguments: `{"query":"todo"}`}},
	}
	got := SignatureToolCallNames(calls)
	want := "read_file|search_text"
	if got != want {
		t.Fatalf("unexpected name-only signature: got=%q want=%q", got, want)
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

func TestToolSequenceTrackerDetectsNameOnlyRepeats(t *testing.T) {
	tracker := NewToolSequenceTracker(2)
	first := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file", Arguments: `{"path":"a.go"}`}},
	}
	second := []llm.ToolCall{
		{Function: llm.ToolFunctionCall{Name: "read_file", Arguments: `{"path":"b.go"}`}},
	}

	one := tracker.Observe(first)
	if one.ReachedThreshold {
		t.Fatalf("unexpected first observation: %#v", one)
	}
	two := tracker.Observe(second)
	if two.ReachedThreshold {
		t.Fatalf("did not expect name-only repeat to reach stop threshold, got %#v", two)
	}
	if two.NameOnlyRepeat != 2 {
		t.Fatalf("expected name-only repeat count to be tracked as 2, got %#v", two)
	}
	if two.MatchMode != "name_only" {
		t.Fatalf("expected name_only match mode, got %#v", two)
	}
}
