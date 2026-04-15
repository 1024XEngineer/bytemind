package tui

import (
	"context"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	tuiapi "bytemind/internal/tui/api"
	tuiruntime "bytemind/internal/tui/runtime"
	tuiservices "bytemind/internal/tui/services"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	runner               *agent.Runner
	store                *session.Store
	sess                 *session.Session
	imageStore           assets.ImageStore
	cfg                  config.Config
	workspace            string
	runtime              tuiruntime.UIAPI
	skills               tuiapi.SkillsManager
	inputPolicy          tuiapi.InputPolicy
	promptBuilder        tuiapi.PromptBuilder
	imageInputController *tuiservices.ImageInputController
	services             tuiapi.Provider

	width  int
	height int

	async    chan tea.Msg
	viewport viewport.Model
	copyView viewport.Model
	planView viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	viewportContentCache  string
	viewportTopCache      viewportTopLookupCache
	chatItems             []chatEntry
	toolRuns              []toolRun
	plan                  planpkg.State
	sessions              []session.Summary
	sessionLimit          int
	sessionCursor         int
	commandCursor         int
	mentionCursor         int
	screen                screenKind
	mode                  agentMode
	sessionsOpen          bool
	skillsOpen            bool
	helpOpen              bool
	commandOpen           bool
	mentionOpen           bool
	promptSearchOpen      bool
	busy                  bool
	streamingIndex        int
	bufferedAssistantText string
	thinkingStartedAt     time.Time
	runFinishedAt         time.Time
	traceCollapsed        bool
	statusNote            string
	phase                 string
	llmConnected          bool
	approval              *approvalPrompt
	mentionQuery          string
	mentionToken          mention.Token
	mentionResults        []mention.Candidate
	mentionIndex          *mention.WorkspaceFileIndex
	mentionRecent         map[string]int
	mentionSeq            int
	lastPasteAt           time.Time
	lastInputAt           time.Time
	inputBurstSize        int
	chatAutoFollow        bool
	draggingScrollbar     bool
	scrollbarDragOffset   int
	mouseSelecting        bool
	mouseSelectionMouseX  int
	mouseSelectionMouseY  int
	mouseSelectionTickID  int
	mouseSelectionActive  bool
	mouseSelectionStart   viewportSelectionPoint
	mouseSelectionEnd     viewportSelectionPoint
	inputMouseSelecting   bool
	inputSelectionActive  bool
	inputSelectionStart   viewportSelectionPoint
	inputSelectionEnd     viewportSelectionPoint
	selectionToast        string
	selectionToastID      int
	tokenUsage            tokenUsageComponent
	tokenUsedTotal        int
	tokenBudget           int
	tokenInput            int
	tokenOutput           int
	tokenContext          int
	tokenHasOfficialUsage bool
	tempEstimatedOutput   int
	tokenEstimator        *realtimeTokenEstimator
	promptHistoryLoaded   bool
	promptHistoryEntries  []history.PromptEntry
	promptSearchMode      promptSearchMode
	promptSearchQuery     string
	promptSearchMatches   []history.PromptEntry
	promptSearchCursor    int
	promptSearchBaseInput string
	inputImageRefs        map[int]llm.AssetID
	inputImageMentions    map[string]llm.AssetID
	orphanedImages        map[llm.AssetID]time.Time
	nextImageID           int
	pastedContents        map[string]pastedContent
	pastedOrder           []string
	nextPasteID           int
	pastedStateLoaded     bool
	lastCompressedPasteAt time.Time
	clipboard             clipboardImageReader
	clipboardText         clipboardTextWriter
	runCancel             context.CancelFunc
	pendingBTW            []string
	interrupting          bool
	interruptSafe         bool
	runSeq                int
	activeRunID           int
	startupGuide          StartupGuide
	skillCatalog          tuiapi.SkillsState
	mouseYOffset          int
}
