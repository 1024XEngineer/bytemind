package runtime

import "bytemind/internal/history"

func (s *Service) LoadRecentPrompts(limit int) ([]history.PromptEntry, error) {
	return history.LoadRecentPrompts(limit)
}
