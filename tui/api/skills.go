package api

type SkillItem struct {
	Name        string
	Scope       string
	Description string
	Slash       string
	Aliases     []string
	ToolPolicy  string
}

type SkillDiagnostic struct {
	Level   string
	Skill   string
	Path    string
	Message string
}

type ActiveSkill struct {
	Name  string
	Scope string
	Args  map[string]string
}

type SkillsState struct {
	Active      *ActiveSkill
	Items       []SkillItem
	Diagnostics []SkillDiagnostic
}

type SkillActivation struct {
	Name       string
	Scope      string
	EntrySlash string
	ToolPolicy string
	Args       map[string]string
}

type SkillDeleteResult struct {
	Name          string
	Dir           string
	ClearedActive bool
}

type SkillsManager interface {
	GetState() Result[SkillsState]
	Activate(name string, args map[string]string) Result[SkillActivation]
	Clear() Result[string]
	Delete(name string) Result[SkillDeleteResult]
}
