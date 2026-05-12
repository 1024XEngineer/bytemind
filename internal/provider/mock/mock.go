package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type StepType int

const (
	StepToolCall StepType = iota
	StepText
	// StepReadAndReplace reads a file and constructs replace_in_file args at runtime
	StepReadAndReplace
)

type ToolCallStep struct {
	Name      string
	Arguments string
}

type ReadAndReplaceStep struct {
	Path     string
	OldLine  string // exact line to find (must match end of the actual file line)
	NewLines string // replacement text
}

type Step struct {
	Type    StepType
	Tool    *ToolCallStep
	Text    string
	Replace *ReadAndReplaceStep
}

type MockProvider struct {
	steps     []Step
	stepIdx   int
	model     string
	workspace string // set at runtime by caller; used by dynamic steps
}

func New(model string) *MockProvider {
	return &MockProvider{
		steps:   selectPreset(model),
		stepIdx: 0,
		model:   model,
	}
}

func NewFromSteps(steps []Step) *MockProvider {
	return &MockProvider{
		steps:   steps,
		stepIdx: 0,
		model:   "custom",
	}
}

func (m *MockProvider) SetWorkspace(dir string) {
	m.workspace = dir
}

func (m *MockProvider) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	return m.nextResponse()
}

func (m *MockProvider) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	msg, err := m.nextResponse()
	if err == nil && onDelta != nil && msg.Content != "" {
		onDelta(msg.Content)
	}
	return msg, err
}

func (m *MockProvider) nextResponse() (llm.Message, error) {
	if m.stepIdx >= len(m.steps) {
		return llm.NewAssistantTextMessage("Task completed."), nil
	}

	step := m.steps[m.stepIdx]
	m.stepIdx++

	switch step.Type {
	case StepToolCall:
		if step.Tool == nil {
			return llm.NewAssistantTextMessage(""), nil
		}
		args := step.Tool.Arguments
		if !json.Valid([]byte(args)) {
			args = "{}"
		}
		return m.makeToolCall(step.Tool.Name, args), nil

	case StepReadAndReplace:
		if step.Replace == nil {
			return llm.NewAssistantTextMessage(""), nil
		}
		args := m.buildReplaceArgs(step.Replace)
		return m.makeToolCall("replace_in_file", args), nil

	default:
		return llm.NewAssistantTextMessage(step.Text), nil
	}
}

func (m *MockProvider) makeToolCall(name, args string) llm.Message {
	msg := llm.NewAssistantTextMessage("")
	msg.ToolCalls = []llm.ToolCall{{
		ID:   fmt.Sprintf("mock_%d", m.stepIdx),
		Type: "function",
		Function: llm.ToolFunctionCall{
			Name:      name,
			Arguments: args,
		},
	}}
	return msg
}

func (m *MockProvider) buildReplaceArgs(r *ReadAndReplaceStep) string {
	// Read the actual file content and build replace_in_file args dynamically
	fullPath := r.Path
	if m.workspace != "" && !filepath.IsAbs(r.Path) {
		fullPath = filepath.Join(m.workspace, r.Path)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		// Fallback: if file can't be read, return empty replace
		return fmt.Sprintf(`{"path":"%s","oldString":"","newString":""}`, r.Path)
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// Find the line containing OldLine
	oldLine := ""
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(trimmed) == strings.TrimSpace(r.OldLine) ||
			strings.Contains(trimmed, r.OldLine) {
			oldLine = line
			break
		}
	}
	if oldLine == "" {
		// Fallback: exact match not found, try partial
		for _, line := range lines {
			if strings.Contains(line, strings.TrimSpace(r.OldLine)) {
				oldLine = line
				break
			}
		}
	}
	if oldLine == "" {
		return fmt.Sprintf(`{"path":"%s","oldString":"","newString":""}`, r.Path)
	}

	oldJSON, _ := json.Marshal(oldLine)
	newJSON, _ := json.Marshal(r.NewLines)
	return fmt.Sprintf(`{"path":"%s","oldString":%s,"newString":%s}`, r.Path, string(oldJSON), string(newJSON))
}

func (m *MockProvider) Model() string {
	return m.model
}

func selectPreset(model string) []Step {
	switch strings.TrimSpace(strings.ToLower(model)) {
	case "bugfix-demo":
		return bugfixDemoPreset()
	case "text-only":
		return textOnlyPreset()
	default:
		env := os.Getenv("BYTEMIND_MOCK_STEPS")
		if env != "" {
			return parseEnvSteps(env)
		}
		return defaultPreset()
	}
}

func defaultPreset() []Step {
	return []Step{
		{Type: StepText, Text: "I have analyzed the project and completed the task. All tests pass."},
	}
}

func textOnlyPreset() []Step {
	return []Step{
		{Type: StepText, Text: "This is a mock text-only response for testing purposes."},
	}
}

func bugfixDemoPreset() []Step {
	return []Step{
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "list_files", Arguments: `{}`}},
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "read_file", Arguments: `{"path":"calculator.go"}`}},
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "read_file", Arguments: `{"path":"calculator_test.go"}`}},
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "run_tests", Arguments: `{}`}},
		{
			Type: StepReadAndReplace,
			Replace: &ReadAndReplaceStep{
				Path:    "calculator.go",
				OldLine: "return total / float64(len(nums))",
				NewLines: `if len(nums) == 0 {
		return 0
	}
	return total / float64(len(nums))`,
			},
		},
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "run_tests", Arguments: `{}`}},
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "git_diff", Arguments: `{}`}},
		{
			Type: StepText,
			Text: `**Summary**
- Fixed ` + "`CalculateAverage`" + ` divide-by-zero bug in empty-slice case.

**Changed Files**
- calculator.go

**Verification**
- go test ./...: passed

**Risks**
- No known remaining risk.

**Next Steps**
- None.`,
		},
	}
}

func parseEnvSteps(env string) []Step {
	var steps []Step
	parts := strings.Split(env, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "tool:") {
			rest := strings.TrimPrefix(p, "tool:")
			parts2 := strings.SplitN(rest, " ", 2)
			name := strings.TrimSpace(parts2[0])
			args := "{}"
			if len(parts2) > 1 {
				args = strings.TrimSpace(parts2[1])
			}
			steps = append(steps, Step{
				Type: StepToolCall,
				Tool: &ToolCallStep{Name: name, Arguments: args},
			})
		} else if strings.HasPrefix(p, "text:") {
			text := strings.TrimPrefix(p, "text:")
			steps = append(steps, Step{Type: StepText, Text: text})
		} else if strings.HasPrefix(p, "replace:") {
			rest := strings.TrimPrefix(p, "replace:")
			parts2 := strings.SplitN(rest, "|", 3)
			if len(parts2) == 3 {
				steps = append(steps, Step{
					Type: StepReadAndReplace,
					Replace: &ReadAndReplaceStep{
						Path:     strings.TrimSpace(parts2[0]),
						OldLine:  strings.TrimSpace(parts2[1]),
						NewLines: strings.TrimSpace(parts2[2]),
					},
				})
			}
		}
	}
	if len(steps) == 0 {
		return defaultPreset()
	}
	return steps
}
