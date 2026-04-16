package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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
	Title        string            `json:"title,omitempty"`
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
	files  *storagepkg.SessionFileStore
	locker storagepkg.Locker

	mu             sync.Mutex
	recentEventIDs map[string]*eventIDWindow

	now            func() time.Time
	newEventID     func() string
	lockTimeout    time.Duration
	snapshotEveryN int64
	snapshotEveryT time.Duration
}

type Summary struct {
	ID                            string    `json:"id"`
	Workspace                     string    `json:"workspace"`
	Title                         string    `json:"title,omitempty"`
	Preview                       string    `json:"preview,omitempty"`
	CreatedAt                     time.Time `json:"created_at"`
	UpdatedAt                     time.Time `json:"updated_at"`
	LastUserMessage               string    `json:"last_user_message,omitempty"`
	MessageCount                  int       `json:"message_count"`
	RawMessageCount               int       `json:"raw_msg_count"`
	UserEffectiveInputCount       int       `json:"user_effective_input_count"`
	AssistantEffectiveOutputCount int       `json:"assistant_effective_output_count"`
	ZeroMsgSession                bool      `json:"zero_msg_session"`
	NoReplySession                bool      `json:"no_reply_session"`
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
	locker, err := storagepkg.NewDefaultLocker(filepath.Join(dir, ".locks"))
	if err != nil {
		return nil, err
	}
	return &Store{
		files:          files,
		locker:         locker,
		recentEventIDs: make(map[string]*eventIDWindow),
		now: func() time.Time {
			return time.Now().UTC()
		},
		newEventID: func() string {
			var entropy [8]byte
			if _, err := rand.Read(entropy[:]); err != nil {
				return fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano())
			}
			return "evt-" + hex.EncodeToString(entropy[:])
		},
		lockTimeout:    5 * time.Second,
		snapshotEveryN: defaultSnapshotEveryN,
		snapshotEveryT: defaultSnapshotEveryT,
	}, nil
}

func (s *Store) Load(id string) (*Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return s.load(id)
}
