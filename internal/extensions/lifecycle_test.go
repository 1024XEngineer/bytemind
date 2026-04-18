package extensions

import "testing"

func TestActivateTransition(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusLoaded}
	next, event, err := activateTransition(info)
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if next.Status != ExtensionStatusActive {
		t.Fatalf("unexpected status: %q", next.Status)
	}
	if event.Type != "activate" {
		t.Fatalf("unexpected event: %q", event.Type)
	}
}

func TestActivateTransitionRejectsInvalidState(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusStopped}
	_, _, err := activateTransition(info)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestDegradeTransitionUsesDefaults(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusActive}
	next, event, err := degradeTransition(info, "", ErrCodeLoadFailed)
	if err != nil {
		t.Fatalf("degrade failed: %v", err)
	}
	if next.Status != ExtensionStatusDegraded {
		t.Fatalf("unexpected status: %q", next.Status)
	}
	if next.Health.LastError != ErrCodeLoadFailed {
		t.Fatalf("unexpected error code: %q", next.Health.LastError)
	}
	if event.Reason != "extension degraded" {
		t.Fatalf("unexpected reason: %q", event.Reason)
	}
}

func TestDegradeTransitionRejectsInvalidState(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusLoaded}
	_, _, err := degradeTransition(info, "broken", ErrCodeLoadFailed)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestRecoverTransitionUsesDefaults(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusDegraded}
	next, event, err := recoverTransition(info, "")
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if next.Status != ExtensionStatusActive {
		t.Fatalf("unexpected status: %q", next.Status)
	}
	if event.Reason != "extension recovered" {
		t.Fatalf("unexpected reason: %q", event.Reason)
	}
}

func TestRecoverTransitionRejectsInvalidState(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusLoaded}
	_, _, err := recoverTransition(info, "ok")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestStopTransitionRejectsStoppedToStopped(t *testing.T) {
	info := ExtensionInfo{ID: "skill.review", Kind: ExtensionSkill, Status: ExtensionStatusStopped}
	_, _, err := stopTransition(info, "stop")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestStateStoreListReturnsSnapshot(t *testing.T) {
	store := newStateStore()
	store.set(ExtensionInfo{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject, Ref: "x"}, Status: ExtensionStatusActive})
	items := store.list()
	items[0].ID = "changed"
	got, ok := store.get("skill.review")
	if !ok {
		t.Fatal("expected stored item")
	}
	if got.ID != "skill.review" {
		t.Fatalf("expected snapshot isolation, got %q", got.ID)
	}
}
