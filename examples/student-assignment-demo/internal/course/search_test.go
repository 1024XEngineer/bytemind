package course

import (
	"testing"
	"time"
)

func sampleAssignments() []Assignment {
	return []Assignment{
		{ID: 1, Course: "Software Engineering", Title: "Design review report", DueAt: time.Date(2026, 5, 20, 23, 59, 0, 0, time.UTC), Done: false},
		{ID: 2, Course: "Database Systems", Title: "SQL practice", DueAt: time.Date(2026, 5, 18, 23, 59, 0, 0, time.UTC), Done: true},
		{ID: 3, Course: "Computer Networks", Title: "Packet tracing lab", DueAt: time.Date(2026, 5, 15, 23, 59, 0, 0, time.UTC), Done: false},
	}
}

func TestSearchAssignmentsMatchesTitleCaseInsensitive(t *testing.T) {
	got := SearchAssignments(sampleAssignments(), "design")
	if len(got) != 1 {
		t.Fatalf("expected one title match, got %d", len(got))
	}
	if got[0].ID != 1 {
		t.Fatalf("expected assignment 1, got %d", got[0].ID)
	}
}

func TestSearchAssignmentsMatchesCourseName(t *testing.T) {
	got := SearchAssignments(sampleAssignments(), "software")
	if len(got) != 1 {
		t.Fatalf("expected one course match, got %d", len(got))
	}
	if got[0].ID != 1 {
		t.Fatalf("expected assignment 1, got %d", got[0].ID)
	}
}

func TestSearchAssignmentsBlankQueryReturnsCopy(t *testing.T) {
	items := sampleAssignments()
	got := SearchAssignments(items, "   ")
	if len(got) != len(items) {
		t.Fatalf("expected all assignments, got %d", len(got))
	}
	got[0].Title = "changed"
	if items[0].Title == "changed" {
		t.Fatal("expected blank query result to be a copy")
	}
}

func TestAssignmentIsOverdue(t *testing.T) {
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	item := Assignment{DueAt: time.Date(2026, 5, 20, 23, 59, 0, 0, time.UTC)}
	if !item.IsOverdue(now) {
		t.Fatal("expected unfinished assignment to be overdue")
	}
	item.Done = true
	if item.IsOverdue(now) {
		t.Fatal("expected completed assignment not to be overdue")
	}
}
