package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGradingFixture(t *testing.T, runDir string, passRate float64, passed, failed, total int) {
	t.Helper()
	grading := map[string]any{
		"summary": map[string]any{
			"pass_rate": passRate,
			"passed":    passed,
			"failed":    failed,
			"total":     total,
		},
		"timing": map[string]any{
			"total_duration_seconds": 1.25,
		},
		"execution_metrics": map[string]any{
			"output_chars":       123,
			"total_tool_calls":   2,
			"errors_encountered": 0,
		},
		"expectations": []any{
			map[string]any{"text": "has text", "passed": true, "evidence": "ok"},
		},
		"user_notes_summary": map[string]any{
			"uncertainties": []any{"u1"},
			"needs_review":  []any{"n1"},
			"workarounds":   []any{"w1"},
		},
	}
	if err := writeJSONFile(filepath.Join(runDir, "grading.json"), grading); err != nil {
		t.Fatalf("write grading: %v", err)
	}
}

func TestAggregateBenchmarkFlow(t *testing.T) {
	tmp := t.TempDir()
	benchDir := filepath.Join(tmp, "bench")

	eval0 := filepath.Join(benchDir, "eval-0")
	writeTestFile(t, filepath.Join(eval0, "eval_metadata.json"), `{"eval_id":0}`)
	runA := filepath.Join(eval0, "with_skill", "run-1")
	runB := filepath.Join(eval0, "without_skill", "run-1")
	if err := os.MkdirAll(runA, 0o755); err != nil {
		t.Fatalf("mkdir runA: %v", err)
	}
	if err := os.MkdirAll(runB, 0o755); err != nil {
		t.Fatalf("mkdir runB: %v", err)
	}
	writeGradingFixture(t, runA, 1.0, 3, 0, 3)
	writeGradingFixture(t, runB, 0.33, 1, 2, 3)

	results, err := loadRunResults(benchDir)
	if err != nil {
		t.Fatalf("loadRunResults: %v", err)
	}
	if len(results["with_skill"]) != 1 || len(results["without_skill"]) != 1 {
		t.Fatalf("unexpected result grouping: %+v", results)
	}

	if err := runAggregateBenchmark([]string{"--skill-name", "demo", benchDir}); err != nil {
		t.Fatalf("runAggregateBenchmark: %v", err)
	}

	if _, err := os.Stat(filepath.Join(benchDir, "benchmark.json")); err != nil {
		t.Fatalf("benchmark.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(benchDir, "benchmark.md")); err != nil {
		t.Fatalf("benchmark.md missing: %v", err)
	}
}

func TestAggregateHelpers(t *testing.T) {
	st := calcStats([]float64{1, 2, 3})
	if st.Mean <= 0 || st.Max != 3 {
		t.Fatalf("unexpected stats: %+v", st)
	}
	if parseRunNumber("run-9") != 9 {
		t.Fatalf("parseRunNumber failed")
	}
	if got := joinInts([]int{1, 2, 3}); got != "1, 2, 3" {
		t.Fatalf("joinInts failed: %q", got)
	}
	if _, err := requireJSONMap("x"); err == nil {
		t.Fatalf("requireJSONMap should fail for non-map")
	}
}

func TestGenerateReport(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "input.json")
	outputPath := filepath.Join(tmp, "report.html")

	data := map[string]any{
		"original_description": "orig",
		"best_description":     "best",
		"best_score":           "1/1",
		"iterations_run":       1,
		"train_size":           1,
		"test_size":            0,
		"history": []any{
			map[string]any{
				"iteration":   1,
				"description": "best",
				"results": []any{
					map[string]any{"query": "q", "should_trigger": true, "pass": true, "triggers": 1, "runs": 1},
				},
			},
		},
	}
	if err := writeJSONFile(inputPath, data); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if err := runGenerateReport([]string{"--output", outputPath, "--skill-name", "demo-skill", inputPath}); err != nil {
		t.Fatalf("runGenerateReport: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "demo-skill") || !strings.Contains(s, "Skill Description Optimization") {
		t.Fatalf("report content mismatch")
	}
}

func createReviewFixture(t *testing.T, workspace string) string {
	t.Helper()
	runDir := filepath.Join(workspace, "eval-0", "with_skill", "run-1")
	outputsDir := filepath.Join(runDir, "outputs")
	if err := os.MkdirAll(outputsDir, 0o755); err != nil {
		t.Fatalf("mkdir outputs: %v", err)
	}
	writeTestFile(t, filepath.Join(runDir, "eval_metadata.json"), `{"eval_id":0,"prompt":"do task"}`)
	writeTestFile(t, filepath.Join(outputsDir, "out.txt"), "hello")
	writeTestFile(t, filepath.Join(outputsDir, "blob.bin"), "abc")
	writeGradingFixture(t, runDir, 1.0, 1, 0, 1)
	return runDir
}

func TestGenerateReviewStaticAndHelpers(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "iter-1")
	createReviewFixture(t, workspace)

	prev := filepath.Join(tmp, "iter-0")
	prevRun := createReviewFixture(t, prev)
	_ = prevRun
	feedback := map[string]any{
		"reviews": []any{
			map[string]any{"run_id": "eval-0-with_skill-run-1", "feedback": "looks good"},
		},
	}
	if err := writeJSONFile(filepath.Join(prev, "feedback.json"), feedback); err != nil {
		t.Fatalf("write feedback: %v", err)
	}
	benchPath := filepath.Join(workspace, "benchmark.json")
	if err := writeJSONFile(benchPath, map[string]any{"ok": true}); err != nil {
		t.Fatalf("write benchmark: %v", err)
	}

	outHTML := filepath.Join(tmp, "review.html")
	err := runGenerateReview([]string{
		"--skill-name", "demo",
		"--previous-workspace", prev,
		"--benchmark", benchPath,
		"--static", outHTML,
		workspace,
	})
	if err != nil {
		t.Fatalf("runGenerateReview: %v", err)
	}
	raw, err := os.ReadFile(outHTML)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	if !strings.Contains(string(raw), "Eval Review") {
		t.Fatalf("html missing title marker")
	}

	runs := findReviewRuns(workspace)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	prevMap, err := loadPreviousIteration(prev)
	if err != nil {
		t.Fatalf("loadPreviousIteration: %v", err)
	}
	if strings.TrimSpace(asString(prevMap["eval-0-with_skill-run-1"]["feedback"])) == "" {
		t.Fatalf("expected previous feedback")
	}
}

func TestReviewServerFeedbackHandler(t *testing.T) {
	tmp := t.TempDir()
	server := &reviewServer{
		workspace:    tmp,
		feedbackPath: filepath.Join(tmp, "feedback.json"),
		skillName:    "demo",
	}
	createReviewFixture(t, tmp)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback", nil)
	w := httptest.NewRecorder()
	server.handleFeedback(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET feedback status = %d", w.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(`{"reviews":[{"run_id":"x","feedback":"ok"}]}`))
	postReq.Header.Set("Content-Type", "application/json")
	postW := httptest.NewRecorder()
	server.handleFeedback(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST feedback status = %d body=%s", postW.Code, postW.Body.String())
	}

	var saved map[string]any
	if err := readJSONFile(server.feedbackPath, &saved); err != nil {
		t.Fatalf("feedback not saved: %v", err)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(`{"not_reviews":1}`))
	badW := httptest.NewRecorder()
	server.handleFeedback(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d", badW.Code)
	}

	idxReq := httptest.NewRequest(http.MethodGet, "/", nil)
	idxW := httptest.NewRecorder()
	server.handleIndex(idxW, idxReq)
	if idxW.Code != http.StatusOK {
		t.Fatalf("handleIndex status = %d", idxW.Code)
	}
	body, _ := io.ReadAll(idxW.Body)
	if !strings.Contains(string(body), "Eval Review") {
		t.Fatalf("index should contain review page")
	}
}

func TestEmbedReviewFileAndMIME(t *testing.T) {
	tmp := t.TempDir()
	textPath := filepath.Join(tmp, "a.txt")
	binPath := filepath.Join(tmp, "b.bin")
	if err := os.WriteFile(textPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	if err := os.WriteFile(binPath, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatalf("write bin: %v", err)
	}

	textOut := embedReviewFile(textPath)
	if asString(textOut["type"]) != "text" {
		t.Fatalf("expected text type, got %+v", textOut)
	}
	binOut := embedReviewFile(binPath)
	if asString(binOut["type"]) != "binary" {
		t.Fatalf("expected binary type, got %+v", binOut)
	}
	if reviewMIME("x.svg") != "image/svg+xml" {
		t.Fatalf("svg mime override failed")
	}
}

func TestGenerateReviewHTML(t *testing.T) {
	runs := []reviewRun{
		{ID: "run-1", Prompt: "p", Outputs: []map[string]any{{"type": "text", "name": "a.txt", "content": "x"}}},
	}
	previous := map[string]map[string]any{
		"run-1": {"feedback": "ok", "outputs": []any{}},
	}
	html, err := generateReviewHTML(runs, "demo", previous, map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("generateReviewHTML: %v", err)
	}
	if !strings.Contains(html, "Eval Review") || !strings.Contains(html, "run-1") {
		t.Fatalf("unexpected html output")
	}
	var payload map[string]any
	start := strings.Index(html, "const EMBEDDED_DATA = ")
	if start < 0 {
		t.Fatalf("embedded data marker missing")
	}
	_ = payload
}

func TestRenderOptimizationReportHelpers(t *testing.T) {
	results := []map[string]any{
		{"should_trigger": true, "triggers": 2, "runs": 2},
		{"should_trigger": false, "triggers": 0, "runs": 2},
	}
	c, total := aggregateCorrectRuns(results)
	if c != 4 || total != 4 {
		t.Fatalf("unexpected aggregateCorrectRuns result: %d/%d", c, total)
	}
	if scoreClass(8, 10) != "good" || scoreClass(3, 10) != "bad" {
		t.Fatalf("scoreClass thresholds mismatch")
	}
}

func TestAsHelpers(t *testing.T) {
	m := asMapSlice([]any{map[string]any{"a": 1}, "x"})
	if len(m) != 1 {
		t.Fatalf("asMapSlice mismatch")
	}
	if asString(1) != "" {
		t.Fatalf("asString should return empty for non-string")
	}
	if asFloat(json.Number("2.5")) != 2.5 {
		t.Fatalf("asFloat json number mismatch")
	}
	if !asBoolDefault("true", false) || asBoolDefault("nope", true) != true {
		t.Fatalf("asBoolDefault mismatch")
	}
}
