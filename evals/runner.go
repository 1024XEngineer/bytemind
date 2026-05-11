package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type EvalTask struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Workspace   string   `yaml:"workspace"`
	Prompt      string   `yaml:"prompt"`
	Success     []Check  `yaml:"success"`
}

type Check struct {
	Command       string   `yaml:"command"`
	ExitCode      *int     `yaml:"exit_code"`
	OutputContains []string `yaml:"output_contains"`
	FileContains  []FileContainsCheck `yaml:"file_contains"`
	FilesModified []string `yaml:"files_modified"`
}

type FileContainsCheck struct {
	Path    string `yaml:"path"`
	Pattern string `yaml:"pattern"`
}

type TaskResult struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Passed   bool   `yaml:"passed"`
	Failures []string `yaml:"failures,omitempty"`
}

func main() {
	bytemindBin := flag.String("bin", "", "Path to bytemind binary (default: build from source)")
	list := flag.Bool("list", false, "List available tasks and exit")
	runID := flag.String("run", "", "Run a specific task by ID, or 'all' to run all")
	tasksDir := flag.String("tasks", "evals/tasks", "Directory containing eval task YAML files")
	flag.Parse()

	tasks := loadTasks(*tasksDir)

	if *list {
		fmt.Println("Available eval tasks:")
		for _, task := range tasks {
			fmt.Printf("  %s: %s\n", task.ID, task.Name)
		}
		return
	}

	if *runID == "" {
		fmt.Println("Usage: go run ./evals/runner.go -run <task-id|all> [-bin <bytemind-binary>]")
		fmt.Println("       go run ./evals/runner.go -list")
		os.Exit(1)
	}

	binPath := *bytemindBin
	if binPath == "" {
		var err error
		binPath, err = buildBytemind()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build bytemind: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(binPath)
	}

	toRun := tasks
	if *runID != "all" {
		var filtered []EvalTask
		for _, t := range tasks {
			if t.ID == *runID {
				filtered = append(filtered, t)
				break
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "Task %q not found\n", *runID)
			os.Exit(1)
		}
		toRun = filtered
	}

	results := runTasks(binPath, toRun)

	allPassed := true
	for _, r := range results {
		if r.Passed {
			fmt.Printf("✓ %s (%s)\n", r.ID, r.Name)
		} else {
			allPassed = false
			fmt.Printf("✗ %s (%s)\n", r.ID, r.Name)
			for _, f := range r.Failures {
				fmt.Printf("  - %s\n", f)
			}
		}
	}

	if !allPassed {
		os.Exit(1)
	}
	fmt.Printf("\nAll %d task(s) passed.\n", len(results))
}

func loadTasks(dir string) []EvalTask {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read tasks dir %s: %v\n", dir, err)
		return nil
	}
	var tasks []EvalTask
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", entry.Name(), err)
			continue
		}
		var task EvalTask
		if err := yaml.Unmarshal(data, &task); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot parse %s: %v\n", entry.Name(), err)
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func buildBytemind() (string, error) {
	tmp, err := os.MkdirTemp("", "bytemind-eval-*")
	if err != nil {
		return "", err
	}
	binPath := filepath.Join(tmp, "bytemind")
	if filepath.Ext(binPath) == "" && isWindows() {
		binPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/bytemind")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}
	return binPath, nil
}

func isWindows() bool {
	return strings.Contains(strings.ToLower(os.Getenv("OS")), "windows")
}

func runTasks(binPath string, tasks []EvalTask) []TaskResult {
	results := make([]TaskResult, 0, len(tasks))
	for _, task := range tasks {
		result := TaskResult{ID: task.ID, Name: task.Name, Passed: true}
		workspace := task.Workspace
		if workspace == "." {
			workspace = "."
		}

		// Run bytemind with the task prompt
		cmd := exec.Command(binPath, "run",
			"-prompt", task.Prompt,
			"-workspace", workspace,
			"-approval-mode", "full_access",
			"-max-iterations", "30",
		)
		output, err := cmd.CombinedOutput()
		_ = output // keep for potential future analysis

		if err != nil {
			// bytemind run returns non-zero exit for errors,
			// but the agent may still complete the task
		}

		// Check each success condition
		for _, check := range task.Success {
			switch {
			case check.Command != "":
				if ok, msg := checkCommand(check.Command, check.ExitCode, workspace); !ok {
					result.Passed = false
					result.Failures = append(result.Failures, msg)
				}
			case len(check.OutputContains) > 0:
				if ok, msg := checkOutputContains(string(output), check.OutputContains); !ok {
					result.Passed = false
					result.Failures = append(result.Failures, msg)
				}
			case len(check.FileContains) > 0:
				for _, fc := range check.FileContains {
					if ok, msg := checkFileContains(workspace, fc.Path, fc.Pattern); !ok {
						result.Passed = false
						result.Failures = append(result.Failures, msg)
					}
				}
			case len(check.FilesModified) > 0:
				// Verify the file was actually modified by checking git status
				for _, path := range check.FilesModified {
					fullPath := filepath.Join(workspace, path)
					if _, err := os.Stat(fullPath); err != nil {
						result.Passed = false
						result.Failures = append(result.Failures, fmt.Sprintf("file missing: %s", path))
						continue
					}
					// Check git diff to confirm the file was actually changed
					gitCmd := exec.Command("git", "-C", workspace, "diff", "--exit-code", "--", path)
					if gitCmd.Run() == nil {
						// exit code 0 = no diff = file not modified
						result.Passed = false
						result.Failures = append(result.Failures, fmt.Sprintf("file %s was not modified by the agent", path))
					}
				}
			}
		}
		results = append(results, result)
	}
	return results
}

func checkCommand(command string, expectedExitCode *int, workspace string) (bool, string) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false, "empty command in success check"
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	if workspace != "" {
		cmd.Dir = workspace
	}
	if err := cmd.Run(); err != nil {
		if expectedExitCode != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() == *expectedExitCode {
					return true, ""
				}
			}
		}
		return false, fmt.Sprintf("command %q failed: %v", command, err)
	}
	if expectedExitCode != nil && *expectedExitCode != 0 {
		return false, fmt.Sprintf("command %q succeeded but expected exit code %d", command, *expectedExitCode)
	}
	return true, ""
}

func checkOutputContains(output string, patterns []string) (bool, string) {
	for _, p := range patterns {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(p)) {
			return false, fmt.Sprintf("output does not contain %q", p)
		}
	}
	return true, ""
}

func checkFileContains(workspace, path, pattern string) (bool, string) {
	fullPath := path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(workspace, path)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return false, fmt.Sprintf("cannot read %s: %v", path, err)
	}
	matched, err := regexp.MatchString(pattern, string(data))
	if err != nil {
		return false, fmt.Sprintf("bad regex %q: %v", pattern, err)
	}
	if !matched {
		return false, fmt.Sprintf("%s does not match pattern %q", path, pattern)
	}
	return true, ""
}
