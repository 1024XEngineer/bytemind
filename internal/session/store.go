package session

import (
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	storagepkg "bytemind/internal/storage"
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

type Store struct {
	files *storagepkg.SessionFileStore
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
	files, err := storagepkg.NewSessionFileStore(dir)
	if err != nil {
		return nil, err
	}
	return &Store{files: files}, nil
}

func (s *Store) Load(id string) (*Session, error) {
	path, err := s.findSessionPath(id)
	if err != nil {
		return nil, err
	}
	return loadSessionFile(s.files, path)
}
