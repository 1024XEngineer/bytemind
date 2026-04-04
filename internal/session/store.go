package session

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

const (
	snapshotEventType = "session_snapshot"
	schemaVersion     = 1
)

type ActiveSkill struct {
	Name        string            `json:"name"`
	Args        map[string]string `json:"args,omitempty"`
	ActivatedAt time.Time         `json:"activated_at,omitempty"`
}

type Session struct {
	ID           string            `json:"id"`
	Workspace    string            `json:"workspace"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Conversation Conversation      `json:"conversation,omitempty"`
	Messages     []llm.Message     `json:"messages,omitempty"`
	Mode         planpkg.AgentMode `json:"mode,omitempty"`
	Plan         planpkg.State     `json:"plan,omitempty"`
	ActiveSkill  *ActiveSkill      `json:"active_skill,omitempty"`
}

type Conversation struct {
	Meta     ConversationMeta   `json:"meta,omitempty"`
	Timeline []llm.Message      `json:"timeline"`
	Assets   ConversationAssets `json:"assets,omitempty"`
}

type ConversationMeta map[string]any

type ConversationAssets struct {
	Images map[llm.AssetID]ImageAssetMeta `json:"images,omitempty"`
}

type ImageAssetMeta struct {
	ImageID   int    `json:"image_id"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name,omitempty"`
	CachePath string `json:"cache_path"`
	ByteSize  int64  `json:"byte_size"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

type sessionRecord struct {
	Version   int       `json:"v"`
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	Payload   Session   `json:"payload"`
}

type Store struct {
	dir string
}

type Summary struct {
	ID              string    `json:"id"`
	Workspace       string    `json:"workspace"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastUserMessage string    `json:"last_user_message,omitempty"`
	MessageCount    int       `json:"message_count"`
}

func New(workspace string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        newID(),
		Workspace: workspace,
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

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Save(session *Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	now := time.Now().UTC()
	session.UpdatedAt = now
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.Mode == "" {
		session.Mode = planpkg.ModeBuild
	}
	normalizeSessionConversation(session)
	for i, message := range session.Conversation.Timeline {
		if err := llm.ValidateMessage(message); err != nil {
			return fmt.Errorf("timeline[%d] validation failed: %w", i, err)
		}
	}
	session.Plan = planpkg.NormalizeState(session.Plan)
	session.ActiveSkill = normalizeActiveSkill(session.ActiveSkill)
	if len(session.Plan.Steps) > 0 {
		session.Plan.UpdatedAt = session.UpdatedAt
	}

	target, err := s.pathForSession(session)
	if err != nil {
		return err
	}
	return writeSessionSnapshot(target, session)
}

func (s *Store) Load(id string) (*Session, error) {
	path, err := s.findSessionPath(id)
	if err != nil {
		return nil, err
	}
	return loadSessionFile(path)
}

func (s *Store) List(limit int) ([]Summary, []string, error) {
	paths, err := s.sessionPaths()
	if err != nil {
		return nil, nil, err
	}

	summaries := make([]Summary, 0, len(paths))
	warnings := make([]string, 0)
	for _, path := range paths {
		sess, err := loadSessionFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: %v", filepath.Base(path), err))
			continue
		}
		if strings.TrimSpace(sess.ID) == "" {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: missing session id", filepath.Base(path)))
			continue
		}

		summaries = append(summaries, Summary{
			ID:              sess.ID,
			Workspace:       sess.Workspace,
			CreatedAt:       sess.CreatedAt,
			UpdatedAt:       sess.UpdatedAt,
			LastUserMessage: summarizeMessage(lastUserMessage(sessionTimeline(sess)), 72),
			MessageCount:    len(sessionTimeline(sess)),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, warnings, nil
}

func (s *Store) pathForSession(session *Session) (string, error) {
	if strings.TrimSpace(session.ID) == "" {
		return "", errors.New("session id is required")
	}
	projectDir := filepath.Join(s.dir, projectID(session.Workspace))
	return filepath.Join(projectDir, session.ID+".jsonl"), nil
}

func (s *Store) findSessionPath(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("session id is required")
	}

	matches := make([]string, 0, 2)
	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == id+".jsonl" {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] < matches[j]
		}
		if leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return matches[i] < matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	return matches[0], nil
}

func (s *Store) sessionPaths() ([]string, error) {
	paths := make([]string, 0, 32)
	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".jsonl" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func loadSessionFile(path string) (*Session, error) {
	if strings.ToLower(filepath.Ext(path)) != ".jsonl" {
		return nil, errors.New("unsupported session file extension")
	}
	return loadJSONLSession(path)
}

func loadJSONLSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		var envelope struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(line, &envelope); err == nil {
			if envelope.Type == snapshotEventType && len(bytes.TrimSpace(envelope.Payload)) > 0 {
				var sess Session
				if err := json.Unmarshal(envelope.Payload, &sess); err == nil {
					normalizeLoadedSession(&sess, path)
					return &sess, nil
				}
			}
		}
	}
	return nil, errors.New("no valid session snapshot found")
}

func normalizeLoadedSession(sess *Session, path string) {
	if strings.TrimSpace(sess.ID) == "" {
		sess.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if sess.CreatedAt.IsZero() && !sess.UpdatedAt.IsZero() {
		sess.CreatedAt = sess.UpdatedAt
	}
	if sess.UpdatedAt.IsZero() && !sess.CreatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	normalizeSessionConversation(sess)
	if sess.Mode == "" {
		sess.Mode = planpkg.ModeBuild
	}
	sess.Plan = planpkg.NormalizeState(sess.Plan)
	sess.ActiveSkill = normalizeActiveSkill(sess.ActiveSkill)
	if len(sess.Plan.Steps) > 0 && sess.Plan.UpdatedAt.IsZero() {
		sess.Plan.UpdatedAt = sess.UpdatedAt
	}
}

func normalizeActiveSkill(raw *ActiveSkill) *ActiveSkill {
	if raw == nil {
		return nil
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return nil
	}

	args := make(map[string]string, len(raw.Args))
	for key, value := range raw.Args {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		args[key] = value
	}
	if len(args) == 0 {
		args = nil
	}

	return &ActiveSkill{
		Name:        name,
		Args:        args,
		ActivatedAt: raw.ActivatedAt,
	}
}

func writeSessionSnapshot(path string, session *Session) error {
	record := sessionRecord{
		Version:   schemaVersion,
		Timestamp: session.UpdatedAt,
		Type:      snapshotEventType,
		Payload:   *session,
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return err
	}
	content := bytes.TrimSpace(buf.Bytes())
	content = append(content, '\n')
	return writeAtomicFile(path, content)
}

func writeAtomicFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func projectID(workspace string) string {
	value := strings.TrimSpace(workspace)
	if value == "" {
		return "-unknown-project"
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	value = filepath.Clean(value)
	// Keep project-id normalization stable across CI/OS by lowercasing paths.
	value = strings.ToLower(value)
	value = filepath.ToSlash(value)

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}

	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "unknown-project"
	}
	if len(id) > 96 {
		sum := sha1.Sum([]byte(value))
		id = id[:80] + "-" + hex.EncodeToString(sum[:4])
	}
	return "-" + id
}

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102-150405")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			messages[i].Normalize()
			textParts := make([]string, 0, len(messages[i].Parts))
			for _, part := range messages[i].Parts {
				if part.Text != nil && strings.TrimSpace(part.Text.Value) != "" {
					textParts = append(textParts, part.Text.Value)
				}
			}
			if len(textParts) > 0 {
				return strings.TrimSpace(strings.Join(textParts, " "))
			}
		}
	}
	return ""
}

func sessionTimeline(sess *Session) []llm.Message {
	if sess == nil {
		return nil
	}
	if len(sess.Conversation.Timeline) > 0 {
		return sess.Conversation.Timeline
	}
	return sess.Messages
}

func normalizeSessionConversation(sess *Session) {
	if sess == nil {
		return
	}
	if len(sess.Messages) > 0 {
		sess.Conversation.Timeline = sess.Messages
	} else if len(sess.Conversation.Timeline) > 0 {
		sess.Messages = sess.Conversation.Timeline
	} else {
		sess.Messages = make([]llm.Message, 0, 32)
		sess.Conversation.Timeline = make([]llm.Message, 0, 32)
	}
	for i := range sess.Conversation.Timeline {
		sess.Conversation.Timeline[i].Normalize()
	}
	for i := range sess.Messages {
		sess.Messages[i].Normalize()
	}
}

func summarizeMessage(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
