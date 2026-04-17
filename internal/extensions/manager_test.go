package extensions

import (
	"testing"
)

func TestNopManagerList(t *testing.T) {
	mgr := NopManager{}
	items, err := mgr.List(nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no extensions, got %d", len(items))
	}
}

func TestExtensionInfoValid(t *testing.T) {
	valid := ExtensionInfo{
		ID:   "skill.review",
		Name: "review",
		Kind: ExtensionSkill,
		Source: ExtensionSource{
			Scope: ExtensionScopeProject,
			Ref:   ".bytemind/skills/review",
		},
		Capabilities: CapabilitySet{Prompts: 1, Tools: 2},
	}
	if !valid.Valid() {
		t.Fatal("expected extension info to be valid")
	}

	cases := []ExtensionInfo{
		{Name: "review", Kind: ExtensionSkill},
		{ID: "skill.review", Kind: ExtensionSkill},
		{ID: "skill.review", Name: "review"},
	}
	for _, tc := range cases {
		if tc.Valid() {
			t.Fatalf("expected invalid extension info: %+v", tc)
		}
	}
}

func TestExtensionInfoIsZero(t *testing.T) {
	if !((ExtensionInfo{}).IsZero()) {
		t.Fatal("expected zero extension info")
	}
	if (ExtensionInfo{ID: "skill.review"}).IsZero() {
		t.Fatal("expected non-zero extension info")
	}
}
