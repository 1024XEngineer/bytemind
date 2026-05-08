package subagents

import "time"

type Scope string

const (
	ScopeBuiltin Scope = "builtin"
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

type Agent struct {
	Name            string
	Description     string
	Scope           Scope
	SourcePath      string
	Entry           string
	Instruction     string
	Tools           []string
	DisallowedTools []string
	Model           string
	Mode            string
	MaxTurns        int
	Timeout         string
	Output          string
	Isolation       string
	PermissionMode  string // reserved: subagent permission strategy (inherit, bubble, acceptEdits, plan); MVP ignores
	Aliases         []string
	WhenToUse       string
	DiscoveredAt    time.Time
}

type Diagnostic struct {
	Scope   Scope
	Path    string
	Agent   string
	Level   string
	Message string
}

type Override struct {
	Name       string
	Winner     Scope
	Loser      Scope
	WinnerPath string
	LoserPath  string
}

type Catalog struct {
	Agents      []Agent
	Diagnostics []Diagnostic
	Overrides   []Override
	LoadedAt    time.Time
}
