package tui

import (
	"fmt"
	"strconv"
	"strings"

	tuiruntime "bytemind/internal/tui/runtime"
)

func normalizeNewlines(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return input
}

func (m *model) ensurePastedContentState() {
	if m == nil || m.pastedStateLoaded {
		return
	}
	if m.pastedContents == nil {
		m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	}
	if m.pastedOrder == nil {
		m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	}
	if m.nextPasteID <= 0 {
		m.nextPasteID = 1
	}
	m.pastedStateLoaded = true

	state, err := m.runtimeAPI().LoadPastedContents(m.sess)
	if err != nil {
		return
	}
	if len(state.Contents) == 0 {
		return
	}

	m.pastedContents = make(map[string]pastedContent, len(state.Contents))
	for id, content := range state.Contents {
		id = strings.TrimSpace(id)
		if id == "" || strings.TrimSpace(content.Content) == "" {
			continue
		}
		content.ID = id
		m.pastedContents[id] = content
	}

	m.pastedOrder = append([]string(nil), state.Order...)
	m.nextPasteID = state.NextID
	if m.nextPasteID <= 0 {
		m.nextPasteID = 1
	}
	for _, id := range m.pastedOrder {
		if n, err := strconv.Atoi(id); err == nil && n >= m.nextPasteID {
			m.nextPasteID = n + 1
		}
	}
}

func (m *model) persistPastedContentState() error {
	if m == nil || m.sess == nil {
		return nil
	}
	return m.runtimeAPI().SavePastedContents(m.sess, tuiruntime.PastedState{
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: m.pastedContents,
	})
}

func (m *model) storePastedContent(content pastedContent) error {
	if m == nil {
		return nil
	}
	m.ensurePastedContentState()
	if strings.TrimSpace(content.ID) == "" {
		return fmt.Errorf("invalid pasted content id")
	}
	if len(content.Content) > maxSinglePastedCharLength {
		return fmt.Errorf("pasted content too large (%d chars), please attach a file instead", len(content.Content))
	}
	if _, exists := m.pastedContents[content.ID]; exists {
		m.pastedContents[content.ID] = content
		return m.persistPastedContentState()
	}
	m.pastedContents[content.ID] = content
	m.pastedOrder = append(m.pastedOrder, content.ID)
	if len(m.pastedOrder) > maxStoredPastedContents {
		drop := len(m.pastedOrder) - maxStoredPastedContents
		for i := 0; i < drop; i++ {
			oldest := m.pastedOrder[i]
			delete(m.pastedContents, oldest)
		}
		m.pastedOrder = append([]string(nil), m.pastedOrder[drop:]...)
	}
	return m.persistPastedContentState()
}

func (m *model) resolvePastedLineReference(input string) (string, error) {
	if m == nil {
		return input, nil
	}
	m.ensurePastedContentState()
	return tuiruntime.ResolvePastedLineReference(input, tuiruntime.PastedState{
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: m.pastedContents,
	})
}

func (m *model) resolvePastedSelection(pasteID, startLineStr, endLineStr string) (string, error) {
	return tuiruntime.ResolvePastedSelection(tuiruntime.PastedState{
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: m.pastedContents,
	}, pasteID, startLineStr, endLineStr)
}

func (m *model) findPastedContent(pasteID string) (pastedContent, bool) {
	m.ensurePastedContentState()
	return tuiruntime.FindPastedContent(tuiruntime.PastedState{
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: m.pastedContents,
	}, pasteID)
}

func extractLineRange(content string, startLine, endLine int) string {
	return tuiruntime.ExtractLineRange(content, startLine, endLine)
}
