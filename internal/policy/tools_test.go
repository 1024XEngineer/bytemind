package policy

import (
	"reflect"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/skills"
)

func TestResolveToolSetsAllowlistEmptyUsesSentinel(t *testing.T) {
	allow, deny := ResolveToolSets(skills.ToolPolicy{Policy: skills.ToolPolicyAllowlist})
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow[EmptyAllowlistSentinel]; !ok {
		t.Fatalf("expected sentinel in allowlist, got %#v", allow)
	}
}

func TestResolveToolSetsDenylist(t *testing.T) {
	allow, deny := ResolveToolSets(skills.ToolPolicy{Policy: skills.ToolPolicyDenylist, Items: []string{"run_shell", " read_file "}})
	if allow != nil {
		t.Fatalf("expected nil allow set, got %#v", allow)
	}
	if _, ok := deny["run_shell"]; !ok {
		t.Fatalf("missing run_shell in deny set: %#v", deny)
	}
	if _, ok := deny["read_file"]; !ok {
		t.Fatalf("missing read_file in deny set: %#v", deny)
	}
}

func TestSortedToolNames(t *testing.T) {
	got := SortedToolNames(map[string]struct{}{"b": {}, "a": {}, "c": {}})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sort result: got=%v want=%v", got, want)
	}
}
