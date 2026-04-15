package runtime

import "testing"

func TestInMemoryTaskManagerSubmitAndCancel(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(nil, TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected task id")
	}
	if err := mgr.Cancel(nil, id, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	task, err := mgr.Get(nil, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != "killed" {
		t.Fatalf("expected killed status, got %s", task.Status)
	}
}
