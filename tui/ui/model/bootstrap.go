package tui

import (
	"os"
	"strconv"
	"strings"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	"bytemind/internal/tools"
	tuiruntime "bytemind/internal/tui/runtime"
	tuiservices "bytemind/internal/tui/services"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

func newModel(opts Options) model {
	ensureZoneManager()
	async := make(chan tea.Msg, 128)

	input := textarea.New()
	input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	input.Focus()
	input.CharLimit = 0
	input.SetWidth(72)
	input.SetHeight(2)
	input.ShowLineNumbers = false
	input.Prompt = ""

	spin := spinner.New()
	spin.Spinner = spinner.MiniDot
	spin.Style = lipgloss.NewStyle().Foreground(colorAccent)

	vp := viewport.New(0, 0)
	vp.YPosition = 0
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = scrollStep

	copyVP := viewport.New(0, 0)
	copyVP.YPosition = 0

	planVP := viewport.New(0, 0)
	planVP.YPosition = 0
	planVP.MouseWheelEnabled = true
	planVP.MouseWheelDelta = scrollStep

	chatItems, toolRuns := rebuildSessionTimeline(opts.Session)

	opts.Runner.SetObserver(agent.ObserverFunc(func(event agent.Event) {
		async <- agentEventMsg{Event: event}
	}))
	opts.Runner.SetApprovalHandler(func(req tools.ApprovalRequest) (bool, error) {
		reply := make(chan approvalDecision, 1)
		async <- approvalRequestMsg{Request: req, Reply: reply}
		decision := <-reply
		return decision.Approved, decision.Err
	})

	m := model{
		runner:             opts.Runner,
		store:              opts.Store,
		sess:               opts.Session,
		imageStore:         opts.ImageStore,
		cfg:                opts.Config,
		workspace:          opts.Workspace,
		runtime:            opts.Runtime,
		async:              async,
		viewport:           vp,
		copyView:           copyVP,
		planView:           planVP,
		input:              input,
		spinner:            spin,
		chatItems:          chatItems,
		toolRuns:           toolRuns,
		plan:               copyPlanState(opts.Session.Plan),
		sessions:           nil,
		sessionLimit:       defaultSessionLimit,
		screen:             initialScreen(opts.Session),
		mode:               toAgentMode(opts.Session.Mode),
		streamingIndex:     -1,
		statusNote:         "Ready.",
		phase:              "idle",
		traceCollapsed:     true,
		llmConnected:       true,
		chatAutoFollow:     true,
		mentionIndex:       mention.NewWorkspaceFileIndex(opts.Workspace),
		tokenUsage:         newTokenUsageComponent(),
		tokenBudget:        max(1, opts.Config.TokenQuota),
		tokenEstimator:     newRealtimeTokenEstimator(opts.Config.Provider.Model),
		inputImageRefs:     make(map[int]llm.AssetID, 8),
		inputImageMentions: make(map[string]llm.AssetID, 8),
		orphanedImages:     make(map[llm.AssetID]time.Time, 8),
		nextImageID:        nextSessionImageID(opts.Session),
		pastedContents:     make(map[string]pastedContent, maxStoredPastedContents),
		pastedOrder:        make([]string, 0, maxStoredPastedContents),
		nextPasteID:        1,
		clipboard:          defaultClipboardImageReader{},
		clipboardText:      defaultClipboardTextWriter{},
		startupGuide:       opts.StartupGuide,
		mouseYOffset:       resolveMouseYOffset(),
	}
	if m.runtime == nil {
		m.runtime = tuiruntime.NewService(tuiruntime.Dependencies{Runner: opts.Runner, Store: opts.Store, ImageStore: opts.ImageStore, Workspace: opts.Workspace})
	}
	m.services = tuiservices.NewProvider(m.runtime, opts.Session)
	m.skills = m.services.Skills()
	m.inputPolicy = m.services.InputPolicy()
	m.promptBuilder = m.services.PromptBuilder()
	m.imageInputController = tuiservices.NewImageInputController()
	if opts.StartupGuide.Active {
		m.statusNote = opts.StartupGuide.Status
		m.llmConnected = false
		m.phase = "error"
		m.initializeStartupGuide()
	}
	m.restoreTokenUsageFromSession(opts.Session)
	_ = m.refreshSkillCatalog()
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.ensureSessionImageAssets()
	m.ensurePastedContentState()
	m.syncInputStyle()
	m.syncInputOverlays()
	if m.mentionIndex != nil {
		go m.mentionIndex.Prewarm()
	}
	return m
}

func ensureZoneManager() {
	zoneInitOnce.Do(func() {
		zone.NewGlobal()
	})
}

func resolveMouseYOffset() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MOUSE_Y_OFFSET"))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return clamp(value, -10, 10)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		waitForAsync(m.async),
		m.tokenUsage.tickCmd(),
		m.loadSessionsCmd(),
	)
}
