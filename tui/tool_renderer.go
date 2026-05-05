package tui

import "sync"

// ToolRenderer describes how a tool's execution is displayed in the TUI.
// Each tool registers an implementation; the rendering pipeline calls these
// methods instead of the old centralized summarizeTool switch-case.
type ToolRenderer interface {
	// DisplayLabel returns the short tag shown in the tool header (e.g. "READ", "SHELL").
	DisplayLabel() string

	// ResultSummary parses the tool's JSON payload and returns:
	//   summary  – one-line description for the collapsed tree node
	//   lines    – detail lines shown when expanded
	//   status   – "done", "warn", "error", etc.
	ResultSummary(payload string) (summary string, lines []string, status string)

	// CompactLine returns a single-line representation for the collapsed tree view.
	// For example: "model.go (1-50)" for read_file, "3 matches for auth" for search_text.
	CompactLine(payload string) string
}

var (
	rendererMu   sync.RWMutex
	rendererReg  = make(map[string]ToolRenderer)
)

// RegisterToolRenderer registers a ToolRenderer for the given tool name.
func RegisterToolRenderer(name string, r ToolRenderer) {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	rendererReg[name] = r
}

// GetToolRenderer returns the registered renderer for the tool, or nil if none.
func GetToolRenderer(name string) ToolRenderer {
	rendererMu.RLock()
	defer rendererMu.RUnlock()
	return rendererReg[name]
}

// defaultRenderer is the fallback when no tool-specific renderer is registered.
type defaultRenderer struct{}

func (defaultRenderer) DisplayLabel() string { return "TOOL" }

func (defaultRenderer) ResultSummary(payload string) (string, []string, string) {
	return compact(payload, 96), nil, "done"
}

func (defaultRenderer) CompactLine(payload string) string {
	return compact(payload, 80)
}
