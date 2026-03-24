package session

import (
	"time"

	"forgecli/internal/executor"
	"forgecli/internal/planner"
)

type Session struct {
	Task           string
	RepoRoot       string
	VerifyCommand  string
	StartedAt      time.Time
	Plan           planner.Plan
	TargetFile     string
	ChangedFiles   []string
	WriteApproved  bool
	FileWritten    bool
	VerifyApproved bool
	VerifyResult   *executor.Result
	Notes          []string
}

func New(task, repoRoot, verifyCommand string) *Session {
	return &Session{
		Task:          task,
		RepoRoot:      repoRoot,
		VerifyCommand: verifyCommand,
		StartedAt:     time.Now(),
	}
}

func (s *Session) AddNote(note string) {
	if note == "" {
		return
	}
	s.Notes = append(s.Notes, note)
}
