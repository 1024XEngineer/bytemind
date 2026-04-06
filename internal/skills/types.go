package skills

import "time"

type Scope string

const (
	ScopeBuiltin Scope = "builtin"
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

type ToolPolicyMode string

const (
	ToolPolicyInherit   ToolPolicyMode = "inherit"
	ToolPolicyAllowlist ToolPolicyMode = "allowlist"
	ToolPolicyDenylist  ToolPolicyMode = "denylist"
)

type Entry struct {
	Slash string `json:"slash"`
}

type PromptRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type ResourceRef struct {
	ID       string `json:"id"`
	URI      string `json:"uri"`
	Optional bool   `json:"optional"`
}

type ToolPolicy struct {
	Policy ToolPolicyMode `json:"policy"`
	Items  []string       `json:"items"`
}

type Arg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default"`
}

type Skill struct {
	Name          string
	Version       string
	Title         string
	Description   string
	DescriptionZH string
	WhenToUse     string
	Scope         Scope
	SourceDir     string
	Instruction   string
	Entry         Entry
	Prompts       []PromptRef
	Resources     []ResourceRef
	ToolPolicy    ToolPolicy
	Args          []Arg
	Aliases       []string
	DiscoveredAt  time.Time
}

type Diagnostic struct {
	Scope   Scope
	Path    string
	Skill   string
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
	Skills      []Skill
	Diagnostics []Diagnostic
	Overrides   []Override
	LoadedAt    time.Time
}
