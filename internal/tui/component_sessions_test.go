package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSessionsModalPaginationAndNavigationBoundaries(t *testing.T) {
	summaries := make([]session.Summary, 0, 17)
	for i := 0; i < 17; i++ {
		summaries = append(summaries, session.Summary{
			ID:              fmtSessionID(i + 1),
			Workspace:       "E:\\repo",
			RawMessageCount: i + 1,
			UpdatedAt:       time.Date(2026, 4, 16, 10, i%60, 0, 0, time.UTC),
		})
	}
	m := model{
		width:        120,
		sessions:     summaries,
		sessionsOpen: true,
	}

	got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyRight})
	updated := got.(model)
	if updated.sessionCursor != 8 {
		t.Fatalf("expected first right page switch to cursor 8, got %d", updated.sessionCursor)
	}

	updated.sessionCursor = 8
	got, _ = updated.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyUp})
	updated = got.(model)
	if updated.sessionCursor != 8 {
		t.Fatalf("expected up to stay within current page start, got %d", updated.sessionCursor)
	}

	updated.sessionCursor = 15
	got, _ = updated.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyDown})
	updated = got.(model)
	if updated.sessionCursor != 15 {
		t.Fatalf("expected down to stay within current page end, got %d", updated.sessionCursor)
	}

	got, _ = updated.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyRight})
	updated = got.(model)
	if updated.sessionCursor != 16 {
		t.Fatalf("expected last page cursor to clamp to index 16, got %d", updated.sessionCursor)
	}

	got, _ = updated.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyLeft})
	updated = got.(model)
	if updated.sessionCursor != 8 {
		t.Fatalf("expected left page switch to keep row offset when possible, got %d", updated.sessionCursor)
	}

	view := updated.renderSessionsModal()
	if !strings.Contains(view, "Page 2/3 · Total 17") {
		t.Fatalf("expected pagination header in sessions modal, got %q", view)
	}
}

func TestSessionsModalRenderUsesTitleBeforePreview(t *testing.T) {
	m := model{
		width: 120,
		sessions: []session.Summary{
			{
				ID:        "with-title",
				Workspace: "E:\\repo",
				Title:     "Chosen Session Title",
				Preview:   "preview should be hidden",
				UpdatedAt: time.Now(),
			},
			{
				ID:        "without-title",
				Workspace: "E:\\repo",
				Preview:   "Fallback preview text",
				UpdatedAt: time.Now(),
			},
		},
	}
	view := m.renderSessionsModal()
	if !strings.Contains(view, "Chosen Session Title") {
		t.Fatalf("expected title to be rendered, got %q", view)
	}
	if strings.Contains(view, "preview should be hidden") {
		t.Fatalf("expected title to take precedence over preview, got %q", view)
	}
	if !strings.Contains(view, "Fallback preview text") {
		t.Fatalf("expected preview fallback when title is missing, got %q", view)
	}
}

func TestDeleteSelectedSessionRemovesEntryAndMovesCursorToNext(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	active := session.New(workspace)
	active.ID = "active"
	active.Messages = []llm.Message{llm.NewUserTextMessage("active")}
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}
	target := session.New(workspace)
	target.ID = "delete-me"
	target.Messages = []llm.Message{llm.NewUserTextMessage("delete")}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}
	keep := session.New(workspace)
	keep.ID = "keep-me"
	keep.Messages = []llm.Message{llm.NewUserTextMessage("keep")}
	if err := store.Save(keep); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          active,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: target.ID, Workspace: workspace}, {ID: keep.ID, Workspace: workspace}},
		input:         textarea.New(),
		screen:        screenChat,
		sessionCursor: 0,
	}
	if err := m.deleteSelectedSession(); err != nil {
		t.Fatalf("expected deleteSelectedSession to succeed, got %v", err)
	}
	if _, err := store.Load(target.ID); !os.IsNotExist(err) {
		t.Fatalf("expected deleted session to be removed, got %v", err)
	}
	if len(m.sessions) != 1 || m.sessions[0].ID != keep.ID {
		t.Fatalf("expected cursor list to keep only next item, got %+v", m.sessions)
	}
	if m.sessionCursor != 0 {
		t.Fatalf("expected cursor to land on next item, got %d", m.sessionCursor)
	}
}

func TestDeleteSelectedSessionLastItemMovesCursorToPrevious(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	active := session.New(workspace)
	active.ID = "active"
	active.Messages = []llm.Message{llm.NewUserTextMessage("active")}
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}
	first := session.New(workspace)
	first.ID = "first"
	first.Messages = []llm.Message{llm.NewUserTextMessage("first")}
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	last := session.New(workspace)
	last.ID = "last"
	last.Messages = []llm.Message{llm.NewUserTextMessage("last")}
	if err := store.Save(last); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          active,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: first.ID, Workspace: workspace}, {ID: last.ID, Workspace: workspace}},
		input:         textarea.New(),
		screen:        screenChat,
		sessionCursor: 1,
	}
	if err := m.deleteSelectedSession(); err != nil {
		t.Fatalf("expected deleteSelectedSession to succeed, got %v", err)
	}
	if len(m.sessions) != 1 || m.sessions[0].ID != first.ID {
		t.Fatalf("expected only previous item to remain, got %+v", m.sessions)
	}
	if m.sessionCursor != 0 {
		t.Fatalf("expected cursor to move to previous item after deleting last row, got %d", m.sessionCursor)
	}
}

func TestDeleteSelectedSessionBlocksBusyActiveSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	active := session.New(workspace)
	active.ID = "active"
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          active,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: active.ID, Workspace: workspace}},
		sessionCursor: 0,
		busy:          true,
		input:         textarea.New(),
	}
	err = m.deleteSelectedSession()
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("expected busy active delete to be rejected, got %v", err)
	}
	if _, err := store.Load(active.ID); err != nil {
		t.Fatalf("expected active session to remain after blocked delete, got %v", err)
	}
}

func TestSessionsModalDeleteKeyShowsBusyActiveError(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	active := session.New(workspace)
	active.ID = "active"
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          active,
		workspace:     workspace,
		sessionsOpen:  true,
		sessions:      []session.Summary{{ID: active.ID, Workspace: workspace}},
		sessionCursor: 0,
		busy:          true,
		input:         textarea.New(),
	}
	got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyDelete})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "in progress") {
		t.Fatalf("expected delete key to surface busy active-session error, got %q", updated.statusNote)
	}
}

func TestDeleteSelectedSessionActiveIdleSwitchesThenDeletes(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	active := session.New(workspace)
	active.ID = "active"
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          active,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: active.ID, Workspace: workspace}},
		sessionCursor: 0,
		input:         textarea.New(),
		screen:        screenChat,
	}
	if err := m.deleteSelectedSession(); err != nil {
		t.Fatalf("expected idle active session delete to succeed, got %v", err)
	}
	if m.sess == nil || m.sess.ID == active.ID {
		t.Fatalf("expected model to switch to a new active session before delete, got %#v", m.sess)
	}
	if _, err := store.Load(active.ID); !os.IsNotExist(err) {
		t.Fatalf("expected old active session to be deleted, got %v", err)
	}
}

func TestSessionCleanupTriggersOnOpenNewAndResume(t *testing.T) {
	t.Run("open", func(t *testing.T) {
		store, workspace, current, zero := prepareCleanupSessions(t)
		m := model{store: store, workspace: workspace, sess: current}
		if err := m.openSessionsModal(); err != nil {
			t.Fatalf("expected openSessionsModal to succeed, got %v", err)
		}
		if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
			t.Fatalf("expected zero session cleanup before opening modal, got %v", err)
		}
	})

	t.Run("new", func(t *testing.T) {
		store, workspace, current, zero := prepareCleanupSessions(t)
		m := model{store: store, workspace: workspace, sess: current, input: textarea.New()}
		if err := m.newSession(); err != nil {
			t.Fatalf("expected newSession to succeed, got %v", err)
		}
		if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
			t.Fatalf("expected zero session cleanup before /new, got %v", err)
		}
	})

	t.Run("resume", func(t *testing.T) {
		store, workspace, current, zero := prepareCleanupSessions(t)
		target := session.New(workspace)
		target.ID = "resume-target"
		target.Messages = []llm.Message{llm.NewUserTextMessage("resume target")}
		if err := store.Save(target); err != nil {
			t.Fatal(err)
		}
		m := model{
			store:     store,
			workspace: workspace,
			sess:      current,
			input:     textarea.New(),
		}
		if err := m.resumeSession("resume-target"); err != nil {
			t.Fatalf("expected resumeSession to succeed, got %v", err)
		}
		if m.sess == nil || m.sess.ID != target.ID {
			t.Fatalf("expected resume target session to become active, got %#v", m.sess)
		}
		if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
			t.Fatalf("expected zero session cleanup before resume, got %v", err)
		}
	})
}

func prepareCleanupSessions(t *testing.T) (*session.Store, string, *session.Session, *session.Session) {
	t.Helper()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	zero := session.New(workspace)
	zero.ID = "zero-cleanup"
	if err := store.Save(zero); err != nil {
		t.Fatal(err)
	}
	return store, workspace, current, zero
}

func fmtSessionID(i int) string {
	return fmt.Sprintf("session-%02d", i)
}
