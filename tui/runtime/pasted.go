package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/session"
)

const pastedContentMetaKey = "pasted_contents"

var pastedRefPattern = regexp.MustCompile(`\[(?:Paste|Pasted)(?:\s+#(\d+))?(?:\s+~(\d+)\s+lines)?(?:\s+line(\d+)(?:~line(\d+))?)?\]`)

const (
	pasteRefGroupID        = 2
	pasteRefGroupLineCount = 4
	pasteRefGroupLineStart = 6
	pasteRefGroupLineEnd   = 8
)

type PastedContent struct {
	ID      string    `json:"id"`
	Content string    `json:"content"`
	Lines   int       `json:"lines"`
	Time    time.Time `json:"time"`
	Preview string    `json:"preview"`
}

type PastedState struct {
	NextID   int
	Order    []string
	Contents map[string]PastedContent
}

type persistedPastedContents struct {
	Version  int                      `json:"version"`
	NextID   int                      `json:"next_id"`
	Order    []string                 `json:"order"`
	Contents map[string]PastedContent `json:"contents"`
}

func (s *Service) LoadPastedContents(sess *session.Session) (PastedState, error) {
	state := PastedState{
		NextID:   1,
		Order:    make([]string, 0),
		Contents: make(map[string]PastedContent),
	}
	if sess == nil || sess.Conversation.Meta == nil {
		return state, nil
	}
	raw, ok := sess.Conversation.Meta[pastedContentMetaKey]
	if !ok || raw == nil {
		return state, nil
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return state, nil
	}
	var persisted persistedPastedContents
	if err := json.Unmarshal(blob, &persisted); err != nil {
		return state, nil
	}
	for id, content := range persisted.Contents {
		id = strings.TrimSpace(id)
		if id == "" || strings.TrimSpace(content.Content) == "" {
			continue
		}
		content.ID = id
		state.Contents[id] = content
	}
	seen := make(map[string]struct{}, len(persisted.Order))
	for _, id := range persisted.Order {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := state.Contents[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		state.Order = append(state.Order, id)
	}
	if len(state.Order) < len(state.Contents) {
		missing := make([]string, 0, len(state.Contents)-len(state.Order))
		for id := range state.Contents {
			if _, ok := seen[id]; ok {
				continue
			}
			missing = append(missing, id)
		}
		sort.Strings(missing)
		state.Order = append(state.Order, missing...)
	}
	state.NextID = persisted.NextID
	if state.NextID <= 0 {
		state.NextID = 1
	}
	for _, id := range state.Order {
		if n, err := strconv.Atoi(id); err == nil && n >= state.NextID {
			state.NextID = n + 1
		}
	}
	return state, nil
}

func (s *Service) SavePastedContents(sess *session.Session, state PastedState) error {
	if sess == nil {
		return nil
	}
	if sess.Conversation.Meta == nil {
		sess.Conversation.Meta = make(map[string]any, 4)
	}
	payload := persistedPastedContents{
		Version:  1,
		NextID:   state.NextID,
		Order:    append([]string(nil), state.Order...),
		Contents: state.Contents,
	}
	sess.Conversation.Meta[pastedContentMetaKey] = payload
	return s.SaveSession(sess)
}

func ResolvePastedLineReference(input string, state PastedState) (string, error) {
	if !strings.Contains(input, "[Paste") && !strings.Contains(input, "[Pasted") {
		return input, nil
	}
	if len(state.Order) == 0 {
		return input, nil
	}

	matches := pastedRefPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	var out strings.Builder
	last := 0
	for _, idx := range matches {
		start, end := idx[0], idx[1]
		out.WriteString(input[last:start])

		full := input[start:end]
		pasteID := submatchString(input, idx, pasteRefGroupID)
		lineCount := submatchString(input, idx, pasteRefGroupLineCount)
		startLineStr := submatchString(input, idx, pasteRefGroupLineStart)
		endLineStr := submatchString(input, idx, pasteRefGroupLineEnd)

		if strings.TrimSpace(startLineStr) == "" && strings.TrimSpace(lineCount) != "" {
			content, ok := FindPastedContent(state, pasteID)
			if !ok {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content.Content)
				out.WriteString("\n```")
			}
		} else if strings.TrimSpace(startLineStr) == "" {
			content, ok := FindPastedContent(state, pasteID)
			if !ok {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content.Content)
				out.WriteString("\n```")
			}
		} else {
			content, err := ResolvePastedSelection(state, pasteID, startLineStr, endLineStr)
			if err != nil {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content)
				out.WriteString("\n```")
			}
		}
		last = end
	}
	out.WriteString(input[last:])
	return out.String(), nil
}

func ResolvePastedSelection(state PastedState, pasteID, startLineStr, endLineStr string) (string, error) {
	content, ok := FindPastedContent(state, pasteID)
	if !ok {
		return "", fmt.Errorf("pasted reference not found")
	}
	if strings.TrimSpace(startLineStr) == "" {
		return content.Content, nil
	}
	startLine, err := strconv.Atoi(startLineStr)
	if err != nil {
		return "", fmt.Errorf("invalid start line")
	}
	endLine := startLine
	if strings.TrimSpace(endLineStr) != "" {
		if v, err := strconv.Atoi(endLineStr); err == nil {
			endLine = v
		}
	}
	return ExtractLineRange(content.Content, startLine, endLine), nil
}

func FindPastedContent(state PastedState, pasteID string) (PastedContent, bool) {
	if strings.TrimSpace(pasteID) == "" {
		if len(state.Order) == 0 {
			return PastedContent{}, false
		}
		latestID := state.Order[len(state.Order)-1]
		content, ok := state.Contents[latestID]
		return content, ok
	}
	content, ok := state.Contents[strings.TrimSpace(pasteID)]
	return content, ok
}

func ExtractLineRange(content string, startLine, endLine int) string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 {
		return ""
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 {
		endLine = startLine
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if endLine < startLine {
		endLine = startLine
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

func submatchString(input string, indexes []int, groupOffset int) string {
	if len(indexes) <= groupOffset+1 {
		return ""
	}
	start, end := indexes[groupOffset], indexes[groupOffset+1]
	if start < 0 || end < 0 || start > end || end > len(input) {
		return ""
	}
	return input[start:end]
}
