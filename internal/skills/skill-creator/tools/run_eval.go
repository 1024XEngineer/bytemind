package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type evalQuery struct {
	Query         string `json:"query"`
	ShouldTrigger bool   `json:"should_trigger"`
}

type evalResult struct {
	Query         string  `json:"query"`
	ShouldTrigger bool    `json:"should_trigger"`
	TriggerRate   float64 `json:"trigger_rate"`
	Triggers      int     `json:"triggers"`
	Runs          int     `json:"runs"`
	Pass          bool    `json:"pass"`
}

type evalOutput struct {
	SkillName   string         `json:"skill_name"`
	Description string         `json:"description"`
	Results     []evalResult   `json:"results"`
	Summary     map[string]int `json:"summary"`
}

func runRunEval(args []string) error {
	fs := flag.NewFlagSet("run-eval", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	evalSetPath := fs.String("eval-set", "", "Path to eval set JSON file")
	skillPath := fs.String("skill-path", "", "Path to skill directory")
	descriptionOverride := fs.String("description", "", "Override description")
	numWorkers := fs.Int("num-workers", 10, "Parallel workers")
	timeout := fs.Int("timeout", 30, "Timeout per query in seconds")
	runsPerQuery := fs.Int("runs-per-query", 3, "Runs per query")
	threshold := fs.Float64("trigger-threshold", 0.5, "Trigger threshold")
	model := fs.String("model", "", "Model for claude -p")
	verbose := fs.Bool("verbose", false, "Verbose logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*evalSetPath) == "" || strings.TrimSpace(*skillPath) == "" {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools run-eval --eval-set <file> --skill-path <dir> [--description ...]")
	}

	var evalSet []evalQuery
	if err := readJSONFile(*evalSetPath, &evalSet); err != nil {
		return err
	}
	doc, err := parseSkillMDDir(*skillPath)
	if err != nil {
		return err
	}
	description := doc.Description
	if strings.TrimSpace(*descriptionOverride) != "" {
		description = *descriptionOverride
	}
	projectRoot := findProjectRootWithClaudeDir()
	if *verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Evaluating description: %s\n", description)
	}

	out, err := executeEval(evalSet, doc.Name, description, *numWorkers, *timeout, *runsPerQuery, *threshold, projectRoot, *model)
	if err != nil {
		return err
	}

	if *verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Results: %d/%d passed\n", out.Summary["passed"], out.Summary["total"])
		for _, r := range out.Results {
			status := "FAIL"
			if r.Pass {
				status = "PASS"
			}
			_, _ = fmt.Fprintf(os.Stderr, "  [%s] rate=%d/%d expected=%v: %s\n", status, r.Triggers, r.Runs, r.ShouldTrigger, truncateString(r.Query, 70))
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func executeEval(evalSet []evalQuery, skillName, description string, numWorkers, timeoutSec, runsPerQuery int, triggerThreshold float64, projectRoot, model string) (evalOutput, error) {
	if numWorkers <= 0 {
		numWorkers = 1
	}
	if runsPerQuery <= 0 {
		runsPerQuery = 1
	}

	type runTask struct {
		Query evalQuery
	}
	type runOutcome struct {
		Query     evalQuery
		Triggered bool
		Err       error
	}

	tasks := make(chan runTask)
	results := make(chan runOutcome)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				triggered, err := runSingleQuery(task.Query.Query, skillName, description, timeoutSec, projectRoot, model)
				results <- runOutcome{Query: task.Query, Triggered: triggered, Err: err}
			}
		}()
	}

	go func() {
		for _, item := range evalSet {
			for i := 0; i < runsPerQuery; i++ {
				tasks <- runTask{Query: item}
			}
		}
		close(tasks)
		wg.Wait()
		close(results)
	}()

	triggerMap := make(map[string][]bool)
	queryMeta := make(map[string]evalQuery)
	for outcome := range results {
		queryMeta[outcome.Query.Query] = outcome.Query
		if _, ok := triggerMap[outcome.Query.Query]; !ok {
			triggerMap[outcome.Query.Query] = make([]bool, 0, runsPerQuery)
		}
		if outcome.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: query failed: %v\n", outcome.Err)
			triggerMap[outcome.Query.Query] = append(triggerMap[outcome.Query.Query], false)
			continue
		}
		triggerMap[outcome.Query.Query] = append(triggerMap[outcome.Query.Query], outcome.Triggered)
	}

	keys := make([]string, 0, len(triggerMap))
	for q := range triggerMap {
		keys = append(keys, q)
	}
	sort.Strings(keys)

	resultsList := make([]evalResult, 0, len(keys))
	passed := 0
	for _, query := range keys {
		triggers := triggerMap[query]
		countTrue := 0
		for _, t := range triggers {
			if t {
				countTrue++
			}
		}
		triggerRate := float64(countTrue) / float64(len(triggers))
		meta := queryMeta[query]
		didPass := false
		if meta.ShouldTrigger {
			didPass = triggerRate >= triggerThreshold
		} else {
			didPass = triggerRate < triggerThreshold
		}
		if didPass {
			passed++
		}
		resultsList = append(resultsList, evalResult{
			Query:         query,
			ShouldTrigger: meta.ShouldTrigger,
			TriggerRate:   triggerRate,
			Triggers:      countTrue,
			Runs:          len(triggers),
			Pass:          didPass,
		})
	}

	total := len(resultsList)
	return evalOutput{
		SkillName:   skillName,
		Description: description,
		Results:     resultsList,
		Summary: map[string]int{
			"total":  total,
			"passed": passed,
			"failed": total - passed,
		},
	}, nil
}

func runSingleQuery(query, skillName, skillDescription string, timeoutSec int, projectRoot, model string) (bool, error) {
	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	cleanName := fmt.Sprintf("%s-skill-%s", skillName, uniqueID)
	commandsDir := filepath.Join(projectRoot, ".claude", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return false, err
	}
	commandFile := filepath.Join(commandsDir, cleanName+".md")

	indented := strings.ReplaceAll(skillDescription, "\n", "\n  ")
	commandContent := fmt.Sprintf("---\ndescription: |\n  %s\n---\n\n# %s\n\nThis skill handles: %s\n", indented, skillName, skillDescription)
	if err := os.WriteFile(commandFile, []byte(commandContent), 0o644); err != nil {
		return false, err
	}
	defer os.Remove(commandFile)

	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	args := []string{"-p", query, "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return false, err
	}

	triggered := false
	pendingTool := ""
	accumulated := ""

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 128*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		eventType := asString(event["type"])
		if eventType == "stream_event" {
			se, _ := event["event"].(map[string]any)
			seType := asString(se["type"])
			switch seType {
			case "content_block_start":
				cb, _ := se["content_block"].(map[string]any)
				if asString(cb["type"]) == "tool_use" {
					toolName := asString(cb["name"])
					if toolName == "Skill" || toolName == "Read" {
						pendingTool = toolName
						accumulated = ""
					} else {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
						return false, nil
					}
				}
			case "content_block_delta":
				if pendingTool != "" {
					delta, _ := se["delta"].(map[string]any)
					if asString(delta["type"]) == "input_json_delta" {
						accumulated += asString(delta["partial_json"])
						if strings.Contains(accumulated, cleanName) {
							_ = cmd.Process.Kill()
							_ = cmd.Wait()
							return true, nil
						}
					}
				}
			case "content_block_stop", "message_stop":
				if pendingTool != "" {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					return strings.Contains(accumulated, cleanName), nil
				}
				if seType == "message_stop" {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					return false, nil
				}
			}
			continue
		}

		if eventType == "assistant" {
			msg, _ := event["message"].(map[string]any)
			content, _ := msg["content"].([]any)
			for _, raw := range content {
				item, ok := raw.(map[string]any)
				if !ok || asString(item["type"]) != "tool_use" {
					continue
				}
				toolName := asString(item["name"])
				input, _ := item["input"].(map[string]any)
				if toolName == "Skill" && strings.Contains(asString(input["skill"]), cleanName) {
					triggered = true
				}
				if toolName == "Read" && strings.Contains(asString(input["file_path"]), cleanName) {
					triggered = true
				}
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return triggered, nil
			}
		}
		if eventType == "result" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return triggered, nil
		}
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return false, err
	}
	waitErr := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return false, fmt.Errorf("timeout after %ds", timeoutSec)
	}
	if waitErr != nil {
		return false, waitErr
	}
	return triggered, nil
}

func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}
