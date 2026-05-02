package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type pairWindow struct {
	ToolUseID       string
	ToolUseIndex    int
	ToolResultIndex int
}

type PairAwareCompactionConfig struct {
	Messages        []llm.Message
	LatestUserIndex int
	KeepPairCount   int
	SummaryBuilder  func(history []llm.Message) (llm.Message, error)
}

const maxPairAwareFallbackAttempts = 3

func BuildPairAwareCompactedMessages(cfg PairAwareCompactionConfig) ([]llm.Message, bool, error) {
	if cfg.SummaryBuilder == nil {
		return nil, false, fmt.Errorf("summary builder is required")
	}

	keepPairCount := cfg.KeepPairCount
	if keepPairCount < 0 {
		keepPairCount = 0
	}

	fallbackUsed := false
	keepCounts := make([]int, 0, maxPairAwareFallbackAttempts)
	for i := 0; i < maxPairAwareFallbackAttempts; i++ {
		pairCount := keepPairCount - i
		if pairCount < 0 {
			pairCount = 0
		}
		keepCounts = append(keepCounts, pairCount)
		if pairCount == 0 {
			break
		}
	}

	for attempt, pairCount := range keepCounts {
		history, preserved := splitPairAwareHistory(cfg.Messages, cfg.LatestUserIndex, pairCount)
		if len(history) == 0 {
			if attempt < len(keepCounts)-1 {
				fallbackUsed = true
				continue
			}
			return nil, fallbackUsed, fmt.Errorf("pair-aware compaction history is empty")
		}

		summaryMessage, err := cfg.SummaryBuilder(history)
		if err != nil {
			return nil, fallbackUsed, err
		}

		candidate := make([]llm.Message, 0, 1+len(preserved))
		candidate = append(candidate, summaryMessage)
		candidate = append(candidate, preserved...)
		if err := ValidateToolPairInvariant(candidate); err == nil {
			return candidate, fallbackUsed, nil
		} else if attempt < len(keepCounts)-1 {
			fallbackUsed = true
			continue
		} else {
			return nil, fallbackUsed, fmt.Errorf("pair-aware compaction failed invariant check: %w", err)
		}
	}

	return nil, fallbackUsed, fmt.Errorf("pair-aware compaction failed")
}

func collectRecentToolPairs(messages []llm.Message) []pairWindow {
	pending := make(map[string]int)
	pairs := make([]pairWindow, 0)
	for i := range messages {
		message := messages[i]
		message.Normalize()
		if message.Role == llm.RoleAssistant {
			for _, part := range message.Parts {
				if part.Type != llm.PartToolUse || part.ToolUse == nil {
					continue
				}
				toolUseID := strings.TrimSpace(part.ToolUse.ID)
				if toolUseID == "" {
					continue
				}
				if _, exists := pending[toolUseID]; exists {
					continue
				}
				pending[toolUseID] = i
			}
		}

		if message.Role == llm.RoleUser {
			for _, part := range message.Parts {
				if part.Type != llm.PartToolResult || part.ToolResult == nil {
					continue
				}
				toolUseID := strings.TrimSpace(part.ToolResult.ToolUseID)
				if toolUseID == "" {
					continue
				}
				start, ok := pending[toolUseID]
				if !ok {
					continue
				}
				pairs = append(pairs, pairWindow{
					ToolUseID:       toolUseID,
					ToolUseIndex:    start,
					ToolResultIndex: i,
				})
				delete(pending, toolUseID)
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].ToolResultIndex != pairs[j].ToolResultIndex {
			return pairs[i].ToolResultIndex < pairs[j].ToolResultIndex
		}
		return pairs[i].ToolUseIndex < pairs[j].ToolUseIndex
	})
	return pairs
}

func splitPairAwareHistory(messages []llm.Message, latestUserIndex, keepPairCount int) ([]llm.Message, []llm.Message) {
	keep := make([]bool, len(messages))
	if latestUserIndex >= 0 && latestUserIndex < len(messages) {
		keep[latestUserIndex] = true
	}

	boundary := len(messages)
	pairs := collectRecentToolPairs(messages)
	if keepPairCount > 0 && len(pairs) > 0 {
		if keepPairCount > len(pairs) {
			keepPairCount = len(pairs)
		}
		selected := pairs[len(pairs)-keepPairCount:]
		boundary = selected[0].ToolUseIndex
		boundary = moveBoundaryPastPairWindows(boundary, pairs, len(messages))
	}
	for i := boundary; i < len(messages); i++ {
		keep[i] = true
	}

	history := make([]llm.Message, 0, len(messages))
	preserved := make([]llm.Message, 0, len(messages))
	for i, message := range messages {
		if keep[i] {
			message.Normalize()
			preserved = append(preserved, message)
			continue
		}
		history = append(history, message)
	}
	return history, preserved
}

func moveBoundaryPastPairWindows(boundary int, windows []pairWindow, maxBoundary int) int {
	if boundary < 0 {
		boundary = 0
	}
	if boundary > maxBoundary {
		return maxBoundary
	}
	adjusted := boundary
	for {
		changed := false
		for _, window := range windows {
			if window.ToolUseIndex < adjusted && adjusted <= window.ToolResultIndex {
				adjusted = window.ToolResultIndex + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if adjusted > maxBoundary {
		return maxBoundary
	}
	return adjusted
}

func ValidateToolPairInvariant(messages []llm.Message) error {
	pending := make(map[string]int)
	for i := range messages {
		message := messages[i]
		message.Normalize()
		for _, part := range message.Parts {
			switch part.Type {
			case llm.PartToolUse:
				if part.ToolUse == nil {
					continue
				}
				toolUseID := strings.TrimSpace(part.ToolUse.ID)
				if toolUseID == "" {
					continue
				}
				pending[toolUseID]++
			case llm.PartToolResult:
				if part.ToolResult == nil {
					continue
				}
				toolUseID := strings.TrimSpace(part.ToolResult.ToolUseID)
				if toolUseID == "" {
					continue
				}
				if pending[toolUseID] == 0 {
					return fmt.Errorf("orphan tool_result for tool_use_id %q", toolUseID)
				}
				pending[toolUseID]--
				if pending[toolUseID] == 0 {
					delete(pending, toolUseID)
				}
			}
		}
	}

	if len(pending) == 0 {
		return nil
	}
	orphans := make([]string, 0, len(pending))
	for toolUseID := range pending {
		orphans = append(orphans, toolUseID)
	}
	sort.Strings(orphans)
	return fmt.Errorf("orphan tool_use without tool_result: %s", strings.Join(orphans, ", "))
}
