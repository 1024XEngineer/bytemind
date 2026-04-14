package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func runRunLoop(args []string) error {
	fs := flag.NewFlagSet("run-loop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	evalSetPath := fs.String("eval-set", "", "Eval set JSON file")
	skillPath := fs.String("skill-path", "", "Skill directory")
	description := fs.String("description", "", "Override starting description")
	numWorkers := fs.Int("num-workers", 10, "Parallel workers")
	timeout := fs.Int("timeout", 30, "Timeout seconds per query")
	maxIterations := fs.Int("max-iterations", 5, "Max iterations")
	runsPerQuery := fs.Int("runs-per-query", 3, "Runs per query")
	triggerThreshold := fs.Float64("trigger-threshold", 0.5, "Trigger threshold")
	holdout := fs.Float64("holdout", 0.4, "Holdout fraction")
	model := fs.String("model", "", "Model for improvement")
	verbose := fs.Bool("verbose", false, "Verbose logs")
	report := fs.String("report", "auto", "Report path: auto|none|<path>")
	resultsDir := fs.String("results-dir", "", "Save outputs to timestamped subdir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*evalSetPath) == "" || strings.TrimSpace(*skillPath) == "" || strings.TrimSpace(*model) == "" {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools run-loop --eval-set <file> --skill-path <dir> --model <model>")
	}

	var evalSet []evalQuery
	if err := readJSONFile(*evalSetPath, &evalSet); err != nil {
		return err
	}
	doc, err := parseSkillMDDir(*skillPath)
	if err != nil {
		return err
	}
	currentDescription := doc.Description
	if strings.TrimSpace(*description) != "" {
		currentDescription = *description
	}

	var liveReportPath string
	if *report != "none" {
		if *report == "auto" {
			name := fmt.Sprintf("skill_description_report_%s_%s.html", filepath.Base(*skillPath), time.Now().Format("20060102_150405"))
			liveReportPath = filepath.Join(os.TempDir(), name)
		} else {
			liveReportPath = mustAbs(*report)
		}
		_ = os.WriteFile(liveReportPath, []byte("<html><body><h1>Starting optimization loop...</h1><meta http-equiv='refresh' content='5'></body></html>"), 0o644)
		openBrowser(liveReportPath)
	}

	var resultsRoot string
	if strings.TrimSpace(*resultsDir) != "" {
		resultsRoot = filepath.Join(mustAbs(*resultsDir), time.Now().Format("2006-01-02_150405"))
		if err := os.MkdirAll(resultsRoot, 0o755); err != nil {
			return err
		}
	}
	logDir := ""
	if resultsRoot != "" {
		logDir = filepath.Join(resultsRoot, "logs")
	}

	output, err := runOptimizationLoop(evalSet, doc, currentDescription, *numWorkers, *timeout, *maxIterations, *runsPerQuery, *triggerThreshold, *holdout, *model, *verbose, liveReportPath, logDir)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return err
	}

	if resultsRoot != "" {
		if err := writeJSONFile(filepath.Join(resultsRoot, "results.json"), output); err != nil {
			return err
		}
	}

	if liveReportPath != "" {
		finalHTML := renderOptimizationReportHTML(output, false, doc.Name)
		_ = os.WriteFile(liveReportPath, []byte(finalHTML), 0o644)
		fmt.Fprintf(os.Stderr, "\nReport: %s\n", liveReportPath)
		if resultsRoot != "" {
			_ = os.WriteFile(filepath.Join(resultsRoot, "report.html"), []byte(finalHTML), 0o644)
		}
	}
	if resultsRoot != "" {
		fmt.Fprintf(os.Stderr, "Results saved to: %s\n", resultsRoot)
	}
	return nil
}

func runOptimizationLoop(evalSet []evalQuery, doc skillDoc, currentDescription string, numWorkers, timeout, maxIterations, runsPerQuery int, triggerThreshold, holdout float64, model string, verbose bool, liveReportPath, logDir string) (map[string]any, error) {
	trainSet, testSet := splitEvalSet(evalSet, holdout, 42)
	if holdout <= 0 {
		trainSet = evalSet
		testSet = nil
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Split: %d train, %d test (holdout=%.2f)\n", len(trainSet), len(testSet), holdout)
	}

	history := make([]map[string]any, 0)
	exitReason := "unknown"
	projectRoot := findProjectRootWithClaudeDir()

	for iteration := 1; iteration <= maxIterations; iteration++ {
		if verbose {
			fmt.Fprintf(os.Stderr, "\n%s\nIteration %d/%d\nDescription: %s\n%s\n", strings.Repeat("=", 60), iteration, maxIterations, currentDescription, strings.Repeat("=", 60))
		}

		allQueries := append([]evalQuery{}, trainSet...)
		allQueries = append(allQueries, testSet...)
		start := time.Now()
		allResults, err := executeEval(allQueries, doc.Name, currentDescription, numWorkers, timeout, runsPerQuery, triggerThreshold, projectRoot, model)
		if err != nil {
			return nil, err
		}
		evalElapsed := time.Since(start)

		trainQuerySet := map[string]struct{}{}
		for _, q := range trainSet {
			trainQuerySet[q.Query] = struct{}{}
		}
		trainResults := make([]map[string]any, 0)
		testResults := make([]map[string]any, 0)
		for _, r := range allResults.Results {
			entry := map[string]any{
				"query":          r.Query,
				"should_trigger": r.ShouldTrigger,
				"trigger_rate":   r.TriggerRate,
				"triggers":       r.Triggers,
				"runs":           r.Runs,
				"pass":           r.Pass,
			}
			if _, ok := trainQuerySet[r.Query]; ok {
				trainResults = append(trainResults, entry)
			} else {
				testResults = append(testResults, entry)
			}
		}
		sort.Slice(trainResults, func(i, j int) bool { return asString(trainResults[i]["query"]) < asString(trainResults[j]["query"]) })
		sort.Slice(testResults, func(i, j int) bool { return asString(testResults[i]["query"]) < asString(testResults[j]["query"]) })

		trainPassed := countPassed(trainResults)
		trainTotal := len(trainResults)
		testPassed := countPassed(testResults)
		testTotal := len(testResults)

		historyEntry := map[string]any{
			"iteration":     iteration,
			"description":   currentDescription,
			"train_passed":  trainPassed,
			"train_failed":  trainTotal - trainPassed,
			"train_total":   trainTotal,
			"train_results": trainResults,
			"test_passed":   nil,
			"test_failed":   nil,
			"test_total":    nil,
			"test_results":  nil,
			"passed":        trainPassed,
			"failed":        trainTotal - trainPassed,
			"total":         trainTotal,
			"results":       trainResults,
		}
		if len(testSet) > 0 {
			historyEntry["test_passed"] = testPassed
			historyEntry["test_failed"] = testTotal - testPassed
			historyEntry["test_total"] = testTotal
			historyEntry["test_results"] = testResults
		}
		history = append(history, historyEntry)

		if liveReportPath != "" {
			partial := map[string]any{
				"original_description": doc.Description,
				"best_description":     currentDescription,
				"best_score":           "in progress",
				"iterations_run":       len(history),
				"holdout":              holdout,
				"train_size":           len(trainSet),
				"test_size":            len(testSet),
				"history":              history,
			}
			_ = os.WriteFile(liveReportPath, []byte(renderOptimizationReportHTML(partial, true, doc.Name)), 0o644)
		}

		if verbose {
			printEvalStats("Train", trainResults, evalElapsed)
			if len(testSet) > 0 {
				printEvalStats("Test ", testResults, 0)
			}
		}

		if trainTotal-trainPassed == 0 {
			exitReason = fmt.Sprintf("all_passed (iteration %d)", iteration)
			if verbose {
				fmt.Fprintf(os.Stderr, "\nAll train queries passed on iteration %d!\n", iteration)
			}
			break
		}
		if iteration == maxIterations {
			exitReason = fmt.Sprintf("max_iterations (%d)", maxIterations)
			if verbose {
				fmt.Fprintf(os.Stderr, "\nMax iterations reached (%d).\n", maxIterations)
			}
			break
		}

		if verbose {
			fmt.Fprintln(os.Stderr, "\nImproving description...")
		}
		improveStart := time.Now()
		blindedHistory := make([]map[string]any, 0, len(history))
		for _, h := range history {
			item := make(map[string]any)
			for k, v := range h {
				if strings.HasPrefix(k, "test_") {
					continue
				}
				item[k] = v
			}
			blindedHistory = append(blindedHistory, item)
		}
		newDesc, _, err := improveDescription(improveParams{
			SkillName:          doc.Name,
			SkillContent:       doc.Content,
			CurrentDescription: currentDescription,
			EvalResults: map[string]any{
				"results": trainResults,
				"summary": map[string]any{"passed": trainPassed, "failed": trainTotal - trainPassed, "total": trainTotal},
			},
			History:   blindedHistory,
			Model:     model,
			LogDir:    logDir,
			Iteration: iteration,
		})
		if err != nil {
			return nil, err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Proposed (%.1fs): %s\n", time.Since(improveStart).Seconds(), newDesc)
		}
		currentDescription = newDesc
	}

	best := map[string]any{}
	if len(history) == 0 {
		best = map[string]any{"description": currentDescription, "iteration": 0, "train_passed": 0, "train_total": 0}
	} else if len(testSet) > 0 {
		best = history[0]
		for _, h := range history[1:] {
			if int(asFloat(h["test_passed"])) > int(asFloat(best["test_passed"])) {
				best = h
			}
		}
	} else {
		best = history[0]
		for _, h := range history[1:] {
			if int(asFloat(h["train_passed"])) > int(asFloat(best["train_passed"])) {
				best = h
			}
		}
	}

	bestScore := fmt.Sprintf("%d/%d", int(asFloat(best["train_passed"])), int(asFloat(best["train_total"])))
	bestTestScore := any(nil)
	if len(testSet) > 0 {
		bestScore = fmt.Sprintf("%d/%d", int(asFloat(best["test_passed"])), int(asFloat(best["test_total"])))
		bestTestScore = bestScore
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "\nExit reason: %s\n", exitReason)
		fmt.Fprintf(os.Stderr, "Best score: %s (iteration %d)\n", bestScore, int(asFloat(best["iteration"])))
	}

	output := map[string]any{
		"exit_reason":          exitReason,
		"original_description": doc.Description,
		"best_description":     asString(best["description"]),
		"best_score":           bestScore,
		"best_train_score":     fmt.Sprintf("%d/%d", int(asFloat(best["train_passed"])), int(asFloat(best["train_total"]))),
		"best_test_score":      bestTestScore,
		"final_description":    currentDescription,
		"iterations_run":       len(history),
		"holdout":              holdout,
		"train_size":           len(trainSet),
		"test_size":            len(testSet),
		"history":              history,
	}
	return output, nil
}

func splitEvalSet(evalSet []evalQuery, holdout float64, seed int64) (trainSet, testSet []evalQuery) {
	if holdout <= 0 || len(evalSet) == 0 {
		return append([]evalQuery{}, evalSet...), nil
	}
	trigger := make([]evalQuery, 0)
	noTrigger := make([]evalQuery, 0)
	for _, e := range evalSet {
		if e.ShouldTrigger {
			trigger = append(trigger, e)
		} else {
			noTrigger = append(noTrigger, e)
		}
	}
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(trigger), func(i, j int) { trigger[i], trigger[j] = trigger[j], trigger[i] })
	rng.Shuffle(len(noTrigger), func(i, j int) { noTrigger[i], noTrigger[j] = noTrigger[j], noTrigger[i] })

	nTrigTest := int(float64(len(trigger)) * holdout)
	nNoTrigTest := int(float64(len(noTrigger)) * holdout)
	if len(trigger) > 0 && nTrigTest < 1 {
		nTrigTest = 1
	}
	if len(noTrigger) > 0 && nNoTrigTest < 1 {
		nNoTrigTest = 1
	}
	if nTrigTest > len(trigger) {
		nTrigTest = len(trigger)
	}
	if nNoTrigTest > len(noTrigger) {
		nNoTrigTest = len(noTrigger)
	}
	testSet = append(testSet, trigger[:nTrigTest]...)
	testSet = append(testSet, noTrigger[:nNoTrigTest]...)
	trainSet = append(trainSet, trigger[nTrigTest:]...)
	trainSet = append(trainSet, noTrigger[nNoTrigTest:]...)
	return trainSet, testSet
}

func countPassed(results []map[string]any) int {
	count := 0
	for _, r := range results {
		if asBoolDefault(r["pass"], false) {
			count++
		}
	}
	return count
}

func printEvalStats(label string, results []map[string]any, elapsed time.Duration) {
	pos := make([]map[string]any, 0)
	neg := make([]map[string]any, 0)
	for _, r := range results {
		if asBoolDefault(r["should_trigger"], true) {
			pos = append(pos, r)
		} else {
			neg = append(neg, r)
		}
	}
	tp := 0
	posRuns := 0
	for _, r := range pos {
		tp += int(asFloat(r["triggers"]))
		posRuns += int(asFloat(r["runs"]))
	}
	fn := posRuns - tp
	fp := 0
	negRuns := 0
	for _, r := range neg {
		fp += int(asFloat(r["triggers"]))
		negRuns += int(asFloat(r["runs"]))
	}
	tn := negRuns - fp
	total := tp + tn + fp + fn
	precision := 1.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 1.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	accuracy := 0.0
	if total > 0 {
		accuracy = float64(tp+tn) / float64(total)
	}
	fmt.Fprintf(os.Stderr, "%s: %d/%d correct, precision=%.0f%% recall=%.0f%% accuracy=%.0f%% (%.1fs)\n", label, tp+tn, total, precision*100, recall*100, accuracy*100, elapsed.Seconds())
	for _, r := range results {
		status := "FAIL"
		if asBoolDefault(r["pass"], false) {
			status = "PASS"
		}
		fmt.Fprintf(os.Stderr, "  [%s] rate=%d/%d expected=%v: %s\n", status, int(asFloat(r["triggers"])), int(asFloat(r["runs"])), asBoolDefault(r["should_trigger"], true), truncateString(asString(r["query"]), 60))
	}
}
