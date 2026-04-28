package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TaskReport struct {
	Executed                     []string `json:"executed,omitempty"`
	Denied                       []string `json:"denied,omitempty"`
	PendingApproval              []string `json:"pending_approval,omitempty"`
	SkippedDueToDeniedDependency []string `json:"skipped_due_to_denied_dependency,omitempty"`
	SkippedDueToDependency       []string `json:"skipped_due_to_dependency,omitempty"`
	SystemSandboxFallback        []string `json:"system_sandbox_fallback,omitempty"`
	StrategyAdjustments          []string `json:"strategy_adjustments,omitempty"`
	RetryReasons                 []string `json:"retry_reasons,omitempty"`
	NoProgressTurns              int      `json:"no_progress_turns,omitempty"`
	Escalations                  []string `json:"escalations,omitempty"`
}

func (r *TaskReport) RecordExecuted(name string) {
	r.appendUnique(&r.Executed, name)
}

func (r *TaskReport) RecordDenied(name string) {
	r.appendUnique(&r.Denied, name)
}

func (r *TaskReport) RecordPendingApproval(name string) {
	r.appendUnique(&r.PendingApproval, name)
}

func (r *TaskReport) RecordSkippedDueToDependency(name string) {
	r.RecordSkippedDueToDeniedDependency(name)
}

func (r *TaskReport) RecordSkippedDueToDeniedDependency(name string) {
	r.appendUnique(&r.SkippedDueToDeniedDependency, name)
	// Compatibility: keep legacy field mirrored for one release cycle.
	r.appendUnique(&r.SkippedDueToDependency, name)
}

func (r *TaskReport) RecordSystemSandboxFallback(note string) {
	r.appendUnique(&r.SystemSandboxFallback, note)
}

func (r *TaskReport) RecordStrategyAdjustment(note string) {
	r.appendUnique(&r.StrategyAdjustments, note)
}

func (r *TaskReport) RecordRetry(reason string) {
	r.appendUnique(&r.RetryReasons, reason)
}

func (r *TaskReport) RecordNoProgressTurn() {
	if r == nil {
		return
	}
	r.NoProgressTurns++
}

func (r *TaskReport) RecordEscalation(reason string) {
	r.appendUnique(&r.Escalations, reason)
}

func (r TaskReport) IsEmpty() bool {
	return len(r.Executed) == 0 &&
		len(r.Denied) == 0 &&
		len(r.PendingApproval) == 0 &&
		len(r.skipped()) == 0 &&
		len(r.SystemSandboxFallback) == 0 &&
		len(r.StrategyAdjustments) == 0 &&
		len(r.RetryReasons) == 0 &&
		r.NoProgressTurns == 0 &&
		len(r.Escalations) == 0
}

func (r TaskReport) HasNonSuccessOutcomes() bool {
	return len(r.Denied) > 0 ||
		len(r.PendingApproval) > 0 ||
		len(r.skipped()) > 0 ||
		len(r.SystemSandboxFallback) > 0 ||
		len(r.RetryReasons) > 0 ||
		r.NoProgressTurns > 0 ||
		len(r.Escalations) > 0
}

func (r TaskReport) JSON() string {
	payload, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func (r TaskReport) HumanSummary() string {
	lines := r.HumanSummaryLines()
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (r TaskReport) HumanSummaryLines() []string {
	lines := make([]string, 0, 8)
	appendLine := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", label, strings.Join(items, ", ")))
	}
	appendLine("Executed", r.Executed)
	appendLine("Denied", r.Denied)
	appendLine("Pending approval", r.PendingApproval)
	appendLine("Skipped due to denied dependency", r.skipped())
	appendLine("System sandbox fallback", r.SystemSandboxFallback)
	appendLine("Strategy adjustments", r.StrategyAdjustments)
	appendLine("Retry reasons", r.RetryReasons)
	if r.NoProgressTurns > 0 {
		lines = append(lines, fmt.Sprintf("- No progress turns: %d", r.NoProgressTurns))
	}
	appendLine("Escalations", r.Escalations)
	return lines
}

func (r *TaskReport) UnmarshalJSON(data []byte) error {
	type rawTaskReport struct {
		Executed                     []string `json:"executed,omitempty"`
		Denied                       []string `json:"denied,omitempty"`
		PendingApproval              []string `json:"pending_approval,omitempty"`
		SkippedDueToDeniedDependency []string `json:"skipped_due_to_denied_dependency,omitempty"`
		SkippedDueToDependency       []string `json:"skipped_due_to_dependency,omitempty"`
		SystemSandboxFallback        []string `json:"system_sandbox_fallback,omitempty"`
		StrategyAdjustments          []string `json:"strategy_adjustments,omitempty"`
		RetryReasons                 []string `json:"retry_reasons,omitempty"`
		NoProgressTurns              int      `json:"no_progress_turns,omitempty"`
		Escalations                  []string `json:"escalations,omitempty"`
	}
	var raw rawTaskReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Executed = append([]string(nil), raw.Executed...)
	r.Denied = append([]string(nil), raw.Denied...)
	r.PendingApproval = append([]string(nil), raw.PendingApproval...)
	skipped := raw.SkippedDueToDeniedDependency
	if len(skipped) == 0 {
		skipped = raw.SkippedDueToDependency
	}
	r.SkippedDueToDeniedDependency = append([]string(nil), skipped...)
	r.SkippedDueToDependency = append([]string(nil), skipped...)
	r.SystemSandboxFallback = append([]string(nil), raw.SystemSandboxFallback...)
	r.StrategyAdjustments = append([]string(nil), raw.StrategyAdjustments...)
	r.RetryReasons = append([]string(nil), raw.RetryReasons...)
	r.NoProgressTurns = raw.NoProgressTurns
	r.Escalations = append([]string(nil), raw.Escalations...)
	return nil
}

func (r TaskReport) skipped() []string {
	if len(r.SkippedDueToDeniedDependency) > 0 {
		return r.SkippedDueToDeniedDependency
	}
	return r.SkippedDueToDependency
}

func (r *TaskReport) appendUnique(target *[]string, name string) {
	if r == nil || target == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for _, existing := range *target {
		if existing == name {
			return
		}
	}
	*target = append(*target, name)
}
