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

		// Track files modified by git diff before agent runs (for computing delta)
		beforeFiles := listGitTrackedFiles(workspace)

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

		// Run negative checks (constraints that should NOT be violated)
		for _, neg := range task.Negative {
			switch neg.Type {
			case "read_only":
				// Verify no files were modified (read-only mode)
				afterFiles := listGitTrackedFiles(workspace)
				modified := gitDiffFiles(workspace)
				untracked := gitUntrackedFiles(workspace)
				if len(modified) > 0 {
					result.Passed = false
					result.Failures = append(result.Failures,
						fmt.Sprintf("negative[read_only]: agent modified %d tracked file(s) but should not have: %v", len(modified), modified))
				}
				if len(untracked) > 0 {
					result.Passed = false
					result.Failures = append(result.Failures,
						fmt.Sprintf("negative[read_only]: agent created %d untracked file(s) but should not have: %v", len(untracked), untracked))
				}
				if !stringSetsEqual(beforeFiles, afterFiles) {
					result.Passed = false
					result.Failures = append(result.Failures,
						"negative[read_only]: agent deleted tracked files")
				}
			case "forbidden_paths":
				allChanged := allChangedFiles(workspace)
				var violations []string
				for _, fp := range neg.ForbiddenPaths {
					for _, mf := range allChanged {
						if matched, _ := filepath.Match(fp, mf); matched {
							violations = append(violations, mf)
						}
					}
				}
				if len(violations) > 0 {
					result.Passed = false
					result.Failures = append(result.Failures,
						fmt.Sprintf("negative[forbidden_paths]: agent modified forbidden file(s): %v", violations))
				}
			case "max_files_changed":
				allChanged := allChangedFiles(workspace)
				if neg.MaxFilesChanged > 0 && len(allChanged) > neg.MaxFilesChanged {
					result.Passed = false
					result.Failures = append(result.Failures,
						fmt.Sprintf("negative[max_files_changed]: agent changed %d files (max: %d): %v",
							len(allChanged), neg.MaxFilesChanged, allChanged))
				}
			}
		}

		// Check task-level constraints
		if task.Constraints != nil {
			allChanged := allChangedFiles(workspace)
			if task.Constraints.MaxFilesChanged > 0 && len(allChanged) > task.Constraints.MaxFilesChanged {
				result.Passed = false
				result.Failures = append(result.Failures,
					fmt.Sprintf("constraint max_files_changed: agent changed %d files (max: %d): %v",
						len(allChanged), task.Constraints.MaxFilesChanged, allChanged))
			}
			for _, fp := range task.Constraints.ForbiddenPaths {
				for _, mf := range allChanged {
					if matched, _ := filepath.Match(fp, mf); matched {
						result.Passed = false
						result.Failures = append(result.Failures,
							fmt.Sprintf("constraint forbidden_paths: agent modified %s (matches %s)", mf, fp))
					}
				}
			}
		}

		results = append(results, result)
	}
	return results
}

func listGitTrackedFiles(workspace string) []string {
	cmd := exec.Command("git", "-C", workspace, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func gitDiffFiles(workspace string) []string {
	cmd := exec.Command("git", "-C", workspace, "diff", "--name-only")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return []string{}
	}
	lines := strings.Split(trimmed, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func gitUntrackedFiles(workspace string) []string {
	cmd := exec.Command("git", "-C", workspace, "ls-files", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return []string{}
	}
	lines := strings.Split(trimmed, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// allChangedFiles returns tracked modifications + untracked files.
func allChangedFiles(workspace string) []string {
	modified := gitDiffFiles(workspace)
	untracked := gitUntrackedFiles(workspace)
	if len(modified) == 0 && len(untracked) == 0 {
		return []string{}
	}
	if len(modified) == 0 {
		return untracked
	}
	if len(untracked) == 0 {
		return modified
	}
	all := make([]string, 0, len(modified)+len(untracked))
	all = append(all, modified...)
	all = append(all, untracked...)
	return all
}

func stringSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}

func RunSmokeChecks(tasks []EvalTask) []TaskResult {
	results := make([]TaskResult, 0, len(tasks))
	for _, task := range tasks {
		result := TaskResult{ID: task.ID, Name: task.Name, Passed: true}
		workspace := task.Workspace
		for _, check := range task.Success {
			// Smoke checks only verify static file conditions.
			// Command checks (go test, etc.) require agent execution and
			// will fail naturally for bugfix/refactor fixtures.
			if len(check.FileContains) > 0 {
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
