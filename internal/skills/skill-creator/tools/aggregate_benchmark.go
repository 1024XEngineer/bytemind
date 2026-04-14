package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type stats struct {
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type runMetric struct {
	EvalID      int              `json:"eval_id"`
	RunNumber   int              `json:"run_number"`
	PassRate    float64          `json:"pass_rate"`
	Passed      int              `json:"passed"`
	Failed      int              `json:"failed"`
	Total       int              `json:"total"`
	TimeSeconds float64          `json:"time_seconds"`
	Tokens      float64          `json:"tokens"`
	ToolCalls   int              `json:"tool_calls"`
	Errors      int              `json:"errors"`
	Expects     []map[string]any `json:"expectations"`
	Notes       []string         `json:"notes"`
}

type benchMetadata struct {
	SkillName            string `json:"skill_name"`
	SkillPath            string `json:"skill_path"`
	ExecutorModel        string `json:"executor_model"`
	AnalyzerModel        string `json:"analyzer_model"`
	Timestamp            string `json:"timestamp"`
	EvalsRun             []int  `json:"evals_run"`
	RunsPerConfiguration int    `json:"runs_per_configuration"`
}

type benchmark struct {
	Metadata   benchMetadata    `json:"metadata"`
	Runs       []map[string]any `json:"runs"`
	RunSummary map[string]any   `json:"run_summary"`
	Notes      []string         `json:"notes"`
}

func runAggregateBenchmark(args []string) error {
	fs := flag.NewFlagSet("aggregate-benchmark", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	skillName := fs.String("skill-name", "", "Name of the skill")
	skillPath := fs.String("skill-path", "", "Path of the skill")
	output := fs.String("output", "", "Output benchmark.json path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools aggregate-benchmark <benchmark_dir> [--skill-name x] [--skill-path y] [--output z]")
	}
	benchmarkDir := mustAbs(fs.Arg(0))
	if info, err := os.Stat(benchmarkDir); err != nil || !info.IsDir() {
		return fmt.Errorf("directory not found: %s", benchmarkDir)
	}

	results, err := loadRunResults(benchmarkDir)
	if err != nil {
		return err
	}
	b := generateBenchmark(results, *skillName, *skillPath)

	outJSON := *output
	if strings.TrimSpace(outJSON) == "" {
		outJSON = filepath.Join(benchmarkDir, "benchmark.json")
	}
	if err := writeJSONFile(outJSON, b); err != nil {
		return err
	}
	fmt.Printf("Generated: %s\n", outJSON)

	outMD := strings.TrimSuffix(outJSON, filepath.Ext(outJSON)) + ".md"
	if err := os.WriteFile(outMD, []byte(generateBenchmarkMarkdown(b)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("Generated: %s\n", outMD)

	fmt.Println("\nSummary:")
	runSummary := b.RunSummary
	for k, v := range runSummary {
		if k == "delta" {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		pr := nestedFloat(m, "pass_rate", "mean")
		fmt.Printf("  %s: %.1f%% pass rate\n", strings.Title(strings.ReplaceAll(k, "_", " ")), pr*100)
	}
	if d, ok := runSummary["delta"].(map[string]any); ok {
		fmt.Printf("  Delta: %v\n", d["pass_rate"])
	}
	return nil
}

func loadRunResults(benchmarkDir string) (map[string][]runMetric, error) {
	searchDir := ""
	runsDir := filepath.Join(benchmarkDir, "runs")
	if info, err := os.Stat(runsDir); err == nil && info.IsDir() {
		searchDir = runsDir
	} else {
		matches, _ := filepath.Glob(filepath.Join(benchmarkDir, "eval-*"))
		if len(matches) > 0 {
			searchDir = benchmarkDir
		}
	}
	if searchDir == "" {
		return nil, fmt.Errorf("no eval directories found in %s or %s", benchmarkDir, runsDir)
	}

	evalDirs, err := filepath.Glob(filepath.Join(searchDir, "eval-*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(evalDirs)
	results := make(map[string][]runMetric)

	for idx, evalDir := range evalDirs {
		evalID := readEvalID(evalDir, idx)
		entries, err := os.ReadDir(evalDir)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			configDir := filepath.Join(evalDir, e.Name())
			runMatches, _ := filepath.Glob(filepath.Join(configDir, "run-*"))
			if len(runMatches) == 0 {
				continue
			}
			sort.Strings(runMatches)

			config := e.Name()
			for _, runDir := range runMatches {
				runNumber := parseRunNumber(filepath.Base(runDir))
				gradingPath := filepath.Join(runDir, "grading.json")
				var grading map[string]any
				if err := readJSONFile(gradingPath, &grading); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: grading.json not found/invalid in %s\n", runDir)
					continue
				}

				metric := runMetric{
					EvalID:      evalID,
					RunNumber:   runNumber,
					PassRate:    nestedFloat(grading, "summary", "pass_rate"),
					Passed:      int(nestedFloat(grading, "summary", "passed")),
					Failed:      int(nestedFloat(grading, "summary", "failed")),
					Total:       int(nestedFloat(grading, "summary", "total")),
					TimeSeconds: nestedFloat(grading, "timing", "total_duration_seconds"),
					Tokens:      nestedFloat(grading, "execution_metrics", "output_chars"),
					ToolCalls:   int(nestedFloat(grading, "execution_metrics", "total_tool_calls")),
					Errors:      int(nestedFloat(grading, "execution_metrics", "errors_encountered")),
				}

				if metric.TimeSeconds == 0 {
					var timing map[string]any
					if err := readJSONFile(filepath.Join(runDir, "timing.json"), &timing); err == nil {
						metric.TimeSeconds = nestedFloat(timing, "total_duration_seconds")
						metric.Tokens = nestedFloat(timing, "total_tokens")
					}
				}

				if rawExpects, ok := grading["expectations"].([]any); ok {
					for _, raw := range rawExpects {
						if obj, ok := raw.(map[string]any); ok {
							metric.Expects = append(metric.Expects, obj)
							if _, ok := obj["text"]; !ok {
								fmt.Fprintf(os.Stderr, "Warning: expectation in %s missing text/passed/evidence fields\n", gradingPath)
							}
						}
					}
				}

				notesSummary, _ := grading["user_notes_summary"].(map[string]any)
				metric.Notes = append(metric.Notes, anyStringSlice(notesSummary["uncertainties"])...)
				metric.Notes = append(metric.Notes, anyStringSlice(notesSummary["needs_review"])...)
				metric.Notes = append(metric.Notes, anyStringSlice(notesSummary["workarounds"])...)

				results[config] = append(results[config], metric)
			}
		}
	}
	return results, nil
}

func generateBenchmark(results map[string][]runMetric, skillName, skillPath string) benchmark {
	summary := aggregateRunSummary(results)
	var runs []map[string]any
	evalIDSet := map[int]struct{}{}
	for config, list := range results {
		for _, r := range list {
			evalIDSet[r.EvalID] = struct{}{}
			runs = append(runs, map[string]any{
				"eval_id":       r.EvalID,
				"configuration": config,
				"run_number":    r.RunNumber,
				"result": map[string]any{
					"pass_rate":    r.PassRate,
					"passed":       r.Passed,
					"failed":       r.Failed,
					"total":        r.Total,
					"time_seconds": r.TimeSeconds,
					"tokens":       int(r.Tokens),
					"tool_calls":   r.ToolCalls,
					"errors":       r.Errors,
				},
				"expectations": r.Expects,
				"notes":        r.Notes,
			})
		}
	}
	var evals []int
	for id := range evalIDSet {
		evals = append(evals, id)
	}
	sort.Ints(evals)

	return benchmark{
		Metadata: benchMetadata{
			SkillName:            defaultString(skillName, "<skill-name>"),
			SkillPath:            defaultString(skillPath, "<path/to/skill>"),
			ExecutorModel:        "<model-name>",
			AnalyzerModel:        "<model-name>",
			Timestamp:            time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			EvalsRun:             evals,
			RunsPerConfiguration: 3,
		},
		Runs:       runs,
		RunSummary: summary,
		Notes:      []string{},
	}
}

func aggregateRunSummary(results map[string][]runMetric) map[string]any {
	out := make(map[string]any)
	configs := make([]string, 0, len(results))
	for config := range results {
		configs = append(configs, config)
	}
	sort.Strings(configs)

	for _, config := range configs {
		list := results[config]
		if len(list) == 0 {
			out[config] = map[string]any{"pass_rate": stats{}, "time_seconds": stats{}, "tokens": stats{}}
			continue
		}
		passRates := make([]float64, 0, len(list))
		times := make([]float64, 0, len(list))
		tokens := make([]float64, 0, len(list))
		for _, r := range list {
			passRates = append(passRates, r.PassRate)
			times = append(times, r.TimeSeconds)
			tokens = append(tokens, r.Tokens)
		}
		out[config] = map[string]any{
			"pass_rate":    calcStats(passRates),
			"time_seconds": calcStats(times),
			"tokens":       calcStats(tokens),
		}
	}

	primary := map[string]any{}
	baseline := map[string]any{}
	if len(configs) >= 1 {
		primary, _ = out[configs[0]].(map[string]any)
	}
	if len(configs) >= 2 {
		baseline, _ = out[configs[1]].(map[string]any)
	}
	deltaPassRate := nestedFloat(primary, "pass_rate", "mean") - nestedFloat(baseline, "pass_rate", "mean")
	deltaTime := nestedFloat(primary, "time_seconds", "mean") - nestedFloat(baseline, "time_seconds", "mean")
	deltaTokens := nestedFloat(primary, "tokens", "mean") - nestedFloat(baseline, "tokens", "mean")

	out["delta"] = map[string]any{
		"pass_rate":    fmt.Sprintf("%+.2f", deltaPassRate),
		"time_seconds": fmt.Sprintf("%+.1f", deltaTime),
		"tokens":       fmt.Sprintf("%+.0f", deltaTokens),
	}
	return out
}

func calcStats(values []float64) stats {
	if len(values) == 0 {
		return stats{}
	}
	n := float64(len(values))
	sum := 0.0
	minVal := values[0]
	maxVal := values[0]
	for _, v := range values {
		sum += v
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	mean := sum / n
	stddev := 0.0
	if len(values) > 1 {
		variance := 0.0
		for _, v := range values {
			variance += (v - mean) * (v - mean)
		}
		variance /= float64(len(values) - 1)
		stddev = math.Sqrt(variance)
	}
	return stats{
		Mean:   round4(mean),
		Stddev: round4(stddev),
		Min:    round4(minVal),
		Max:    round4(maxVal),
	}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func readEvalID(evalDir string, fallback int) int {
	metaPath := filepath.Join(evalDir, "eval_metadata.json")
	var meta map[string]any
	if err := readJSONFile(metaPath, &meta); err == nil {
		return int(nestedFloat(meta, "eval_id"))
	}
	parts := strings.Split(filepath.Base(evalDir), "-")
	if len(parts) >= 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			return n
		}
	}
	return fallback
}

func parseRunNumber(name string) int {
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			return n
		}
	}
	return 0
}

func nestedFloat(obj map[string]any, keys ...string) float64 {
	if obj == nil {
		return 0
	}
	var curr any = obj
	for _, key := range keys {
		m, ok := curr.(map[string]any)
		if !ok {
			return 0
		}
		curr, ok = m[key]
		if !ok {
			return 0
		}
	}
	switch v := curr.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
	}
}

func anyStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func generateBenchmarkMarkdown(b benchmark) string {
	runSummary := b.RunSummary
	configs := make([]string, 0)
	for key := range runSummary {
		if key != "delta" {
			configs = append(configs, key)
		}
	}
	sort.Strings(configs)
	configA := "config_a"
	configB := "config_b"
	if len(configs) >= 1 {
		configA = configs[0]
	}
	if len(configs) >= 2 {
		configB = configs[1]
	}
	labelA := strings.Title(strings.ReplaceAll(configA, "_", " "))
	labelB := strings.Title(strings.ReplaceAll(configB, "_", " "))

	aSummary, _ := runSummary[configA].(map[string]any)
	bSummary, _ := runSummary[configB].(map[string]any)
	delta, _ := runSummary["delta"].(map[string]any)

	lines := []string{
		fmt.Sprintf("# Skill Benchmark: %s", b.Metadata.SkillName),
		"",
		fmt.Sprintf("**Model**: %s", b.Metadata.ExecutorModel),
		fmt.Sprintf("**Date**: %s", b.Metadata.Timestamp),
		fmt.Sprintf("**Evals**: %s (%d runs each per configuration)", joinInts(b.Metadata.EvalsRun), b.Metadata.RunsPerConfiguration),
		"",
		"## Summary",
		"",
		fmt.Sprintf("| Metric | %s | %s | Delta |", labelA, labelB),
		"|--------|------------|---------------|-------|",
		fmt.Sprintf("| Pass Rate | %.0f%% ± %.0f%% | %.0f%% ± %.0f%% | %v |",
			nestedFloat(aSummary, "pass_rate", "mean")*100,
			nestedFloat(aSummary, "pass_rate", "stddev")*100,
			nestedFloat(bSummary, "pass_rate", "mean")*100,
			nestedFloat(bSummary, "pass_rate", "stddev")*100,
			delta["pass_rate"],
		),
		fmt.Sprintf("| Time | %.1fs ± %.1fs | %.1fs ± %.1fs | %vs |",
			nestedFloat(aSummary, "time_seconds", "mean"),
			nestedFloat(aSummary, "time_seconds", "stddev"),
			nestedFloat(bSummary, "time_seconds", "mean"),
			nestedFloat(bSummary, "time_seconds", "stddev"),
			delta["time_seconds"],
		),
		fmt.Sprintf("| Tokens | %.0f ± %.0f | %.0f ± %.0f | %v |",
			nestedFloat(aSummary, "tokens", "mean"),
			nestedFloat(aSummary, "tokens", "stddev"),
			nestedFloat(bSummary, "tokens", "mean"),
			nestedFloat(bSummary, "tokens", "stddev"),
			delta["tokens"],
		),
	}
	if len(b.Notes) > 0 {
		lines = append(lines, "", "## Notes", "")
		for _, note := range b.Notes {
			lines = append(lines, "- "+note)
		}
	}
	return strings.Join(lines, "\n")
}

func joinInts(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ", ")
}

func requireJSONMap(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("not an object")
	}
	return m, nil
}
