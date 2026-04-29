package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type listLimitSpyStore struct {
	summaries []session.Summary
	listErr   error
	listLimit int
	listCalls int
}

func (s *listLimitSpyStore) Save(*session.Session) error {
	return nil
}

func (s *listLimitSpyStore) Load(string) (*session.Session, error) {
	return nil, os.ErrNotExist
}

func (s *listLimitSpyStore) List(limit int) ([]session.Summary, []string, error) {
	s.listLimit = limit
	s.listCalls++
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	if limit > 0 && len(s.summaries) > limit {
		out := make([]session.Summary, limit)
		copy(out, s.summaries[:limit])
		return out, nil, nil
	}
	out := make([]session.Summary, len(s.summaries))
	copy(out, s.summaries)
	return out, nil, nil
}

func (s *listLimitSpyStore) DeleteInWorkspace(string, string) error {
	return nil
}

func (s *listLimitSpyStore) CleanupZeroMessageSessions(string, string) (session.CleanupResult, error) {
	return session.CleanupResult{}, nil
}

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
	if !strings.Contains(view, "Page 2/3") || !strings.Contains(view, "Total 17") {
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

func TestHandleSessionsModalKeyEscAndEnterGuards(t *testing.T) {
	t.Run("esc closes modal", func(t *testing.T) {
		m := model{sessionsOpen: true}
		got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyEsc})
		updated := got.(model)
		if updated.sessionsOpen {
			t.Fatal("expected esc to close sessions modal")
		}
	})

	t.Run("enter ignored when busy", func(t *testing.T) {
		m := model{
			sessionsOpen: true,
			busy:         true,
			sessions:     []session.Summary{{ID: "any"}},
		}
		got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyEnter})
		updated := got.(model)
		if updated.statusNote != "" {
			t.Fatalf("expected no status update when enter ignored in busy mode, got %q", updated.statusNote)
		}
	})

	t.Run("enter ignored when empty", func(t *testing.T) {
		m := model{sessionsOpen: true}
		got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyEnter})
		updated := got.(model)
		if updated.statusNote != "" {
			t.Fatalf("expected no status update when enter ignored for empty list, got %q", updated.statusNote)
		}
	})
}

func TestHandleSessionsModalKeyEnterSetsResumeError(t *testing.T) {
	m := model{
		sessionsOpen: true,
		sessions:     []session.Summary{{ID: "missing"}},
	}
	got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "session not found") {
		t.Fatalf("expected resume error status, got %q", updated.statusNote)
	}
}

func TestOpenSessionsModalPropagatesCleanupError(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	m := model{store: store, workspace: workspace, sess: current}
	if err := m.openSessionsModal(); err == nil {
		t.Fatal("expected openSessionsModal to surface cleanup error")
	}
}

func TestNewSessionRequiresStore(t *testing.T) {
	m := model{}
	if err := m.newSession(); err == nil || !strings.Contains(err.Error(), "session store is unavailable") {
		t.Fatalf("expected explicit store unavailable error, got %v", err)
	}
}

func TestReloadSessionsHandlesStoreNilAndListError(t *testing.T) {
	t.Run("store nil", func(t *testing.T) {
		m := model{
			sessions:      []session.Summary{{ID: "keep"}},
			sessionCursor: 3,
		}
		if err := m.reloadSessions(); err != nil {
			t.Fatalf("expected nil store reload to succeed, got %v", err)
		}
		if len(m.sessions) != 0 || m.sessionCursor != 0 {
			t.Fatalf("expected nil store reload to reset sessions/cursor, got %+v cursor=%d", m.sessions, m.sessionCursor)
		}
	})

	t.Run("list error", func(t *testing.T) {
		dir := t.TempDir()
		store, err := session.NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		m := model{store: store}
		if err := m.reloadSessions(); err == nil {
			t.Fatal("expected reloadSessions to return list error")
		}
	})
}

func TestDeleteSelectedSessionRequiresStore(t *testing.T) {
	m := model{
		sessions: []session.Summary{{ID: "a"}},
	}
	if err := m.deleteSelectedSession(); err == nil || !strings.Contains(err.Error(), "session store is unavailable") {
		t.Fatalf("expected explicit store unavailable error, got %v", err)
	}
}

func TestSessionPageHelpersAndResolveSessionIDBranches(t *testing.T) {
	m := model{}
	if got := m.sessionPageCount(); got != 1 {
		t.Fatalf("expected empty list page count 1, got %d", got)
	}
	if got := m.sessionCurrentPage(); got != 0 {
		t.Fatalf("expected empty list current page 0, got %d", got)
	}
	start, end := m.sessionPageBounds(0)
	if start != 0 || end != 0 {
		t.Fatalf("expected empty bounds 0,0 got %d,%d", start, end)
	}

	items := []session.Summary{
		{ID: "abc-1"},
		{ID: "abc-2"},
		{ID: "xyz-1"},
	}
	if got, err := resolveSessionID(items, "xyz-1"); err != nil || got != "xyz-1" {
		t.Fatalf("expected exact match resolution, got id=%q err=%v", got, err)
	}
	if _, err := resolveSessionID(items, "abc"); err == nil {
		t.Fatal("expected ambiguous prefix error")
	}
	if _, err := resolveSessionID(items, "missing"); err == nil {
		t.Fatal("expected missing prefix error")
	}
}

func TestLoadSessionsCmdBranches(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		m := model{}
		msg := m.loadSessionsCmd()().(sessionsLoadedMsg)
		if msg.Err != nil || len(msg.Summaries) != 0 {
			t.Fatalf("expected nil-store load command to return empty result, got %+v", msg)
		}
	})

	t.Run("list error", func(t *testing.T) {
		dir := t.TempDir()
		store, err := session.NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		m := model{store: store}
		msg := m.loadSessionsCmd()().(sessionsLoadedMsg)
		if msg.Err == nil {
			t.Fatalf("expected list error from loadSessionsCmd, got %+v", msg)
		}
	})

	t.Run("success", func(t *testing.T) {
		store, err := session.NewStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		workspace := t.TempDir()
		sess := session.New(workspace)
		sess.ID = "load-success"
		sess.Messages = []llm.Message{llm.NewUserTextMessage("hello")}
		if err := store.Save(sess); err != nil {
			t.Fatal(err)
		}
		m := model{store: store}
		msg := m.loadSessionsCmd()().(sessionsLoadedMsg)
		if msg.Err != nil {
			t.Fatalf("expected loadSessionsCmd success, got err=%v", msg.Err)
		}
		if len(msg.Summaries) == 0 {
			t.Fatalf("expected at least one summary on success, got %+v", msg)
		}
	})
}

func TestReloadSessionsUsesFetchLimit(t *testing.T) {
	spy := &listLimitSpyStore{
		summaries: []session.Summary{
			{ID: "a"}, {ID: "b"}, {ID: "c"},
		},
	}
	m := model{
		store:         spy,
		sessionCursor: 2,
	}
	if err := m.reloadSessions(); err != nil {
		t.Fatalf("expected reloadSessions success, got %v", err)
	}
	if spy.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", spy.listCalls)
	}
	if spy.listLimit != sessionListFetchLimit() {
		t.Fatalf("expected List limit %d, got %d", sessionListFetchLimit(), spy.listLimit)
	}
}

func TestLoadSessionsCmdUsesFetchLimit(t *testing.T) {
	spy := &listLimitSpyStore{
		summaries: []session.Summary{{ID: "one"}},
	}
	m := model{store: spy}
	msg := m.loadSessionsCmd()().(sessionsLoadedMsg)
	if msg.Err != nil {
		t.Fatalf("expected loadSessionsCmd success, got err=%v", msg.Err)
	}
	if spy.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", spy.listCalls)
	}
	if spy.listLimit != sessionListFetchLimit() {
		t.Fatalf("expected List limit %d, got %d", sessionListFetchLimit(), spy.listLimit)
	}
}

func TestReloadSessionsCapsToTenPages(t *testing.T) {
	spy := &listLimitSpyStore{
		summaries: make([]session.Summary, 0, sessionListFetchLimit()+7),
	}
	for i := 0; i < sessionListFetchLimit()+7; i++ {
		spy.summaries = append(spy.summaries, session.Summary{
			ID:        fmt.Sprintf("session-%03d", i),
			Workspace: "E:\\repo",
			UpdatedAt: time.Now().UTC(),
		})
	}

	m := model{store: spy}
	if err := m.reloadSessions(); err != nil {
		t.Fatalf("expected reloadSessions success, got %v", err)
	}
	if len(m.sessions) != sessionListFetchLimit() {
		t.Fatalf("expected sessions capped at %d, got %d", sessionListFetchLimit(), len(m.sessions))
	}
	if m.sessionPageCount() != sessionPageMax {
		t.Fatalf("expected max %d pages, got %d", sessionPageMax, m.sessionPageCount())
	}
}

func TestRenderSessionsModalEmptyAndFallbackTitles(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		m := model{
			width:        120,
			sessionsOpen: true,
		}
		view := m.renderSessionsModal()
		if !strings.Contains(view, "No sessions available.") {
			t.Fatalf("expected empty sessions hint, got %q", view)
		}
		if !strings.Contains(view, "Page 1/1") || !strings.Contains(view, "Total 0") {
			t.Fatalf("expected page header for empty list, got %q", view)
		}
	})

	t.Run("title fallbacks", func(t *testing.T) {
		m := model{
			width: 120,
			sessions: []session.Summary{
				{
					ID:              "fallback-last-user",
					Workspace:       "E:\\repo",
					LastUserMessage: "last user fallback",
					UpdatedAt:       time.Now(),
				},
				{
					ID:        "fallback-default",
					Workspace: "E:\\repo",
					UpdatedAt: time.Now(),
				},
			},
		}
		view := m.renderSessionsModal()
		if !strings.Contains(view, "last user fallback") {
			t.Fatalf("expected LastUserMessage fallback title, got %q", view)
		}
		if !strings.Contains(view, "(no title yet)") {
			t.Fatalf("expected default fallback title, got %q", view)
		}
	})
}

func TestHandleSessionsModalDeleteNoOpWhenListEmpty(t *testing.T) {
	m := model{
		sessionsOpen: true,
	}
	got, cmd := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyDelete})
	updated := got.(model)
	if !updated.sessionsOpen {
		t.Fatal("expected empty-list delete to keep modal open")
	}
	if updated.statusNote != "" {
		t.Fatalf("expected no status note for empty-list delete, got %q", updated.statusNote)
	}
	if cmd != nil {
		t.Fatal("expected empty-list delete not to trigger reload command")
	}
}

func TestHandleSessionsModalEnterSuccessClosesModal(t *testing.T) {
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
	target := session.New(workspace)
	target.ID = "target"
	target.Messages = []llm.Message{llm.NewUserTextMessage("resume me")}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		workspace:     workspace,
		sess:          current,
		sessionsOpen:  true,
		sessions:      []session.Summary{{ID: target.ID, Workspace: workspace}},
		sessionCursor: 0,
		input:         textarea.New(),
	}
	got, _ := m.handleSessionsModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.sessionsOpen {
		t.Fatal("expected enter on valid row to close sessions modal")
	}
	if updated.sess == nil || updated.sess.ID != target.ID {
		t.Fatalf("expected enter to resume selected session, got %#v", updated.sess)
	}
}

func TestDeleteSelectedSessionAdditionalErrorBranches(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		m := model{}
		if err := m.deleteSelectedSession(); err != nil {
			t.Fatalf("expected empty-list delete to be no-op, got %v", err)
		}
	})

	t.Run("active switch failure is wrapped", func(t *testing.T) {
		dir := t.TempDir()
		store, err := session.NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		workspace := t.TempDir()
		active := session.New(workspace)
		active.ID = "active"
		if err := store.Save(active); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}

		m := model{
			store:         store,
			workspace:     workspace,
			sess:          active,
			sessions:      []session.Summary{{ID: active.ID, Workspace: workspace}},
			sessionCursor: 0,
			input:         textarea.New(),
		}
		err = m.deleteSelectedSession()
		if err == nil || !strings.Contains(err.Error(), "failed to switch to a new session before deleting active session") {
			t.Fatalf("expected wrapped switch failure, got %v", err)
		}
	})

	t.Run("invalid target id bubbles delete error", func(t *testing.T) {
		store, err := session.NewStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		m := model{
			store:         store,
			workspace:     t.TempDir(),
			sessions:      []session.Summary{{ID: "   ", Workspace: t.TempDir()}},
			sessionCursor: 0,
		}
		err = m.deleteSelectedSession()
		if err == nil || !strings.Contains(err.Error(), "session id is required") {
			t.Fatalf("expected delete validation error for blank id, got %v", err)
		}
	})
}

func TestSessionPageMovementNoOpBranches(t *testing.T) {
	m := model{
		sessions: []session.Summary{
			{ID: "s1"}, {ID: "s2"}, {ID: "s3"},
		},
	}
	m.moveSessionCursorInPage(0)
	if m.sessionCursor != 0 {
		t.Fatalf("expected zero-delta cursor move to no-op, got %d", m.sessionCursor)
	}

	m.moveSessionPage(0)
	if m.sessionCursor != 0 {
		t.Fatalf("expected zero-delta page move to no-op, got %d", m.sessionCursor)
	}

	m.moveSessionPage(-1)
	if m.sessionCursor != 0 {
		t.Fatalf("expected first-page negative page move to no-op, got %d", m.sessionCursor)
	}

	empty := model{}
	empty.moveSessionCursorInPage(1)
	if empty.sessionCursor != 0 {
		t.Fatalf("expected empty-list cursor move to stay 0, got %d", empty.sessionCursor)
	}
	empty.moveSessionPage(1)
	if empty.sessionCursor != 0 {
		t.Fatalf("expected empty-list page move to stay 0, got %d", empty.sessionCursor)
	}
}

func TestResolveSessionIDSinglePrefixAndSameWorkspaceBranches(t *testing.T) {
	id, err := resolveSessionID([]session.Summary{
		{ID: "alpha-1"},
		{ID: "beta-1"},
	}, "alp")
	if err != nil || id != "alpha-1" {
		t.Fatalf("expected single prefix match, got id=%q err=%v", id, err)
	}

	equivalentLocal := "." + string(os.PathSeparator)
	if !sameWorkspace(".", equivalentLocal) {
		t.Fatal("expected equivalent local workspace paths to match")
	}
	if sameWorkspace(t.TempDir(), t.TempDir()) {
		t.Fatal("expected different workspace paths not to match")
	}

	invalid := "\x00bad"
	if !sameWorkspace(invalid, invalid) {
		t.Fatal("expected same invalid paths to match after fallback normalization")
	}
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
