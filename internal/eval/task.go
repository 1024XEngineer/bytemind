package eval

type EvalTask struct {
	ID          string      `yaml:"id"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Workspace   string      `yaml:"workspace"`
	Prompt      string      `yaml:"prompt"`
	Success     []Check     `yaml:"success"`
	Negative    []NegCheck  `yaml:"negative,omitempty"`
	Constraints *Constraints `yaml:"constraints,omitempty"`
}

type Check struct {
	Command        string             `yaml:"command"`
	ExitCode       *int               `yaml:"exit_code"`
	OutputContains []string           `yaml:"output_contains"`
	FileContains   []FileContainsCheck `yaml:"file_contains"`
	FilesModified  []string           `yaml:"files_modified"`
}

type FileContainsCheck struct {
	Path    string `yaml:"path"`
	Pattern string `yaml:"pattern"`
}

type NegCheck struct {
	Type        string   `yaml:"type"`
	ForbiddenPaths  []string `yaml:"forbidden_paths,omitempty"`
	ForbiddenTools  []string `yaml:"forbidden_tools,omitempty"`
	MaxFilesChanged int      `yaml:"max_files_changed,omitempty"`
	Description     string   `yaml:"description,omitempty"`
}

type Constraints struct {
	ForbiddenPaths       []string `yaml:"forbidden_paths,omitempty"`
	ForbiddenTools       []string `yaml:"forbidden_tools,omitempty"`
	MaxFilesChanged      int      `yaml:"max_files_changed,omitempty"`
	RequireTestRun       bool     `yaml:"require_test_run,omitempty"`
	RequireUserApproval  []string `yaml:"require_user_approval_for,omitempty"`
}

type TaskResult struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Passed   bool     `yaml:"passed"`
	Failures []string `yaml:"failures,omitempty"`
}
