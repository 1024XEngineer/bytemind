package extensions

import "testing"

func TestNopManagerGet(t *testing.T) {
	mgr := NopManager{}
	item, err := mgr.Get(nil, "skill.review")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !item.IsZero() {
		t.Fatal("expected zero extension info")
	}
}

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
		Status:       ExtensionStatusReady,
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

	cases := []ExtensionInfo{
		{ID: "skill.review"},
		{Version: "1.0.0"},
		{Title: "Review"},
		{Description: "desc"},
		{Source: ExtensionSource{Scope: ExtensionScopeProject}},
		{Source: ExtensionSource{Ref: ".bytemind/skills/review"}},
		{Capabilities: CapabilitySet{Tools: 1}},
		{Manifest: Manifest{Name: "review"}},
		{Health: HealthSnapshot{Status: ExtensionStatusReady}},
		{Status: ExtensionStatusReady},
	}
	for _, tc := range cases {
		if tc.IsZero() {
			t.Fatalf("expected non-zero extension info: %+v", tc)
		}
	}
}

func TestExtensionErrorWrap(t *testing.T) {
	err := wrapError(ErrCodeLoadFailed, "load extension", nil)
	extErr, ok := err.(*ExtensionError)
	if !ok {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeLoadFailed {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
	if extErr.Message == "" {
		t.Fatal("expected message")
	}
}
