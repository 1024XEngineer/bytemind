package tui

import (
	"strings"

	"bytemind/internal/agent"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

type chatEntry struct {
	Kind   string
	Title  string
	Meta   string
	Body   string
	Status string
}

type viewportSelectionPoint struct {
	Col int
	Row int
}

type viewportTopLookupCache struct {
	left           int
	expectedTop    int
	viewportWidth  int
	viewportHeight int
	viewportOffset int
	top            int
	found          bool
	valid          bool
}

type commandItem struct {
	Name        string
	Usage       string
	Description string
	Group       string
	Kind        string
}

func (c commandItem) FilterValue() string {
	return strings.ToLower(strings.TrimPrefix(c.Usage, "/") + " " + c.Description)
}

type toolRun struct {
	Name    string
	Summary string
	Lines   []string
	Status  string
}

type approvalPrompt struct {
	Command string
	Reason  string
	Reply   chan approvalDecision
}

type approvalDecision struct {
	Approved bool
	Err      error
}

type agentEventMsg struct {
	Event agent.Event
}

type runFinishedMsg struct {
	RunID int
	Err   error
}

type runFinishReason string

const (
	runFinishReasonCompleted  runFinishReason = "completed"
	runFinishReasonFailed     runFinishReason = "failed"
	runFinishReasonCanceled   runFinishReason = "canceled"
	runFinishReasonBTWRestart runFinishReason = "btw_restart"
)

type approvalRequestMsg struct {
	Request tools.ApprovalRequest
	Reply   chan approvalDecision
}

type sessionsLoadedMsg struct {
	Summaries []session.Summary
	Err       error
}

type tokenUsagePulledMsg struct {
	Used    int
	Input   int
	Output  int
	Context int
	Err     error
}

type selectionToastExpiredMsg struct {
	ID int
}

type mouseSelectionScrollTickMsg struct {
	ID int
}
