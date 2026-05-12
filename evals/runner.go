package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/eval"
)

func main() {
	bytemindBin := flag.String("bin", "", "Path to bytemind binary (default: build from source)")
	list := flag.Bool("list", false, "List available tasks and exit")
	runID := flag.String("run", "", "Run a specific task by ID, or 'all' to run all")
	smoke := flag.Bool("smoke", false, "Run static checks only (no LLM, no binary build)")
	validate := flag.Bool("validate", false, "Validate task YAML files parse correctly (exit 1 on errors)")
	tasksDir := flag.String("tasks", "evals/tasks", "Directory containing eval task YAML files")
	flag.Parse()

	tasks := eval.LoadTasks(*tasksDir)

	if *validate {
		if len(tasks) == 0 {
			fmt.Fprintln(os.Stderr, "ERROR: no valid tasks found in", *tasksDir)
			os.Exit(1)
		}
		fmt.Printf("OK: %d task(s) loaded successfully\n", len(tasks))
		for _, task := range tasks {
			fmt.Printf("  \u2713 %s (%s)\n", task.ID, task.Name)
		}
		return
	}

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
		fmt.Println("       go run ./evals/runner.go -validate")
		fmt.Println("       go run ./evals/runner.go -smoke -run <task-id>  (static checks only)")
		os.Exit(1)
	}

	toRun := eval.FilterTasks(tasks, *runID)
	if len(toRun) == 0 {
		fmt.Fprintf(os.Stderr, "Task %q not found\n", *runID)
		os.Exit(1)
	}

	if *smoke {
		results := eval.RunSmokeChecks(toRun)
		eval.PrintResults(results)
		if !eval.AllPassed(results) {
			os.Exit(1)
		}
		return
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

	results := eval.RunTasks(binPath, toRun)
	eval.PrintResults(results)
	if !eval.AllPassed(results) {
		os.Exit(1)
	}
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
