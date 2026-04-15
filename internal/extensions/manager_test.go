package extensions

import "testing"

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
