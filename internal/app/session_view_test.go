package app

import (
	"testing"
	"time"

	"bytemind/internal/session"
)

func TestBuildSessionSnapshotView(t *testing.T) {
	sess := &session.Session{
		ID:        "sess-1",
		Workspace: "C:\\repo",
		UpdatedAt: time.Date(2026, 4, 14, 12, 30, 45, 0, time.UTC),
	}
	view := BuildSessionSnapshotView(sess, time.UTC)
	if view.ID != "sess-1" || view.Workspace != "C:\\repo" {
		t.Fatalf("unexpected snapshot view: %#v", view)
	}
	if view.Updated != "2026-04-14 12:30:45" {
		t.Fatalf("unexpected formatted updated time: %#v", view)
	}
}

func TestBuildSessionSummaryViews(t *testing.T) {
	summaries := []session.Summary{
		{
			ID:              "s1",
			Workspace:       "C:\\repo-a",
			UpdatedAt:       time.Date(2026, 4, 14, 10, 5, 0, 0, time.UTC),
			LastUserMessage: "",
			MessageCount:    1,
		},
		{
			ID:              "s2",
			Workspace:       "C:\\repo-b",
			UpdatedAt:       time.Date(2026, 4, 14, 11, 15, 0, 0, time.UTC),
			LastUserMessage: "hello",
			MessageCount:    3,
		},
	}
	views := BuildSessionSummaryViews(summaries, "s2", time.UTC)
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	if views[0].Preview != "(no user prompt yet)" {
		t.Fatalf("expected fallback preview, got %#v", views[0])
	}
	if views[1].Marker != "*" {
		t.Fatalf("expected current marker '*', got %#v", views[1])
	}
	if views[1].Updated != "2026-04-14 11:15" {
		t.Fatalf("unexpected formatted summary time: %#v", views[1])
	}
}
