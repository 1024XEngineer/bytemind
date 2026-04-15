package plan

import (
	"fmt"
	"strings"
	"time"

	corepkg "bytemind/internal/core"
)

type AgentMode string

const (
	ModeBuild AgentMode = "build"
	ModePlan  AgentMode = "plan"
)

type Phase string

const (
	PhaseNone      Phase = "none"
	PhaseDrafting  Phase = "drafting"
	PhaseReady     Phase = "ready"
	PhaseApproved  Phase = "approved"
	PhaseExecuting Phase = "executing"
	PhaseBlocked   Phase = "blocked"
	PhaseCompleted Phase = "completed"
)

type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepCompleted  StepStatus = "completed"
	StepBlocked    StepStatus = "blocked"
)

type RiskLevel = corepkg.RiskLevel

const (
	RiskLow    RiskLevel = corepkg.RiskLow
	RiskMedium RiskLevel = corepkg.RiskMedium
	RiskHigh   RiskLevel = corepkg.RiskHigh
)

type Step struct {
	ID          string     `json:"id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      StepStatus `json:"status"`
	Files       []string   `json:"files,omitempty"`
	Verify      []string   `json:"verify,omitempty"`
	Risk        RiskLevel  `json:"risk,omitempty"`
}

type State struct {
	Goal        string    `json:"goal,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Phase       Phase     `json:"phase,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Steps       []Step    `json:"steps,omitempty"`
	NextAction  string    `json:"next_action,omitempty"`
	BlockReason string    `json:"block_reason,omitempty"`
}

func NormalizeMode(raw string) AgentMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModePlan):
		return ModePlan
	default:
		return ModeBuild
	}
}

func NormalizePhase(raw string) Phase {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PhaseDrafting):
		return PhaseDrafting
	case string(PhaseReady):
		return PhaseReady
	case string(PhaseApproved):
		return PhaseApproved
	case string(PhaseExecuting):
		return PhaseExecuting
	case string(PhaseBlocked):
		return PhaseBlocked
	case string(PhaseCompleted):
		return PhaseCompleted
	default:
		return PhaseNone
	}
}

func NormalizeStepStatus(raw string) StepStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(StepInProgress):
		return StepInProgress
	case string(StepCompleted):
		return StepCompleted
	case string(StepBlocked):
		return StepBlocked
	default:
		return StepPending
	}
}

func NormalizeRisk(raw string) RiskLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(RiskMedium):
		return RiskMedium
	case string(RiskHigh):
		return RiskHigh
	default:
		return RiskLow
	}
}

func CloneSteps(steps []Step) []Step {
	if len(steps) == 0 {
		return nil
	}
	cloned := make([]Step, len(steps))
	for i, step := range steps {
		cloned[i] = step
		cloned[i].Files = append([]string(nil), step.Files...)
		cloned[i].Verify = append([]string(nil), step.Verify...)
	}
	return cloned
}

func CloneState(state State) State {
	cloned := state
	cloned.Steps = CloneSteps(state.Steps)
	return cloned
}

func HasStructuredPlan(state State) bool {
	return len(state.Steps) > 0
}

func CurrentStep(state State) (Step, bool) {
	for _, step := range state.Steps {
		if NormalizeStepStatus(string(step.Status)) == StepInProgress || NormalizeStepStatus(string(step.Status)) == StepBlocked {
			return step, true
		}
	}
	return Step{}, false
}

func CountByStatus(state State, status StepStatus) int {
	count := 0
	for _, step := range state.Steps {
		if NormalizeStepStatus(string(step.Status)) == status {
			count++
		}
	}
	return count
}

func DefaultNextAction(state State) string {
	if step, ok := CurrentStep(state); ok && strings.TrimSpace(step.Title) != "" {
		return "Continue: " + strings.TrimSpace(step.Title)
	}
	for _, step := range state.Steps {
		if NormalizeStepStatus(string(step.Status)) == StepPending && strings.TrimSpace(step.Title) != "" {
			return "Start: " + strings.TrimSpace(step.Title)
		}
	}
	return ""
}

func DerivePhase(mode AgentMode, steps []Step, blockReason string) Phase {
	if len(steps) == 0 {
		if mode == ModePlan {
			return PhaseDrafting
		}
		return PhaseNone
	}

	if strings.TrimSpace(blockReason) != "" || hasStepStatus(steps, StepBlocked) {
		return PhaseBlocked
	}
	if allStepsCompleted(steps) {
		return PhaseCompleted
	}
	if mode == ModePlan {
		return PhaseReady
	}
	if hasStepStatus(steps, StepInProgress) {
		return PhaseExecuting
	}
	return PhaseApproved
}

func NormalizeState(state State) State {
	state.Phase = NormalizePhase(string(state.Phase))
	state.Goal = strings.TrimSpace(state.Goal)
	state.Summary = strings.TrimSpace(state.Summary)
	state.NextAction = strings.TrimSpace(state.NextAction)
	state.BlockReason = strings.TrimSpace(state.BlockReason)
	if len(state.Steps) == 0 {
		state.Steps = nil
		if state.Phase == PhaseNone && strings.TrimSpace(state.Goal) == "" && strings.TrimSpace(state.Summary) == "" {
			return state
		}
		return state
	}

	normalized := make([]Step, 0, len(state.Steps))
	for i, raw := range state.Steps {
		title := strings.TrimSpace(raw.Title)
		if title == "" {
			continue
		}
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			id = fmt.Sprintf("s%d", i+1)
		}
		step := Step{
			ID:          id,
			Title:       title,
			Description: strings.TrimSpace(raw.Description),
			Status:      NormalizeStepStatus(string(raw.Status)),
			Files:       trimStrings(raw.Files),
			Verify:      trimStrings(raw.Verify),
			Risk:        normalizeOptionalRisk(raw.Risk),
		}
		normalized = append(normalized, step)
	}
	state.Steps = normalized
	if strings.TrimSpace(state.NextAction) == "" {
		state.NextAction = DefaultNextAction(state)
	}
	if state.Phase == PhaseNone {
		state.Phase = DerivePhase(ModeBuild, state.Steps, state.BlockReason)
	}
	return state
}

func trimStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOptionalRisk(raw RiskLevel) RiskLevel {
	switch NormalizeRisk(string(raw)) {
	case RiskHigh:
		return RiskHigh
	case RiskMedium:
		return RiskMedium
	default:
		if strings.TrimSpace(string(raw)) == "" {
			return ""
		}
		return RiskLow
	}
}

func hasStepStatus(steps []Step, want StepStatus) bool {
	for _, step := range steps {
		if NormalizeStepStatus(string(step.Status)) == want {
			return true
		}
	}
	return false
}

func allStepsCompleted(steps []Step) bool {
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if NormalizeStepStatus(string(step.Status)) != StepCompleted {
			return false
		}
	}
	return true
}
