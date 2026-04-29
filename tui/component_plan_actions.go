package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	planActionStartExecution = "start execution"
	planActionAdjustPlan     = "adjust plan"

	planPickerKindConverged = "converged"
	planPickerKindClarify   = "clarify"
)

type planActionItem struct {
	ID              string
	Shortcut        string
	TitleText       string
	DescriptionText string
	Action          string
	Recommended     bool
	Freeform        bool
}

func (i planActionItem) FilterValue() string {
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{i.Action, i.TitleText, i.DescriptionText, i.Shortcut}, " ")))
}

func (i planActionItem) Title() string       { return i.TitleText }
func (i planActionItem) Description() string { return i.DescriptionText }

type planActionDelegate struct{}

func (d planActionDelegate) Height() int  { return 4 }
func (d planActionDelegate) Spacing() int { return 1 }
func (d planActionDelegate) Update(tea.Msg, *list.Model) tea.Cmd {
	return nil
}

func (d planActionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	action, ok := item.(planActionItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	width := max(24, m.Width())
	bodyWidth := max(16, width-4)

	titleStyle := strongStyle.Copy().Foreground(colorAccent)
	descStyle := mutedStyle.Copy()
	cardStyle := lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent)

	badgeKind := "accent"
	if selected {
		cardStyle = cardStyle.
			BorderForeground(colorTool).
			Background(semanticColors.WarningSoft)
		titleStyle = titleStyle.Foreground(colorTool)
		descStyle = descStyle.Foreground(lipgloss.Color("#F3E4C7"))
		badgeKind = "warning"
	}

	titleParts := []string{renderPillBadge(action.Shortcut, badgeKind), " ", titleStyle.Render(action.TitleText)}
	if action.Recommended {
		titleParts = append(titleParts, " ", mutedStyle.Render("(推荐)"))
	}
	title := lipgloss.JoinHorizontal(lipgloss.Left, titleParts...)
	desc := descStyle.Width(bodyWidth).Render(action.Description())
	content := lipgloss.JoinVertical(lipgloss.Left, title, desc)
	fmt.Fprint(w, cardStyle.Render(content)) //nolint:errcheck
}

type planActionPicker struct {
	list     list.Model
	kind     string
	title    string
	prompt   string
	hint     string
	choiceID string
}

type planActionPickerConfig struct {
	kind     string
	title    string
	prompt   string
	hint     string
	choiceID string
	items    []planActionItem
}

func newPlanActionPicker(width int, cfg planActionPickerConfig) *planActionPicker {
	items := make([]list.Item, 0, len(cfg.items))
	for _, item := range cfg.items {
		items = append(items, item)
	}

	l := list.New(items, planActionDelegate{}, max(32, width), max(8, len(items)*5))
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.SetStatusBarItemName("option", "options")
	l.ResetSelected()

	return &planActionPicker{
		list:     l,
		kind:     cfg.kind,
		title:    strings.TrimSpace(cfg.title),
		prompt:   strings.TrimSpace(cfg.prompt),
		hint:     strings.TrimSpace(cfg.hint),
		choiceID: strings.TrimSpace(cfg.choiceID),
	}
}

func buildPlanActionPicker(width int, state planpkg.State) *planActionPicker {
	state = planpkg.NormalizeState(state)
	if choice := state.ActiveChoice; choice != nil && len(choice.Options) > 0 {
		return newPlanActionPicker(width, planActionPickerConfig{
			kind:     planPickerKindClarify,
			title:    "当前决策",
			prompt:   choice.Question,
			hint:     clarifyChoiceHint(choice),
			choiceID: choice.ID,
			items:    clarifyChoiceItems(*choice),
		})
	}
	if !planpkg.CanStartExecution(state) {
		return nil
	}
	return newPlanActionPicker(width, planActionPickerConfig{
		kind:   planPickerKindConverged,
		title:  "下一步",
		prompt: "计划已收敛，请确认下一步动作。",
		hint:   "A/B 直接选择，或用 ↑/↓ 后按 Enter 确认",
		items: []planActionItem{
			{
				ID:              "start_execution",
				Shortcut:        "A",
				TitleText:       "切到 Build 模式，开始执行",
				DescriptionText: "按当前计划基线进入执行，直接从第一步开始落地。",
				Action:          planActionStartExecution,
				Recommended:     true,
			},
			{
				ID:              "adjust_plan",
				Shortcut:        "B",
				TitleText:       "继续微调计划",
				DescriptionText: "保持在 Plan 模式，继续补充范围、风险或验收标准。",
				Action:          planActionAdjustPlan,
			},
		},
	})
}

func clarifyChoiceItems(choice planpkg.ActiveChoice) []planActionItem {
	items := make([]planActionItem, 0, len(choice.Options))
	for _, option := range choice.Options {
		items = append(items, planActionItem{
			ID:              option.ID,
			Shortcut:        strings.ToUpper(strings.TrimSpace(option.Shortcut)),
			TitleText:       option.Title,
			DescriptionText: option.Description,
			Action:          formatActiveChoiceAction(choice.ID, option.ID),
			Recommended:     option.Recommended,
			Freeform:        option.Freeform,
		})
	}
	return items
}

func clarifyChoiceHint(choice *planpkg.ActiveChoice) string {
	if choice == nil {
		return "A/B 直接选择，或用 ↑/↓ 后按 Enter 确认"
	}
	for _, option := range choice.Options {
		if option.Freeform {
			return "A/B/C 直接选择，或用 ↑/↓ 后按 Enter 确认。选择 Other 后可在输入框补充说明"
		}
	}
	return "A/B/C 直接选择，或用 ↑/↓ 后按 Enter 确认"
}

func formatActiveChoiceAction(choiceID, optionID string) string {
	return "choice:" + strings.TrimSpace(choiceID) + ":" + strings.TrimSpace(optionID)
}

func parseActiveChoiceAction(action string) (choiceID, optionID string, ok bool) {
	if !strings.HasPrefix(action, "choice:") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(action, "choice:"), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	choiceID = strings.TrimSpace(parts[0])
	optionID = strings.TrimSpace(parts[1])
	if choiceID == "" || optionID == "" {
		return "", "", false
	}
	return choiceID, optionID, true
}

func (m *model) syncPlanActionPicker() {
	if m == nil {
		return
	}
	if !m.canShowPlanActionPicker() {
		m.closePlanActionPicker()
		return
	}
	m.planAction = buildPlanActionPicker(m.chatPanelInnerWidth(), m.plan)
	m.planActionOpen = m.planAction != nil
	m.updatePlanActionPickerSize()
}

func (m *model) canShowPlanActionPicker() bool {
	if m == nil || m.mode != modePlan || m.busy {
		return false
	}
	return hasRenderablePlanAction(m.plan)
}

func hasRenderablePlanAction(state planpkg.State) bool {
	state = planpkg.NormalizeState(state)
	return planpkg.HasActiveChoice(state) || planpkg.CanStartExecution(state)
}

func (m *model) closePlanActionPicker() {
	if m == nil {
		return
	}
	m.planActionOpen = false
}

func (m *model) updatePlanActionPickerSize() {
	if m == nil || m.planAction == nil {
		return
	}
	width := min(72, max(36, m.chatPanelInnerWidth()))
	m.planAction.list.SetSize(width, max(8, len(m.planAction.list.Items())*5))
}

func (m model) renderPlanActionPicker() string {
	if m.planAction == nil || !m.planActionOpen {
		return ""
	}

	picker := m.planAction.list
	boxWidth := min(76, max(40, m.chatPanelInnerWidth()))
	innerWidth := boxWidth - modalBoxStyle.GetHorizontalFrameSize() - 2
	picker.SetSize(innerWidth, max(8, len(picker.Items())*5))

	lines := []string{modalTitleStyle.Render(m.planAction.title)}
	if prompt := strings.TrimSpace(m.planAction.prompt); prompt != "" {
		lines = append(lines, strongStyle.Width(innerWidth).Render(prompt))
	}
	if hint := strings.TrimSpace(m.planAction.hint); hint != "" {
		lines = append(lines, mutedStyle.Width(innerWidth).Render(hint))
	}
	lines = append(lines, "", picker.View())
	return modalBoxStyle.Width(boxWidth).Render(strings.Join(lines, "\n"))
}

func (m model) handlePlanActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(strings.TrimSpace(msg.String())) {
	case "esc":
		m.closePlanActionPicker()
		m.statusNote = "已收起当前选项。"
		return m, nil
	case "enter":
		if action, ok := m.selectedPlanAction(); ok {
			return m.executePlanActionSelection(action)
		}
		return m, nil
	}

	if action, ok := m.lookupPlanActionByShortcut(msg.String()); ok {
		return m.executePlanActionSelection(action)
	}

	if m.planAction == nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.planAction.list, cmd = m.planAction.list.Update(msg)
	return m, cmd
}

func (m model) lookupPlanActionByShortcut(input string) (string, bool) {
	if m.planAction == nil {
		return "", false
	}
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return "", false
	}
	for index, raw := range m.planAction.list.Items() {
		item, ok := raw.(planActionItem)
		if !ok {
			continue
		}
		number := fmt.Sprintf("%d", index+1)
		shortcut := strings.ToLower(strings.TrimSpace(item.Shortcut))
		if normalized == number || normalized == number+"." || normalized == shortcut || normalized == shortcut+"." {
			return item.Action, true
		}
	}
	return "", false
}

func (m model) selectedPlanAction() (string, bool) {
	if m.planAction == nil {
		return "", false
	}
	item, ok := m.planAction.list.SelectedItem().(planActionItem)
	if !ok {
		return "", false
	}
	return item.Action, true
}

func (m model) planActionItemByAction(action string) (planActionItem, bool) {
	if m.planAction == nil {
		return planActionItem{}, false
	}
	for _, raw := range m.planAction.list.Items() {
		item, ok := raw.(planActionItem)
		if ok && item.Action == action {
			return item, true
		}
	}
	return planActionItem{}, false
}

func (m model) executePlanActionSelection(action string) (tea.Model, tea.Cmd) {
	item, _ := m.planActionItemByAction(action)
	return m.submitPlanActionSelection(action, item, m.planActionDisplayText(action, item))
}

func (m model) submitPlanActionSelectionWithDisplay(action, displayText string) (tea.Model, tea.Cmd) {
	item, _ := syntheticPlanActionItemForAction(m.plan, action)
	return m.submitPlanActionSelection(action, item, strings.TrimSpace(displayText))
}

func (m model) submitPlanActionSelection(action string, item planActionItem, displayText string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(displayText) == "" {
		displayText = m.planActionDisplayText(action, item)
	}
	m.closePlanActionPicker()

	switch {
	case action == planActionStartExecution:
		state, err := preparePlanForContinuation(m.plan)
		if err != nil {
			m.statusNote = err.Error()
			return m, nil
		}
		m.plan = state
		m.mode = modeBuild
		if m.sess != nil {
			m.sess.Mode = planpkg.ModeBuild
			m.sess.Plan = copyPlanState(state)
			if m.store != nil {
				if err := m.store.Save(m.sess); err != nil {
					m.statusNote = err.Error()
					return m, nil
				}
			}
		}
		prompt := buildExecutionStartPrompt(state)
		return m.submitPreparedPrompt(RunPromptInput{
			UserMessage: llm.NewUserTextMessage(prompt),
			DisplayText: displayText,
		}, displayText)
	case action == planActionAdjustPlan:
		m.input.Reset()
		m.clearPasteTransaction()
		m.clearVirtualPasteParts()
		m.screen = screenChat
		m.statusNote = "Plan mode kept. Describe what to refine next."
		m.phase = "idle"
		m.chatAutoFollow = true
		return m, nil
	default:
		if item.Freeform {
			m.statusNote = "请在输入框补充你的自定义方案，然后按 Enter。"
			return m, nil
		}
		if choiceID, optionID, ok := parseActiveChoiceAction(action); ok {
			prompt := buildActiveChoiceSelectionPrompt(m.plan, item, choiceID, optionID)
			return m.submitPreparedPrompt(RunPromptInput{
				UserMessage: llm.NewUserTextMessage(prompt),
				DisplayText: displayText,
			}, displayText)
		}
		return m.submitPreparedPrompt(RunPromptInput{
			UserMessage: llm.NewUserTextMessage(action),
			DisplayText: displayText,
		}, displayText)
	}
}

func buildExecutionStartPrompt(state planpkg.State) string {
	state = planpkg.NormalizeState(state)
	currentStep := ""
	if step, ok := planpkg.CurrentStep(state); ok {
		currentStep = strings.TrimSpace(step.Title)
	}
	nextAction := strings.TrimSpace(state.NextAction)
	if nextAction == "" {
		nextAction = planpkg.DefaultNextAction(state)
	}

	lines := []string{
		"start execution",
		"",
		"Execution handoff is already approved.",
		"The session has switched to build mode.",
	}
	if currentStep != "" {
		lines = append(lines, "Current step: "+currentStep)
	}
	if nextAction != "" {
		lines = append(lines, "Next action: "+nextAction)
	}
	lines = append(lines,
		"Begin concrete execution from the current plan baseline in this turn.",
		"Do not stop at a confirmation-only reply.",
		"Emit structured tool calls immediately unless a real blocker requires user input.",
	)
	return strings.Join(lines, "\n")
}

func (m model) planActionDisplayText(action string, item planActionItem) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case planActionStartExecution:
		return "A. 切到 Build 模式，开始执行"
	case planActionAdjustPlan:
		return "B. 继续微调计划"
	default:
		if strings.TrimSpace(item.Shortcut) != "" && strings.TrimSpace(item.TitleText) != "" {
			return strings.TrimSpace(item.Shortcut) + ". " + strings.TrimSpace(item.TitleText)
		}
		return strings.TrimSpace(action)
	}
}

func buildActiveChoiceSelectionPrompt(state planpkg.State, item planActionItem, choiceID, optionID string) string {
	question := ""
	if state.ActiveChoice != nil {
		question = strings.TrimSpace(state.ActiveChoice.Question)
	}
	return fmt.Sprintf(
		`selected_choice: {"choice_id":%q,"option_id":%q,"shortcut":%q,"title":%q,"question":%q}`,
		choiceID,
		optionID,
		strings.TrimSpace(item.Shortcut),
		strings.TrimSpace(item.TitleText),
		question,
	)
}

func syntheticPlanActionItemForAction(state planpkg.State, action string) (planActionItem, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case planActionStartExecution:
		return planActionItem{
			ID:              "start_execution",
			Shortcut:        "A",
			TitleText:       "切到 Build 模式，开始执行",
			DescriptionText: "按当前计划基线进入执行，直接从第一步开始落地。",
			Action:          planActionStartExecution,
			Recommended:     true,
		}, true
	case planActionAdjustPlan:
		return planActionItem{
			ID:              "adjust_plan",
			Shortcut:        "B",
			TitleText:       "继续微调计划",
			DescriptionText: "保持在 Plan 模式，继续补充范围、风险或验收标准。",
			Action:          planActionAdjustPlan,
		}, true
	default:
		if choiceID, optionID, ok := parseActiveChoiceAction(action); ok && state.ActiveChoice != nil && state.ActiveChoice.ID == choiceID {
			for _, option := range state.ActiveChoice.Options {
				if option.ID == optionID {
					return planActionItem{
						ID:              option.ID,
						Shortcut:        option.Shortcut,
						TitleText:       option.Title,
						DescriptionText: option.Description,
						Action:          action,
						Recommended:     option.Recommended,
						Freeform:        option.Freeform,
					}, true
				}
			}
		}
		return planActionItem{}, false
	}
}

func planActionStatusNote(state planpkg.State) string {
	switch {
	case planpkg.HasActiveChoice(state):
		return "Choose the current decision from the picker."
	case planpkg.CanStartExecution(state):
		return "Choose the next step from the picker."
	default:
		return "Ready."
	}
}

func stripPlanActionTailFromAnswer(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	endTag := "</proposed_plan>"
	index := strings.LastIndex(strings.ToLower(content), strings.ToLower(endTag))
	if index < 0 {
		return content
	}
	index += len(endTag)
	prefix := strings.TrimRight(content[:index], "\n")
	suffix := strings.TrimSpace(content[index:])
	if !looksLikePlanActionTail(suffix) {
		return content
	}
	return prefix
}

func looksLikePlanActionTail(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "start execution") && strings.Contains(normalized, "adjust plan") {
		return true
	}
	if strings.Contains(normalized, "切到 build") || strings.Contains(normalized, "继续微调计划") {
		return true
	}
	return strings.Contains(normalized, "choose next step") ||
		strings.Contains(normalized, "choose next action") ||
		strings.Contains(normalized, "下一步") ||
		strings.Contains(normalized, "reply with 1") ||
		strings.Contains(normalized, "开始执行") ||
		strings.Contains(normalized, "调整计划")
}

func stripClarifyChoiceBlockFromAnswer(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	start, end, ok := findClarifyChoiceBlock(lines)
	if !ok {
		return content
	}
	prefix := strings.TrimSpace(strings.Join(lines[:start], "\n"))
	suffix := strings.TrimSpace(strings.Join(lines[end:], "\n"))
	switch {
	case prefix == "" && suffix == "":
		return "请在下方卡片中选择当前决策。"
	case prefix == "":
		return suffix
	case suffix == "":
		return prefix
	default:
		return prefix + "\n\n" + suffix
	}
}

func findClarifyChoiceBlock(lines []string) (start, end int, ok bool) {
	for i := 0; i < len(lines); i++ {
		if !isClarifyQuestionLine(lines[i]) {
			continue
		}
		optionCount := 0
		j := i + 1
		for ; j < len(lines); j++ {
			line := strings.TrimSpace(lines[j])
			if line == "" {
				continue
			}
			if isClarifyOptionLine(line) {
				optionCount++
				continue
			}
			break
		}
		if optionCount >= 2 {
			return i, j, true
		}
	}
	return 0, 0, false
}

func isClarifyQuestionLine(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(normalized, "question:") ||
		strings.HasPrefix(normalized, "question：") ||
		strings.HasPrefix(normalized, "问题:") ||
		strings.HasPrefix(normalized, "问题：")
}

func isClarifyOptionLine(line string) bool {
	normalized := strings.ToLower(strings.TrimSpace(line))
	for _, prefix := range []string{"a.", "b.", "c.", "d.", "other:", "other：", "其他:", "其他："} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}
