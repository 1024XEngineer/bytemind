package session

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bytemind/internal/llm"
)

type PlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

type Session struct {
	ID        string        `json:"id"`
	Workspace string        `json:"workspace"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Plan      []PlanItem    `json:"plan,omitempty"`
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
		Messages:  make([]llm.Message, 0, 32),
		Plan:      make([]PlanItem, 0, 8),
	}
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Save(session *Session) error {
	session.UpdatedAt = time.Now().UTC()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(session); err != nil {
		return err
	}

	target := filepath.Join(s.dir, session.ID+".json")
	tmp, err := os.CreateTemp(s.dir, session.ID+".*.tmp")
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
	if _, err := tmp.Write(bytes.TrimRight(buf.Bytes(), "\n")); err != nil {
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
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *Store) Load(id string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) List(limit int) ([]Summary, []string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, nil, err
	}

	summaries := make([]Summary, 0, len(entries))
	warnings := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, nil, err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: empty file", entry.Name()))
			continue
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: invalid JSON (%v)", entry.Name(), err))
			continue
		}

		summaries = append(summaries, Summary{
			ID:              sess.ID,
			Workspace:       sess.Workspace,
			CreatedAt:       sess.CreatedAt,
			UpdatedAt:       sess.UpdatedAt,
			LastUserMessage: summarizeMessage(lastUserMessage(sess.Messages), 72),
			MessageCount:    len(sess.Messages),
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

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102-150405")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
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
