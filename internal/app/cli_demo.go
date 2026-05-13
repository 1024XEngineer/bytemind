package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const builtinDemoName = "bugfix"

var demoFixtures = map[string]struct {
	workspace string
	prompt    string
	desc      string
}{
	"bugfix": {
		workspace: "examples/bugfix-demo/broken-project",
		prompt:    "Fix the failing test in the project and verify all tests pass",
		desc:      "Fix a divide-by-zero bug in a Go calculator project",
	},
}

// demoExecutable is overridable in tests to avoid running a real binary.
var demoExecutable = os.Executable

func RunDemo(args []string, stdout, stderr io.Writer) error {
	demoName := builtinDemoName
	if len(args) > 0 && args[0] != "" && !strings.HasPrefix(args[0], "-") {
		demoName = strings.TrimSpace(args[0])
		args = args[1:]
	}

	fixture, ok := demoFixtures[demoName]
	if !ok {
		fmt.Fprintf(stderr, "Unknown demo %q.\n", demoName)
		fmt.Fprintf(stderr, "Available demos:\n")
		for name, f := range demoFixtures {
			fmt.Fprintf(stderr, "  %s: %s\n", name, f.desc)
		}
		return fmt.Errorf("unknown demo: %s", demoName)
	}

	// Resolve the fixture workspace relative to the project root
	// First, find project root (where go.mod is)
	projectRoot := findProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("cannot find project root (no go.mod found); run from the bytemind repository")
	}
	srcWorkspace := filepath.Join(projectRoot, fixture.workspace)

	if info, err := os.Stat(srcWorkspace); err != nil || !info.IsDir() {
		return fmt.Errorf("demo fixture not found at %s", srcWorkspace)
	}

	// Create a temporary copy of the workspace so demo is reproducible
	tmpDir, err := os.MkdirTemp("", "bytemind-demo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	destWorkspace := filepath.Join(tmpDir, "workspace")
	if err := copyDir(srcWorkspace, destWorkspace); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}

	// Initialize git in the temp workspace so bytemind can use it
	gitInitCmd := exec.Command("git", "init")
	gitInitCmd.Dir = destWorkspace
	if err := gitInitCmd.Run(); err != nil {
		return fmt.Errorf("git init in workspace: %w", err)
	}
	gitAddCmd := exec.Command("git", "add", ".")
	gitAddCmd.Dir = destWorkspace
	if err := gitAddCmd.Run(); err != nil {
		return fmt.Errorf("git add in workspace: %w", err)
	}
	gitCommitCmd := exec.Command("git", "commit", "-m", "initial state")
	gitCommitCmd.Dir = destWorkspace
	gitCommitCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=bytemind-demo", "GIT_AUTHOR_EMAIL=demo@bytemind.dev",
		"GIT_COMMITTER_NAME=bytemind-demo", "GIT_COMMITTER_EMAIL=demo@bytemind.dev")
	if err := gitCommitCmd.Run(); err != nil {
		return fmt.Errorf("git commit in workspace: %w", err)
	}

	fmt.Fprintf(stdout, "ByteMind Demo: %s\n", demoName)
	fmt.Fprintf(stdout, "  %s\n\n", fixture.desc)
	fmt.Fprintf(stdout, "Running agent to: %s\n", fixture.prompt)
	fmt.Fprintf(stdout, "Workspace: %s\n\n", destWorkspace)

	// Run bytemind as a subprocess with the demo prompt
	binPath, err := demoExecutable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	cmd := exec.Command(binPath, "run",
		"-prompt", fixture.prompt,
		"-workspace", destWorkspace,
		"-approval-mode", "full_access",
		"-max-iterations", "20",
	)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("demo run failed: %w", err)
	}

	// Show final diff
	fmt.Fprintf(stdout, "\n--- Changes Made ---\n")
	gitDiffCmd := exec.Command("git", "-C", destWorkspace, "diff", "--stat")
	diffStat, _ := gitDiffCmd.Output()
	if len(diffStat) > 0 {
		fmt.Fprintf(stdout, "%s", diffStat)
	}

	gitDiffFullCmd := exec.Command("git", "-C", destWorkspace, "diff")
	diffFull, _ := gitDiffFullCmd.Output()
	if len(diffFull) > 0 {
		fmt.Fprintf(stdout, "\n%s\n", diffFull)
	} else {
		fmt.Fprintf(stdout, "(no changes detected)\n")
	}

	fmt.Fprintf(stdout, "\nDemo complete.\n")
	return nil
}

func findProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
