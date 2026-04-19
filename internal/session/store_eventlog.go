package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

const (
	eventSchemaVersion = 1

	eventTypeSessionCreated    = "session_created"
	eventTypeModeSwitched      = "mode_switched"
	eventTypeTurnAppended      = "turn_appended"
	eventTypeSessionClosed     = "session_closed"
	eventTypeSnapshotCompacted = "snapshot_compacted"

	recentEventIDWindowSize = 256
	defaultReadFromLimit    = 128
	defaultSnapshotEveryN   = int64(20)
	defaultSnapshotEveryT   = 30 * time.Second
)

type SessionEvent struct {
	EventID       string          `json:"event_id"`
	SessionID     string          `json:"session_id"`
	Seq           int64           `json:"seq"`
	Type          string          `json:"type"`
	TS            time.Time       `json:"ts"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type modeSwitchedPayload struct {
	Mode planpkg.AgentMode `json:"mode"`
}

type turnAppendedPayload struct {
	Messages []llm.Message `json:"messages,omitempty"`
	Replace  bool          `json:"replace,omitempty"`
}

type sessionClosedPayload struct {
	ClosedAt time.Time `json:"closed_at,omitempty"`
}

type fullSessionPayload struct {
	Session Session `json:"session"`
}

type sessionSnapshot struct {
	SchemaVersion int       `json:"schema_version"`
	SessionID     string    `json:"session_id"`
	LastSeq       int64     `json:"last_seq"`
	SavedAt       time.Time `json:"saved_at"`
	Session       Session   `json:"session"`
}

type eventDraft struct {
	eventID string
	kind    string
	payload any
}

type eventIDWindow struct {
	size  int
	order []string
	set   map[string]struct{}
}

func newEventIDWindow(size int) *eventIDWindow {
	if size <= 0 {
		size = recentEventIDWindowSize
	}
	return &eventIDWindow{
		size:  size,
		order: make([]string, 0, size),
		set:   make(map[string]struct{}, size),
	}
}

func (w *eventIDWindow) Has(id string) bool {
	if w == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	_, ok := w.set[id]
	return ok
}

func (w *eventIDWindow) Add(id string) {
	if w == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if _, ok := w.set[id]; ok {
		return
	}
	if len(w.order) >= w.size {
		oldest := w.order[0]
		w.order = w.order[1:]
		delete(w.set, oldest)
	}
	w.order = append(w.order, id)
	w.set[id] = struct{}{}
}

func (s *Store) Replay(sessionID string) (*Session, error) {
	return s.load(sessionID)
}

func (s *Store) ReadEvents(sessionID string, afterSeq int64) ([]SessionEvent, error) {
	source, err := s.findSessionSource(sessionID)
	if err != nil {
		return nil, err
	}
	if source.kind != sourceKindEvents {
		return nil, os.ErrNotExist
	}
	return readEventsFromFile(source.paths.Events, afterSeq)
}

func (s *Store) ReadFrom(sessionID string, offset int64, limit int) ([]SessionEvent, int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, 0, errors.New("session id is required")
	}
	source, err := s.findSessionSource(sessionID)
	if err != nil {
		return nil, 0, err
	}
	if source.kind != sourceKindEvents {
		return nil, 0, os.ErrNotExist
	}
	return readEventsFromOffset(source.paths.Events, offset, limit)
}

func (s *Store) Snapshot(sessionID string) (err error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	unlock, err := s.lockSession(sessionID)
	if err != nil {
		return err
	}
	defer func() {
		err = combineUnlockError(err, unlock, sessionID)
	}()

	source, err := s.findSessionSource(sessionID)
	if err != nil {
		return err
	}
	if source.kind != sourceKindEvents {
		return os.ErrNotExist
	}

	state, lastSeq, _, err := s.replayFromEventStore(source.paths)
	if err != nil {
		return err
	}
	return s.writeSnapshot(source.paths, state, lastSeq, s.now())
}

func (s *Store) load(sessionID string) (*Session, error) {
	source, err := s.findSessionSource(sessionID)
	if err != nil {
		return nil, err
	}

	switch source.kind {
	case sourceKindEvents:
		state, _, _, err := s.replayFromEventStore(source.paths)
		return state, err
	case sourceKindLegacy:
		return loadLegacySessionFile(s.files, source.paths.Legacy)
	default:
		return nil, os.ErrNotExist
	}
}

func (s *Store) save(session *Session) (err error) {
	paths, err := s.pathForSession(session)
	if err != nil {
		return err
	}
	unlock, err := s.lockSession(paths.SessionID)
	if err != nil {
		return err
	}
	defer func() {
		err = combineUnlockError(err, unlock, paths.SessionID)
	}()

	window, err := s.eventWindow(paths.SessionID, paths.Events)
	if err != nil {
		return err
	}

	current, currentSeq, snap, err := s.loadCurrentForWrite(paths)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	drafts := s.buildSaveEventDrafts(current, session)
	appendedAny := false
	for _, draft := range drafts {
		eventID := strings.TrimSpace(draft.eventID)
		if eventID == "" {
			eventID = s.newEventID()
		}
		if window.Has(eventID) {
			continue
		}

		rawPayload, err := json.Marshal(draft.payload)
		if err != nil {
			return err
		}
		currentSeq++
		event := SessionEvent{
			EventID:       eventID,
			SessionID:     paths.SessionID,
			Seq:           currentSeq,
			Type:          draft.kind,
			TS:            s.now(),
			SchemaVersion: eventSchemaVersion,
			Payload:       rawPayload,
		}
		if err := appendEventLine(s.files, paths.Events, event); err != nil {
			return err
		}
		window.Add(eventID)
		appendedAny = true
	}

	if !appendedAny {
		return nil
	}
	if shouldRefreshSnapshot(snap, currentSeq, s.snapshotEveryN, s.snapshotEveryT, s.now()) {
		if err := s.writeSnapshot(paths, session, currentSeq, s.now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadCurrentForWrite(paths sessionPaths) (*Session, int64, *sessionSnapshot, error) {
	state, lastSeq, snap, err := s.replayFromEventStore(paths)
	if err == nil {
		return state, lastSeq, snap, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil, err
	}

	legacy, legacyErr := loadLegacySessionFile(s.files, paths.Legacy)
	if legacyErr == nil {
		return legacy, 0, nil, nil
	}
	if errors.Is(legacyErr, os.ErrNotExist) {
		return nil, 0, nil, os.ErrNotExist
	}
	return nil, 0, nil, legacyErr
}

func (s *Store) replayFromEventStore(paths sessionPaths) (*Session, int64, *sessionSnapshot, error) {
	snap, snapErr := readSnapshotFile(paths.Snapshot)
	events, eventsErr := readEventsFromFile(paths.Events, 0)
	if snapErr != nil && !errors.Is(snapErr, os.ErrNotExist) {
		return nil, 0, nil, snapErr
	}
	if eventsErr != nil && !errors.Is(eventsErr, os.ErrNotExist) {
		return nil, 0, nil, eventsErr
	}
	if errors.Is(snapErr, os.ErrNotExist) && errors.Is(eventsErr, os.ErrNotExist) {
		return nil, 0, nil, os.ErrNotExist
	}
	if events == nil {
		events = make([]SessionEvent, 0)
	}

	state, lastSeq, err := replaySnapshotAndEvents(paths.SessionID, snap, events)
	if err != nil {
		return nil, 0, nil, err
	}
	return state, lastSeq, snap, nil
}

func (s *Store) buildSaveEventDrafts(previous, current *Session) []eventDraft {
	drafts := make([]eventDraft, 0, 3)
	if previous == nil {
		drafts = append(drafts, eventDraft{
			kind:    eventTypeSessionCreated,
			payload: fullSessionPayload{Session: *current},
		})
	} else {
		if previous.Mode != current.Mode {
			drafts = append(drafts, eventDraft{
				kind: eventTypeModeSwitched,
				payload: modeSwitchedPayload{
					Mode: current.Mode,
				},
			})
		}
		if payload, ok := buildTurnAppendedPayload(previous, current); ok {
			drafts = append(drafts, eventDraft{
				kind:    eventTypeTurnAppended,
				payload: payload,
			})
		}
	}

	drafts = append(drafts, eventDraft{
		kind:    eventTypeSnapshotCompacted,
		payload: fullSessionPayload{Session: *current},
	})
	return drafts
}

func buildTurnAppendedPayload(previous, current *Session) (turnAppendedPayload, bool) {
	before := sessionTimeline(previous)
	after := sessionTimeline(current)
	if reflect.DeepEqual(before, after) {
		return turnAppendedPayload{}, false
	}
	if len(after) >= len(before) && reflect.DeepEqual(before, after[:len(before)]) {
		appended := append([]llm.Message(nil), after[len(before):]...)
		return turnAppendedPayload{
			Messages: appended,
			Replace:  false,
		}, len(appended) > 0
	}
	replaced := append([]llm.Message(nil), after...)
	return turnAppendedPayload{
		Messages: replaced,
		Replace:  true,
	}, true
}

func shouldRefreshSnapshot(last *sessionSnapshot, currentSeq, threshold int64, interval time.Duration, now time.Time) bool {
	if last == nil {
		return true
	}
	if threshold > 0 && currentSeq-last.LastSeq >= threshold {
		return true
	}
	if interval > 0 {
		lastAt := last.SavedAt
		if lastAt.IsZero() {
			return true
		}
		if now.Sub(lastAt) >= interval {
			return true
		}
	}
	return false
}

func (s *Store) writeSnapshot(paths sessionPaths, state *Session, lastSeq int64, at time.Time) error {
	if state == nil {
		return errors.New("session state is required")
	}
	if at.IsZero() {
		at = s.now()
	}
	record := sessionSnapshot{
		SchemaVersion: eventSchemaVersion,
		SessionID:     paths.SessionID,
		LastSeq:       lastSeq,
		SavedAt:       at.UTC(),
		Session:       *state,
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return s.files.WriteAtomic(paths.Snapshot, content)
}

func replaySnapshotAndEvents(sessionID string, snap *sessionSnapshot, events []SessionEvent) (*Session, int64, error) {
	var state *Session
	lastSeq := int64(0)
	if snap != nil {
		copied, err := cloneSessionValue(&snap.Session)
		if err != nil {
			return nil, 0, err
		}
		state = copied
		lastSeq = snap.LastSeq
	}

	seenEventIDs := make(map[string]struct{}, len(events))
	currentSeq := lastSeq
	for _, event := range events {
		if event.Seq <= lastSeq {
			continue
		}
		eventID := strings.TrimSpace(event.EventID)
		if eventID == "" {
			return nil, currentSeq, fmt.Errorf("event seq %d missing event_id", event.Seq)
		}
		if _, duplicated := seenEventIDs[eventID]; duplicated {
			continue
		}
		if event.Seq <= currentSeq {
			return nil, currentSeq, fmt.Errorf("non-monotonic event seq for session %s: seq=%d <= %d", sessionID, event.Seq, currentSeq)
		}

		next, err := applyEvent(state, event)
		if err != nil {
			return nil, currentSeq, err
		}
		state = next
		seenEventIDs[eventID] = struct{}{}
		currentSeq = event.Seq
	}

	if state == nil {
		return nil, currentSeq, os.ErrNotExist
	}
	if strings.TrimSpace(state.ID) == "" {
		state.ID = sessionID
	}
	normalizeLoadedSession(state, filepath.Join("sessions", state.ID+".jsonl"))
	return state, currentSeq, nil
}

func applyEvent(current *Session, event SessionEvent) (*Session, error) {
	switch strings.TrimSpace(event.Type) {
	case eventTypeSessionCreated:
		payload := fullSessionPayload{}
		if err := decodePayload(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", eventTypeSessionCreated, err)
		}
		next, err := cloneSessionValue(&payload.Session)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(next.ID) == "" {
			next.ID = event.SessionID
		}
		next.UpdatedAt = pickLaterTime(next.UpdatedAt, event.TS)
		return next, nil

	case eventTypeModeSwitched:
		payload := modeSwitchedPayload{}
		if err := decodePayload(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", eventTypeModeSwitched, err)
		}
		next := ensureSessionState(current, event.SessionID, event.TS)
		if payload.Mode == "" {
			payload.Mode = planpkg.ModeBuild
		}
		next.Mode = payload.Mode
		next.UpdatedAt = pickLaterTime(next.UpdatedAt, event.TS)
		return next, nil

	case eventTypeTurnAppended:
		payload := turnAppendedPayload{}
		if err := decodePayload(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", eventTypeTurnAppended, err)
		}
		next := ensureSessionState(current, event.SessionID, event.TS)
		if payload.Replace {
			next.Messages = append([]llm.Message(nil), payload.Messages...)
		} else {
			next.Messages = append(next.Messages, payload.Messages...)
		}
		next.Conversation.Timeline = append([]llm.Message(nil), next.Messages...)
		next.UpdatedAt = pickLaterTime(next.UpdatedAt, event.TS)
		return next, nil

	case eventTypeSessionClosed:
		payload := sessionClosedPayload{}
		if err := decodePayload(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", eventTypeSessionClosed, err)
		}
		next := ensureSessionState(current, event.SessionID, event.TS)
		next.UpdatedAt = pickLaterTime(next.UpdatedAt, payload.ClosedAt)
		return next, nil

	case eventTypeSnapshotCompacted:
		payload := fullSessionPayload{}
		if err := decodePayload(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", eventTypeSnapshotCompacted, err)
		}
		next, err := cloneSessionValue(&payload.Session)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(next.ID) == "" {
			next.ID = event.SessionID
		}
		next.UpdatedAt = pickLaterTime(next.UpdatedAt, event.TS)
		return next, nil

	default:
		return current, nil
	}
}

func ensureSessionState(current *Session, sessionID string, at time.Time) *Session {
	if current == nil {
		now := at.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		return &Session{
			ID:        strings.TrimSpace(sessionID),
			CreatedAt: now,
			UpdatedAt: now,
			Conversation: Conversation{
				Timeline: make([]llm.Message, 0, 32),
			},
			Messages: make([]llm.Message, 0, 32),
			Mode:     planpkg.ModeBuild,
			Plan: planpkg.State{
				Phase: planpkg.PhaseNone,
				Steps: make([]planpkg.Step, 0, 8),
			},
		}
	}
	return current
}

func decodePayload(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func pickLaterTime(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.After(current) {
		return candidate.UTC()
	}
	return current.UTC()
}

func cloneSessionValue(source *Session) (*Session, error) {
	if source == nil {
		return nil, nil
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	var target Session
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func appendEventLine(files fileAppender, path string, event SessionEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return files.AppendLine(path, encoded)
}

type fileAppender interface {
	AppendLine(path string, line []byte) error
}

func readEventsFromFile(path string, afterSeq int64) ([]SessionEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events := make([]SessionEvent, 0, 32)
	reader := bufio.NewReader(file)
	lineNo := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return events, readErr
		}
		if errors.Is(readErr, io.EOF) && len(line) == 0 {
			break
		}
		lineNo++
		payload := bytes.TrimSpace(line)
		if len(payload) > 0 {
			var event SessionEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				log.Printf("session: skipped corrupted event line %d in %s: %v", lineNo, path, err)
			} else if afterSeq <= 0 || event.Seq > afterSeq {
				events = append(events, event)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return events, nil
}

func readEventsFromOffset(path string, offset int64, limit int) ([]SessionEvent, int64, error) {
	if offset < 0 {
		return nil, 0, errors.New("offset must be >= 0")
	}
	if limit <= 0 {
		limit = defaultReadFromLimit
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	fileSize := fileInfo.Size()
	if offset >= fileSize {
		return []SessionEvent{}, fileSize, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}

	events := make([]SessionEvent, 0, minInt(limit, 32))
	nextOffset := offset
	reader := bufio.NewReader(file)
	for len(events) < limit {
		lineStart := nextOffset
		line, readErr := reader.ReadBytes('\n')
		// Keep offset pinned on trailing partial line (EOF without newline)
		// so later append can make the record readable.
		if errors.Is(readErr, io.EOF) && len(line) > 0 && line[len(line)-1] != '\n' {
			nextOffset = lineStart
			break
		}
		if len(line) > 0 {
			nextOffset = lineStart + int64(len(line))
			payload := bytes.TrimSpace(line)
			if len(payload) > 0 {
				var event SessionEvent
				if err := json.Unmarshal(payload, &event); err != nil {
					log.Printf("session: skipped corrupted event line at offset %d in %s: %v", lineStart, path, err)
				} else {
					events = append(events, event)
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return events, nextOffset, readErr
		}
	}
	return events, nextOffset, nil
}

func readSnapshotFile(path string) (*sessionSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap sessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) lockSession(sessionID string) (func() error, error) {
	if s == nil {
		return nil, errors.New("session store is nil")
	}
	if s.locker == nil {
		return nil, errors.New("session locker is not configured")
	}

	ctx := context.Background()
	cancel := func() {}
	if s.lockTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, s.lockTimeout)
	}

	unlock, err := s.locker.LockSession(ctx, corepkg.SessionID(sessionID))
	cancel()
	if err != nil {
		return nil, err
	}
	return unlock, nil
}

func (s *Store) eventWindow(sessionID, eventsPath string) (*eventIDWindow, error) {
	s.mu.Lock()
	window := s.recentEventIDs[sessionID]
	if window == nil {
		window = newEventIDWindow(recentEventIDWindowSize)
		s.recentEventIDs[sessionID] = window
	}
	s.mu.Unlock()

	if len(window.order) > 0 {
		return window, nil
	}
	ids, err := tailEventIDs(eventsPath, recentEventIDWindowSize)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i := range ids {
		window.Add(ids[i])
	}
	return window, nil
}

func tailEventIDs(path string, limit int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(data, []byte("\n"))
	ids := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	for i := len(lines) - 1; i >= 0 && len(ids) < limit; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var event SessionEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		eventID := strings.TrimSpace(event.EventID)
		if eventID == "" {
			continue
		}
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}
		ids = append(ids, eventID)
	}

	for left, right := 0, len(ids)-1; left < right; left, right = left+1, right-1 {
		ids[left], ids[right] = ids[right], ids[left]
	}
	return ids, nil
}

func combineUnlockError(baseErr error, unlock func() error, sessionID string) error {
	if unlock == nil {
		return baseErr
	}
	unlockErr := unlock()
	if unlockErr == nil {
		return baseErr
	}
	if baseErr == nil {
		return fmt.Errorf("unlock session %q failed: %w", sessionID, unlockErr)
	}
	return errors.Join(baseErr, fmt.Errorf("unlock session %q failed: %w", sessionID, unlockErr))
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
