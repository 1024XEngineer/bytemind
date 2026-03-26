package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

type Session struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(workspace string) *Session {
	return &Session{
		ID:        generateID(),
		Workspace: workspace,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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

func (s *Session) GetMessages() []Message {
	return s.Messages
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
	path := filepath.Join(dir, ".forgecli", "sessions", sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, s)
}

func (s *Session) List(dir string) ([]string, error) {
	sessionDir := filepath.Join(dir, ".forgecli", "sessions")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, e := range entries {
		if !e.IsDir() {
			sessions = append(sessions, e.Name())
		}
	}
	return sessions, nil
}

func generateID() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:])[:16]
}
