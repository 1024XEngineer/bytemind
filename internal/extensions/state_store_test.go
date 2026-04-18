package extensions

import "testing"

func TestStateStoreBeginLoadBusy(t *testing.T) {
	store := newStateStore()
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("beginLoad failed: %v", err)
	}
	if err := store.beginLoad("skill.review"); err == nil {
		t.Fatal("expected busy error")
	}
}

func TestStateStoreBeginLoadAlreadyLoaded(t *testing.T) {
	store := newStateStore()
	store.set(ExtensionInfo{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject, Ref: "x"}, Status: ExtensionStatusActive})
	if err := store.beginLoad("skill.review"); err == nil {
		t.Fatal("expected already loaded error")
	}
}

func TestStateStoreCancelLoadClearsLoading(t *testing.T) {
	store := newStateStore()
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("beginLoad failed: %v", err)
	}
	store.cancelLoad("skill.review")
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("expected beginLoad to succeed after cancel, got %v", err)
	}
}

func TestStateStoreDeleteClearsLock(t *testing.T) {
	store := newStateStore()
	first := store.lockFor("skill.review")
	store.set(ExtensionInfo{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject, Ref: "x"}, Status: ExtensionStatusActive})
	store.delete("skill.review")
	second := store.lockFor("skill.review")
	if first == second {
		t.Fatal("expected lock to be recreated after delete")
	}
}
