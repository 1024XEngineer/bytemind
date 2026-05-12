package eval

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func RunTasks(binPath string, tasks []EvalTask) []TaskResult {
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
		_ = output

		if err != nil {
			// bytemind run returns non-zero exit for errors,
			// but the agent may still complete the task
		}

		// Check each success condition
		for _, check := range task.Success {
			switch {
			case check.Command != "":
				if ok, msg := CheckCommand(check.Command, check.ExitCode, workspace); !ok {
					result.Passed = false
					result.Failures = append(result.Failures, msg)
				}
			case len(check.OutputContains) > 0:
				if ok, msg := CheckOutputContains(string(output), check.OutputContains); !ok {
					result.Passed = false
					result.Failures = append(result.Failures, msg)
				}
			case len(check.FileContains) > 0:
				for _, fc := range check.FileContains {
					if ok, msg := CheckFileContains(workspace, fc.Path, fc.Pattern); !ok {
						result.Passed = false
						result.Failures = append(result.Failures, msg)
					}
				}
			case len(check.FilesModified) > 0:
				for _, path := range check.FilesModified {
					fullPath := filepath.Join(workspace, path)
					if _, err := os.Stat(fullPath); err != nil {
						result.Passed = false
						result.Failures = append(result.Failures, fmt.Sprintf("file missing: %s", path))
						continue
					}
					gitCmd := exec.Command("git", "-C", workspace, "diff", "--exit-code", "--", path)
					if gitCmd.Run() == nil {
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

func RunSmokeChecks(tasks []EvalTask) []TaskResult {
	results := make([]TaskResult, 0, len(tasks))
	for _, task := range tasks {
		result := TaskResult{ID: task.ID, Name: task.Name, Passed: true}
		workspace := task.Workspace
		for _, check := range task.Success {
			switch {
			case check.Command != "":
				if ok, msg := CheckCommand(check.Command, check.ExitCode, workspace); !ok {
					result.Passed = false
					result.Failures = append(result.Failures, msg)
				}
			case len(check.FileContains) > 0:
				for _, fc := range check.FileContains {
					if ok, msg := CheckFileContains(workspace, fc.Path, fc.Pattern); !ok {
						result.Passed = false
						result.Failures = append(result.Failures, msg)
					}
				}
			}
		}
		results = append(results, result)
	}
	return results
}

func PrintResults(results []TaskResult) {
	allPassed := true
	for _, r := range results {
		if r.Passed {
			fmt.Printf("\u2713 %s (%s)\n", r.ID, r.Name)
		} else {
			allPassed = false
			fmt.Printf("\u2717 %s (%s)\n", r.ID, r.Name)
			for _, f := range r.Failures {
				fmt.Printf("  - %s\n", f)
			}
		}
	}
	if allPassed {
		fmt.Printf("\nAll %d task(s) passed.\n", len(results))
	}
}

func AllPassed(results []TaskResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}

func CheckCommand(command string, expectedExitCode *int, workspace string) (bool, string) {
	if command == "" {
		return false, "empty command in success check"
	}
	var cmd *exec.Cmd
	shell, err := exec.LookPath("bash")
	if err == nil {
		cmd = exec.Command(shell, "-c", command)
	} else {
		shell, err := exec.LookPath("sh")
		if err == nil {
			cmd = exec.Command(shell, "-c", command)
		} else {
			parts := strings.Fields(command)
			if len(parts) == 0 {
				return false, "empty command in success check"
			}
			cmd = exec.Command(parts[0], parts[1:]...)
		}
	}
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

func CheckOutputContains(output string, patterns []string) (bool, string) {
	for _, p := range patterns {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(p)) {
			return false, fmt.Sprintf("output does not contain %q", p)
		}
	}
	return true, ""
}

func CheckFileContains(workspace, path, pattern string) (bool, string) {
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
