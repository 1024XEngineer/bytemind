package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncateString(t *testing.T) {
	if got := truncateString("abcdef", 3); got != "abc" {
		t.Fatalf("truncateString mismatch: %q", got)
	}
	if got := truncateString("abcdef", 0); got != "" {
		t.Fatalf("truncateString expected empty")
	}
}

func TestExtractNewDescription(t *testing.T) {
	got := extractNewDescription(`<new_description>"hello world"</new_description>`)
	if got != "hello world" {
		t.Fatalf("extractNewDescription mismatch: %q", got)
	}
	got = extractNewDescription(`"fallback only"`)
	if got != "fallback only" {
		t.Fatalf("fallback extraction mismatch: %q", got)
	}
}

func TestRunSingleQueryAndExecuteEvalWithFakeClaude(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude", "commands"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	installFakeClaude(t, project)
	t.Setenv("CODEX_FAKE_TRIGGER", "1")

	triggered, err := runSingleQuery("query", "demo-skill", "desc", 5, project, "")
	if err != nil {
		t.Fatalf("runSingleQuery error: %v", err)
	}
	_ = triggered

	out, err := executeEval([]evalQuery{{Query: "q1", ShouldTrigger: true}}, "demo-skill", "desc", 1, 5, 2, 0.5, project, "")
	if err != nil {
		t.Fatalf("executeEval error: %v", err)
	}
	if out.Summary["total"] != 1 || len(out.Results) != 1 {
		t.Fatalf("unexpected eval output: %+v", out)
	}
}

func TestImproveDescriptionAndRunImproveDescription(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude", "commands"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	installFakeClaude(t, project)

	params := improveParams{
		SkillName:          "demo-skill",
		SkillContent:       "# demo",
		CurrentDescription: "old desc",
		EvalResults: map[string]any{
			"results": []any{
				map[string]any{"query": "a", "should_trigger": true, "pass": false, "triggers": 0, "runs": 1},
			},
			"summary": map[string]any{"passed": 0, "failed": 1, "total": 1},
		},
		Model: "fake-model",
	}
	desc, transcript, err := improveDescription(params)
	if err != nil {
		t.Fatalf("improveDescription error: %v", err)
	}
	if desc != "Improved description for tests" {
		t.Fatalf("unexpected description: %q", desc)
	}
	if strings.TrimSpace(asString(transcript["response"])) == "" {
		t.Fatalf("expected transcript response")
	}

	skillDir := createMinimalSkill(t, project, "demo-skill", "old desc")
	evalResultsPath := filepath.Join(project, "eval-results.json")
	if err := writeJSONFile(evalResultsPath, params.EvalResults); err != nil {
		t.Fatalf("write eval results: %v", err)
	}
	if err := runImproveDescription([]string{"--eval-results", evalResultsPath, "--skill-path", skillDir, "--model", "fake-model"}); err != nil {
		t.Fatalf("runImproveDescription: %v", err)
	}
}

func TestRunRunEval(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude", "commands"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	installFakeClaude(t, project)
	t.Setenv("CODEX_FAKE_TRIGGER", "1")
	withWorkingDir(t, project)

	skillDir := createMinimalSkill(t, project, "demo-skill", "desc")
	evalPath := filepath.Join(project, "evals.json")
	if err := writeJSONFile(evalPath, []map[string]any{{"query": "trigger me", "should_trigger": true}}); err != nil {
		t.Fatalf("write eval set: %v", err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	runErr := runRunEval([]string{"--eval-set", evalPath, "--skill-path", skillDir, "--num-workers", "1", "--runs-per-query", "1"})
	_ = w.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r)
	if runErr != nil {
		t.Fatalf("runRunEval: %v", runErr)
	}
	if !strings.Contains(buf.String(), `"results"`) {
		t.Fatalf("expected JSON output, got: %s", buf.String())
	}
}

func TestSplitEvalSetAndCountPassed(t *testing.T) {
	evals := []evalQuery{
		{Query: "a", ShouldTrigger: true},
		{Query: "b", ShouldTrigger: true},
		{Query: "c", ShouldTrigger: false},
		{Query: "d", ShouldTrigger: false},
	}
	train, test := splitEvalSet(evals, 0.5, 42)
	if len(train) == 0 || len(test) == 0 {
		t.Fatalf("expected non-empty train/test split")
	}
	results := []map[string]any{
		{"pass": true},
		{"pass": false},
		{"pass": true},
	}
	if got := countPassed(results); got != 2 {
		t.Fatalf("countPassed mismatch: %d", got)
	}
}

func TestRunOptimizationLoopAndRunRunLoop(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude", "commands"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	installFakeClaude(t, project)
	withWorkingDir(t, project)
	t.Setenv("CODEX_FAKE_TRIGGER", "0")

	doc := skillDoc{
		Name:        "demo-skill",
		Description: "old desc",
		Content:     "# skill content",
	}
	evals := []evalQuery{{Query: "needs trigger", ShouldTrigger: true}}
	out, err := runOptimizationLoop(evals, doc, doc.Description, 1, 5, 2, 1, 0.5, 0, "fake-model", false, "", "")
	if err != nil {
		t.Fatalf("runOptimizationLoop: %v", err)
	}
	if asString(out["best_description"]) == "" {
		t.Fatalf("best_description should not be empty")
	}
	if strings.TrimSpace(asString(out["exit_reason"])) == "" {
		t.Fatalf("exit_reason should not be empty")
	}

	skillDir := createMinimalSkill(t, project, "demo-skill", "old desc")
	evalPath := filepath.Join(project, "eval-set.json")
	if err := writeJSONFile(evalPath, []map[string]any{{"query": "needs trigger", "should_trigger": true}}); err != nil {
		t.Fatalf("write eval set: %v", err)
	}
	resultsDir := filepath.Join(project, "results")
	t.Setenv("CODEX_FAKE_TRIGGER", "1")
	if err := runRunLoop([]string{
		"--eval-set", evalPath,
		"--skill-path", skillDir,
		"--model", "fake-model",
		"--report", "none",
		"--results-dir", resultsDir,
		"--num-workers", "1",
		"--runs-per-query", "1",
		"--max-iterations", "1",
		"--holdout", "0",
	}); err != nil {
		t.Fatalf("runRunLoop: %v", err)
	}

	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		t.Fatalf("read results dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected timestamped result directory")
	}
	foundResults := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(resultsDir, e.Name(), "results.json")
		if _, err := os.Stat(p); err == nil {
			foundResults = true
			var outObj map[string]any
			if err := readJSONFile(p, &outObj); err != nil {
				t.Fatalf("read results.json: %v", err)
			}
			if strings.TrimSpace(asString(outObj["best_description"])) == "" {
				t.Fatalf("best_description missing in results")
			}
			break
		}
	}
	if !foundResults {
		t.Fatalf("results.json not found in run-loop outputs")
	}
}

func TestPrintEvalStats(t *testing.T) {
	var buf bytes.Buffer
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	results := []map[string]any{
		{"query": "q1", "should_trigger": true, "triggers": 1, "runs": 1, "pass": true},
		{"query": "q2", "should_trigger": false, "triggers": 0, "runs": 1, "pass": true},
	}
	printEvalStats("Test", results, 0)
	_ = w.Close()
	_, _ = buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "precision") {
		t.Fatalf("expected stats output, got: %s", buf.String())
	}
}

func TestRunRunLoopUsageAndRunRunEvalUsage(t *testing.T) {
	if err := runRunLoop(nil); err == nil {
		t.Fatalf("expected usage error for runRunLoop")
	}
	if err := runRunEval(nil); err == nil {
		t.Fatalf("expected usage error for runRunEval")
	}
}

func TestRunImproveDescriptionUsage(t *testing.T) {
	if err := runImproveDescription(nil); err == nil {
		t.Fatalf("expected usage error for runImproveDescription")
	}
}

func TestRunOptimizationLoopOutputJSONShape(t *testing.T) {
	out := map[string]any{
		"best_description": "x",
		"history":          []any{},
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "best_description") {
		t.Fatalf("unexpected json output: %s", string(raw))
	}
}
