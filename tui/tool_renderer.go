package tui

import (
	"strings"
	"sync"
)

// ToolRenderResult is the normalized output used by the TUI pipeline.
type ToolRenderResult struct {
	Summary     string
	DetailLines []string
	Status      string
	CompactLine string
}

// ToolRenderer describes how a tool's execution is displayed in the TUI.
// Each tool registers an implementation; the rendering pipeline calls Render
// once so payload parsing is not repeated for summary and compact fields.
type ToolRenderer interface {
	// DisplayLabel returns the short tag shown in the tool header (e.g. "READ", "SHELL").
	DisplayLabel() string

	// Render parses payload once and returns all fields needed by the TUI.
	Render(payload string) ToolRenderResult
}

var (
	rendererMu  sync.RWMutex
	rendererReg = make(map[string]ToolRenderer)
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

func (defaultRenderer) Render(payload string) ToolRenderResult {
	return ToolRenderResult{
		Summary:     compact(payload, 96),
		DetailLines: nil,
		Status:      "done",
		CompactLine: compact(payload, 80),
	}
}

func renderToolPayload(name, payload string) ToolRenderResult {
	renderer := GetToolRenderer(name)
	if renderer == nil {
		renderer = defaultRenderer{}
	}
	return normalizeToolRenderResult(renderer.Render(payload), payload)
}

func normalizeToolRenderResult(result ToolRenderResult, payload string) ToolRenderResult {
	if strings.TrimSpace(result.Status) == "" {
		result.Status = "done"
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = compact(payload, 96)
	}
	if strings.TrimSpace(result.CompactLine) == "" {
		result.CompactLine = compact(payload, 80)
	}
	return result
}
