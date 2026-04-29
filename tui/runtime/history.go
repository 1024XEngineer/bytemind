package runtime

import "github.com/1024XEngineer/bytemind/internal/history"

func (s *Service) LoadRecentPrompts(limit int) ([]history.PromptEntry, error) {
	return history.LoadRecentPrompts(limit)
}
