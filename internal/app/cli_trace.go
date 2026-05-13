package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/storage"
)

func RunTrace(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stdout, "ByteMind Trace\n\n")
		fmt.Fprintf(stdout, "Usage:\n")
		fmt.Fprintf(stdout, "  bytemind trace list                   List recent trace sessions\n")
		fmt.Fprintf(stdout, "  bytemind trace show <run_id>         Show trace details\n")
		fmt.Fprintf(stdout, "  bytemind trace export <run_id>       Export trace as JSON\n")
		fmt.Fprintf(stdout, "  bytemind trace export <run_id> --format markdown  Export as Markdown\n")
		return nil
	}

	subcommand := args[0]
	switch subcommand {
	case "list":
		return traceList(stdout, stderr)
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: bytemind trace show <run_id>")
		}
		return traceShow(args[1], stdout, stderr)
	case "export":
		if len(args) < 2 {
			return fmt.Errorf("usage: bytemind trace export <run_id> [--format json|markdown]")
		}
		format := "json"
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--format", "-f":
				if i+1 < len(args) {
					format = args[i+1]
					i++
				}
			}
		}
		return traceExport(args[1], format, stdout, stderr)
	default:
		return fmt.Errorf("unknown trace subcommand: %s (expected list, show, export)", subcommand)
	}
}

func traceList(w io.Writer, stderr io.Writer) error {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return err
	}
	auditDir := filepath.Join(home, "audit")
	if _, err := os.Stat(auditDir); os.IsNotExist(err) {
		fmt.Fprintln(w, "No trace data found.")
		return nil
	}

	entries, err := os.ReadDir(auditDir)
	if err != nil {
		return fmt.Errorf("read audit dir: %w", err)
	}

	fmt.Fprintf(w, "Recent trace sessions:\n\n")
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			dateStr := strings.TrimSuffix(entry.Name(), ".jsonl")
			// Count events in this file
			count := countLines(filepath.Join(auditDir, entry.Name()))
			fmt.Fprintf(w, "  %s  %d event(s)\n", dateStr, count)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use 'bytemind trace show <run_id>' for details.")
	return nil
}

func traceShow(runID string, w io.Writer, stderr io.Writer) error {
	events, err := loadEventsForRun(runID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Fprintf(w, "No trace found for run_id %q.\n", runID)
		return nil
	}

	fmt.Fprintf(w, "Trace: %s\n", runID)
	fmt.Fprintf(w, "Events: %d\n\n", len(events))

	for _, ev := range events {
		ts := ev.Timestamp.Format(time.RFC3339)
		action := ev.Action
		result := ev.Result
		actor := ev.Actor

		switch action {
		case "tool_call":
			toolName := ev.Metadata["tool_name"]
			fmt.Fprintf(w, "  [%s] %s called %s -> %s\n", ts, actor, toolName, result)
		case "task_state_changed":
			toolName := ev.Metadata["tool_name"]
			fmt.Fprintf(w, "  [%s] task %s (%s) -> %s\n", ts, ev.TaskID, toolName, result)
		default:
			metaStr := ""
			if ev.Metadata != nil && len(ev.Metadata) > 0 {
				parts := make([]string, 0, len(ev.Metadata))
				for k, v := range ev.Metadata {
					parts = append(parts, k+"="+v)
				}
				metaStr = " [" + strings.Join(parts, " ") + "]"
			}
			fmt.Fprintf(w, "  [%s] %s/%s -> %s%s\n", ts, actor, action, result, metaStr)
		}
	}
	return nil
}

func traceExport(runID, format string, w io.Writer, stderr io.Writer) error {
	events, err := loadEventsForRun(runID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("no trace found for run_id %q", runID)
	}

	switch format {
	case "json", "JSON":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(events)
	case "markdown", "md":
		fmt.Fprintf(w, "# Agent Trace: %s\n\n", runID)
		fmt.Fprintf(w, "| Time | Actor | Action | Result | Details |\n")
		fmt.Fprintf(w, "|------|-------|--------|--------|--------|\n")
		for _, ev := range events {
			ts := ev.Timestamp.Format(time.RFC3339)
			action := ev.Action
			result := ev.Result
			actor := ev.Actor
			metaStr := ""
			if ev.Metadata != nil {
				parts := make([]string, 0, len(ev.Metadata))
				for k, v := range ev.Metadata {
					parts = append(parts, k+"="+v)
				}
				if len(parts) > 0 {
					metaStr = strings.Join(parts, ", ")
				}
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n", ts, actor, action, result, metaStr)
		}
		return nil
	default:
		return fmt.Errorf("unsupported format: %s (expected json or markdown)", format)
	}
}

func loadEventsForRun(runID string) ([]storage.AuditEvent, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return nil, err
	}
	auditDir := filepath.Join(home, "audit")
	if _, err := os.Stat(auditDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(auditDir)
	if err != nil {
		return nil, err
	}

	var events []storage.AuditEvent
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		fileEvents, err := readAuditFile(filepath.Join(auditDir, entry.Name()), runID)
		if err != nil {
			continue
		}
		events = append(events, fileEvents...)
	}
	return events, nil
}

func readAuditFile(path, runID string) ([]storage.AuditEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []storage.AuditEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev storage.AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if runID == "all" || string(ev.TraceID) == runID || string(ev.SessionID) == runID || string(ev.EventID) == runID {
			events = append(events, ev)
		}
	}
	return events, scanner.Err()
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}
