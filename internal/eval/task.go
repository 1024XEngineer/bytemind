package eval

type EvalTask struct {
	ID          string  `yaml:"id"`
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Workspace   string  `yaml:"workspace"`
	Prompt      string  `yaml:"prompt"`
	Success     []Check `yaml:"success"`
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

type TaskResult struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Passed   bool     `yaml:"passed"`
	Failures []string `yaml:"failures,omitempty"`
}
