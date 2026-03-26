package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

type Event struct {
	Type     string    `json:"type"`
	Target   string    `json:"target,omitempty"`
	Details  string    `json:"details,omitempty"`
	Approved bool      `json:"approved"`
	Success  bool      `json:"success"`
	Time     time.Time `json:"time"`
}

type Summary struct {
	ID        string
	UpdatedAt time.Time
	Messages  int
	Events    int
}

type Session struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Messages  []Message `json:"messages"`
	Events    []Event   `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(workspace string) *Session {
	now := time.Now()
	return &Session{
		ID:        generateID(),
		Workspace: workspace,
		Messages:  []Message{},
		Events:    []Event{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *Session) AddUserMessage(content string) {
	s.Messages = append(s.Messages, Message{
		Role:    "user",
		Content: content,
		Time:    time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func (s *Session) AddAssistantMessage(content string) {
	s.Messages = append(s.Messages, Message{
		Role:    "assistant",
		Content: content,
		Time:    time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func (s *Session) AddSystemMessage(content string) {
	s.Messages = append(s.Messages, Message{
		Role:    "system",
		Content: content,
		Time:    time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func (s *Session) AddEvent(eventType, target, details string, approved, success bool) {
	s.Events = append(s.Events, Event{
		Type:     eventType,
		Target:   target,
		Details:  details,
		Approved: approved,
		Success:  success,
		Time:     time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func (s *Session) GetMessages() []Message {
	return s.Messages
}

func (s *Session) GetEvents() []Event {
	return s.Events
}

func (s *Session) Save(dir string) error {
	sessionDir := filepath.Join(dir, ".forgecli", "sessions")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(sessionDir, s.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func (s *Session) Load(dir, sessionID string) error {
	data, err := os.ReadFile(sessionPath(dir, sessionID))
	if err != nil {
		return err
	}

	return json.Unmarshal(data, s)
}

func List(dir string) ([]Summary, error) {
	sessionDir := filepath.Join(dir, ".forgecli", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var summaries []Summary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
		if err != nil {
			continue
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}

		summaries = append(summaries, Summary{
			ID:        strings.TrimSuffix(entry.Name(), ".json"),
			UpdatedAt: sess.UpdatedAt,
			Messages:  len(sess.Messages),
			Events:    len(sess.Events),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	return summaries, nil
}

func LoadLatest(dir string) (*Session, error) {
	summaries, err := List(dir)
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("no saved sessions")
	}

	sess := &Session{}
	if err := sess.Load(dir, summaries[0].ID); err != nil {
		return nil, err
	}

	return sess, nil
}

func sessionPath(dir, sessionID string) string {
	return filepath.Join(dir, ".forgecli", "sessions", sessionID+".json")
}

func generateID() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:])[:16]
}
