package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadTasks(dir string) []EvalTask {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot read tasks dir %s: %v\n", dir, err)
		return []EvalTask{}
	}
	tasks := make([]EvalTask, 0, len(entries))
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

func FilterTasks(tasks []EvalTask, runID string) []EvalTask {
	if runID == "all" {
		return tasks
	}
	var filtered []EvalTask
	for _, t := range tasks {
		if t.ID == runID {
			filtered = append(filtered, t)
			break
		}
	}
	return filtered
}
