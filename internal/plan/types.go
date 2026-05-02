package plan

import (
	"fmt"
	"strings"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type AgentMode = corepkg.SessionMode

const (
	ModeBuild AgentMode = corepkg.SessionModeBuild
	ModePlan  AgentMode = corepkg.SessionModePlan
)

type Phase string

const (
	PhaseNone            Phase = "none"
	PhaseExplore         Phase = "explore"
	PhaseClarify         Phase = "clarify"
	PhaseDraft           Phase = "draft"
	PhaseConvergeReady   Phase = "converge_ready"
	PhaseApprovedToBuild Phase = "approved_to_build"
	PhaseExecuting       Phase = "executing"
	PhaseBlocked         Phase = "blocked"
	PhaseCompleted       Phase = "completed"

	// Legacy aliases kept for persisted sessions and older callers.
	PhaseDrafting = PhaseDraft
	PhaseReady    = PhaseConvergeReady
	PhaseApproved = PhaseApprovedToBuild
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

type Decision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type ChoiceOption struct {
	ID          string `json:"id,omitempty"`
	Shortcut    string `json:"shortcut,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Freeform    bool   `json:"freeform,omitempty"`
}

type ActiveChoice struct {
	ID       string         `json:"id,omitempty"`
	Kind     string         `json:"kind,omitempty"`
	Question string         `json:"question"`
	GapKey   string         `json:"gap_key,omitempty"`
	Options  []ChoiceOption `json:"options,omitempty"`
}

type State struct {
	Goal                string        `json:"goal,omitempty"`
	Summary             string        `json:"summary,omitempty"`
	ImplementationBrief string        `json:"implementation_brief,omitempty"`
	Phase               Phase         `json:"phase,omitempty"`
	UpdatedAt           time.Time     `json:"updated_at,omitempty"`
	Steps               []Step        `json:"steps,omitempty"`
	Risks               []string      `json:"risks,omitempty"`
	Verification        []string      `json:"verification,omitempty"`
	DecisionLog         []Decision    `json:"decision_log,omitempty"`
	DecisionGaps        []string      `json:"decision_gaps,omitempty"`
	ActiveChoice        *ActiveChoice `json:"active_choice,omitempty"`
	ScopeDefined        bool          `json:"scope_defined,omitempty"`
	RiskRollbackDefined bool          `json:"risk_and_rollback_defined,omitempty"`
	VerificationDefined bool          `json:"verification_defined,omitempty"`
	NextAction          string        `json:"next_action,omitempty"`
	BlockReason         string        `json:"block_reason,omitempty"`
}

func NormalizeMode(raw string) AgentMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModePlan):
		return ModePlan
	case "default", "acceptedits", "bypasspermissions":
		return ModeBuild
	default:
		return ModeBuild
	}
}

func NormalizePhase(raw string) Phase {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PhaseExplore):
		return PhaseExplore
	case string(PhaseClarify):
		return PhaseClarify
	case string(PhaseDraft), "drafting":
		return PhaseDraft
	case string(PhaseConvergeReady), "ready":
		return PhaseConvergeReady
	case string(PhaseApprovedToBuild), "approved":
		return PhaseApprovedToBuild
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

func CloneDecisionLog(entries []Decision) []Decision {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]Decision, len(entries))
	copy(cloned, entries)
	return cloned
}

func CloneActiveChoice(choice *ActiveChoice) *ActiveChoice {
	if choice == nil {
		return nil
	}
	cloned := *choice
	if len(choice.Options) > 0 {
		cloned.Options = make([]ChoiceOption, len(choice.Options))
		copy(cloned.Options, choice.Options)
	}
	return &cloned
}

func CloneState(state State) State {
	cloned := state
	cloned.Steps = CloneSteps(state.Steps)
	cloned.Risks = append([]string(nil), state.Risks...)
	cloned.Verification = append([]string(nil), state.Verification...)
	cloned.DecisionLog = CloneDecisionLog(state.DecisionLog)
	cloned.DecisionGaps = append([]string(nil), state.DecisionGaps...)
	cloned.ActiveChoice = CloneActiveChoice(state.ActiveChoice)
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

func HasDecisionGaps(state State) bool {
	return len(state.DecisionGaps) > 0
}

func HasActiveChoice(state State) bool {
	state = NormalizeState(state)
	return state.ActiveChoice != nil && len(state.ActiveChoice.Options) > 0 && strings.TrimSpace(state.ActiveChoice.Question) != ""
}

func HasExecutionReadiness(state State) bool {
	return state.ScopeDefined && state.RiskRollbackDefined && state.VerificationDefined
}

func CanStartExecution(state State) bool {
	state = NormalizeState(state)
	if !HasStructuredPlan(state) || HasDecisionGaps(state) {
		return false
	}
	switch NormalizePhase(string(state.Phase)) {
	case PhaseConvergeReady, PhaseApprovedToBuild:
		return true
	default:
		return false
	}
}

func DefaultNextAction(state State) string {
	switch NormalizePhase(string(state.Phase)) {
	case PhaseExplore:
		return "Inspect the relevant code and sketch the first plan draft."
	case PhaseClarify:
		return "Ask the next 1 to 2 decision questions and update the plan."
	case PhaseDraft:
		return "Refine the plan and close the remaining decision gaps."
	case PhaseConvergeReady:
		return "Ask whether to start execution or keep adjusting the plan."
	case PhaseApprovedToBuild:
		return "Switch to Build mode and execute the first planned step."
	case PhaseBlocked, PhaseCompleted:
		return ""
	}
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

func DerivePhase(mode AgentMode, state State) Phase {
	if len(state.Steps) == 0 {
		if mode == ModePlan {
			return PhaseExplore
		}
		return PhaseNone
	}

	if strings.TrimSpace(state.BlockReason) != "" || hasStepStatus(state.Steps, StepBlocked) {
		return PhaseBlocked
	}
	if allStepsCompleted(state.Steps) {
		return PhaseCompleted
	}
	if mode == ModePlan {
		if HasDecisionGaps(state) {
			return PhaseClarify
		}
		if HasExecutionReadiness(state) {
			return PhaseConvergeReady
		}
		return PhaseDraft
	}
	if hasStepStatus(state.Steps, StepInProgress) {
		return PhaseExecuting
	}
	if NormalizePhase(string(state.Phase)) == PhaseApprovedToBuild {
		return PhaseApprovedToBuild
	}
	return PhaseExecuting
}

func NormalizeState(state State) State {
	state.Phase = NormalizePhase(string(state.Phase))
	state.Goal = strings.TrimSpace(state.Goal)
	state.Summary = strings.TrimSpace(state.Summary)
	state.ImplementationBrief = strings.TrimSpace(state.ImplementationBrief)
	state.NextAction = strings.TrimSpace(state.NextAction)
	state.BlockReason = strings.TrimSpace(state.BlockReason)
	state.Risks = trimStrings(state.Risks)
	state.Verification = trimStrings(state.Verification)
	state.DecisionLog = normalizeDecisionLog(state.DecisionLog)
	state.DecisionGaps = trimStrings(state.DecisionGaps)
	state.ActiveChoice = normalizeActiveChoice(state.ActiveChoice)
	state = reconcileClarifyState(state)
	if len(state.Steps) == 0 {
		state.Steps = nil
		if len(state.DecisionGaps) == 0 || state.Phase == PhaseConvergeReady || state.Phase == PhaseApprovedToBuild {
			state.ActiveChoice = nil
		}
		if state.Phase == PhaseNone &&
			strings.TrimSpace(state.Goal) == "" &&
			strings.TrimSpace(state.Summary) == "" &&
			strings.TrimSpace(state.ImplementationBrief) == "" &&
			len(state.DecisionLog) == 0 &&
			len(state.DecisionGaps) == 0 &&
			state.ActiveChoice == nil &&
			len(state.Risks) == 0 &&
			len(state.Verification) == 0 {
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
		state.Phase = DerivePhase(ModeBuild, state)
	}
	if len(state.DecisionGaps) == 0 || state.Phase == PhaseConvergeReady || state.Phase == PhaseApprovedToBuild {
		state.ActiveChoice = nil
	}
	return state
}

func reconcileClarifyState(state State) State {
	if state.ActiveChoice == nil || !activeChoiceKeepsClarifyPhase(state.ActiveChoice) {
		return state
	}
	if len(state.DecisionGaps) == 0 {
		if gap := activeChoiceDecisionGap(state.ActiveChoice); gap != "" {
			state.DecisionGaps = []string{gap}
		}
	}
	switch state.Phase {
	case PhaseNone, PhaseConvergeReady, PhaseApprovedToBuild:
		state.Phase = PhaseClarify
	}
	return state
}

func activeChoiceKeepsClarifyPhase(choice *ActiveChoice) bool {
	if choice == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(choice.Kind))
	return kind == "" || kind == "clarify"
}

func activeChoiceDecisionGap(choice *ActiveChoice) string {
	if choice == nil {
		return ""
	}
	if gap := strings.TrimSpace(choice.GapKey); gap != "" {
		return gap
	}
	return strings.TrimSpace(choice.Question)
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

func normalizeDecisionLog(entries []Decision) []Decision {
	if len(entries) == 0 {
		return nil
	}
	out := make([]Decision, 0, len(entries))
	for _, entry := range entries {
		decision := strings.TrimSpace(entry.Decision)
		reason := strings.TrimSpace(entry.Reason)
		if decision == "" {
			continue
		}
		out = append(out, Decision{
			Decision: decision,
			Reason:   reason,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeActiveChoice(choice *ActiveChoice) *ActiveChoice {
	if choice == nil {
		return nil
	}
	normalized := &ActiveChoice{
		ID:       strings.TrimSpace(choice.ID),
		Kind:     strings.ToLower(strings.TrimSpace(choice.Kind)),
		Question: strings.TrimSpace(choice.Question),
		GapKey:   strings.TrimSpace(choice.GapKey),
	}
	options := make([]ChoiceOption, 0, len(choice.Options))
	for i, option := range choice.Options {
		title := strings.TrimSpace(option.Title)
		if title == "" {
			continue
		}
		id := strings.TrimSpace(option.ID)
		if id == "" {
			id = fmt.Sprintf("o%d", i+1)
		}
		shortcut := strings.ToUpper(strings.TrimSpace(option.Shortcut))
		if shortcut == "" {
			shortcut = string(rune('A' + i))
		}
		options = append(options, ChoiceOption{
			ID:          id,
			Shortcut:    shortcut,
			Title:       title,
			Description: strings.TrimSpace(option.Description),
			Recommended: option.Recommended,
			Freeform:    option.Freeform,
		})
	}
	if normalized.Question == "" || len(options) < 2 {
		return nil
	}
	if normalized.Kind == "" {
		normalized.Kind = "clarify"
	}
	normalized.Options = options
	return normalized
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
