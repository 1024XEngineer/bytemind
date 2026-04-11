package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"strconv"
	"strings"
)

func runGenerateReport(args []string) error {
	fs := flag.NewFlagSet("generate-report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	output := fs.String("output", "", "Output HTML file (default stdout)")
	skillName := fs.String("skill-name", "", "Skill name in report title")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools generate-report <run_loop_output.json|-> [--output file] [--skill-name name]")
	}

	inputArg := fs.Arg(0)
	var data map[string]any
	if inputArg == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
	} else {
		if err := readJSONFile(inputArg, &data); err != nil {
			return err
		}
	}

	htmlOutput := renderOptimizationReportHTML(data, false, *skillName)
	if strings.TrimSpace(*output) == "" {
		_, _ = fmt.Fprint(os.Stdout, htmlOutput)
		return nil
	}
	if err := os.WriteFile(*output, []byte(htmlOutput), 0o644); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(os.Stderr, "Report written to %s\n", *output)
	return nil
}

func renderOptimizationReportHTML(data map[string]any, autoRefresh bool, skillName string) string {
	history := asMapSlice(data["history"])
	titlePrefix := ""
	if strings.TrimSpace(skillName) != "" {
		titlePrefix = html.EscapeString(skillName) + " - "
	}

	trainQueries := make([]map[string]any, 0)
	testQueries := make([]map[string]any, 0)
	if len(history) > 0 {
		first := history[0]
		trainResults := asMapSlice(first["train_results"])
		if len(trainResults) == 0 {
			trainResults = asMapSlice(first["results"])
		}
		for _, r := range trainResults {
			trainQueries = append(trainQueries, map[string]any{
				"query":          asString(r["query"]),
				"should_trigger": asBoolDefault(r["should_trigger"], true),
			})
		}
		for _, r := range asMapSlice(first["test_results"]) {
			testQueries = append(testQueries, map[string]any{
				"query":          asString(r["query"]),
				"should_trigger": asBoolDefault(r["should_trigger"], true),
			})
		}
	}

	refreshTag := ""
	if autoRefresh {
		refreshTag = `<meta http-equiv="refresh" content="5">`
	}

	var buf bytes.Buffer
	buf.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\">")
	buf.WriteString(refreshTag)
	buf.WriteString("<title>")
	buf.WriteString(titlePrefix)
	buf.WriteString("Skill Description Optimization</title>")
	buf.WriteString(`<style>
body{font-family:system-ui,Segoe UI,Arial,sans-serif;background:#faf9f5;color:#141413;margin:0;padding:20px}
.summary,.explainer{background:#fff;border:1px solid #e8e6dc;border-radius:6px;padding:12px;margin-bottom:16px}
table{border-collapse:collapse;width:100%;font-size:12px;background:#fff;border:1px solid #e8e6dc}
th,td{border:1px solid #e8e6dc;padding:8px;vertical-align:top}
th{background:#141413;color:#faf9f5}
th.test{background:#6a9bcc}
th.pos{border-bottom:3px solid #788c5d}
th.neg{border-bottom:3px solid #c44}
tr.best{background:#f5f8f2}
td.desc{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;max-width:460px;word-break:break-word}
td.result{text-align:center;min-width:48px}
.rate{display:block;color:#777;font-size:10px}
.pass{color:#788c5d}.fail{color:#c44}
.score{display:inline-block;padding:2px 6px;border-radius:4px;font-weight:700}
.good{background:#eef2e8;color:#788c5d}.ok{background:#fef3c7;color:#d97706}.bad{background:#fceaea;color:#c44}
</style></head><body>`)

	buf.WriteString("<h1>")
	buf.WriteString(titlePrefix)
	buf.WriteString("Skill Description Optimization</h1>")
	buf.WriteString(`<div class="explainer"><strong>Optimizing your skill description.</strong> Each row is one iteration. Check means trigger behavior matched expected intent.</div>`)

	bestDescription := html.EscapeString(asString(data["best_description"]))
	origDescription := html.EscapeString(asString(data["original_description"]))
	bestScore := html.EscapeString(asString(data["best_score"]))
	buf.WriteString(`<div class="summary">`)
	buf.WriteString("<p><strong>Original:</strong> " + origDescription + "</p>")
	buf.WriteString("<p><strong>Best:</strong> " + bestDescription + "</p>")
	buf.WriteString("<p><strong>Best Score:</strong> " + bestScore + "</p>")
	buf.WriteString(fmt.Sprintf("<p><strong>Iterations:</strong> %d | <strong>Train:</strong> %d | <strong>Test:</strong> %d</p>", int(asFloat(data["iterations_run"])), int(asFloat(data["train_size"])), int(asFloat(data["test_size"]))))
	buf.WriteString(`</div>`)

	bestIter := 0
	bestValue := -1.0
	for _, h := range history {
		candidate := asFloat(h["test_passed"])
		if candidate == 0 && h["test_passed"] == nil {
			candidate = asFloat(h["train_passed"])
		}
		if candidate > bestValue {
			bestValue = candidate
			bestIter = int(asFloat(h["iteration"]))
		}
	}

	buf.WriteString("<table><thead><tr><th>Iter</th><th>Train</th><th>Test</th><th>Description</th>")
	for _, q := range trainQueries {
		polarity := "pos"
		if !asBoolDefault(q["should_trigger"], true) {
			polarity = "neg"
		}
		buf.WriteString(fmt.Sprintf("<th class=\"%s\">%s</th>", polarity, html.EscapeString(asString(q["query"]))))
	}
	for _, q := range testQueries {
		polarity := "pos"
		if !asBoolDefault(q["should_trigger"], true) {
			polarity = "neg"
		}
		buf.WriteString(fmt.Sprintf("<th class=\"test %s\">%s</th>", polarity, html.EscapeString(asString(q["query"]))))
	}
	buf.WriteString("</tr></thead><tbody>")

	for _, h := range history {
		iter := int(asFloat(h["iteration"]))
		trainResults := asMapSlice(h["train_results"])
		if len(trainResults) == 0 {
			trainResults = asMapSlice(h["results"])
		}
		testResults := asMapSlice(h["test_results"])
		trainByQuery := make(map[string]map[string]any)
		for _, r := range trainResults {
			trainByQuery[asString(r["query"])] = r
		}
		testByQuery := make(map[string]map[string]any)
		for _, r := range testResults {
			testByQuery[asString(r["query"])] = r
		}

		trainCorrect, trainRuns := aggregateCorrectRuns(trainResults)
		testCorrect, testRuns := aggregateCorrectRuns(testResults)
		rowClass := ""
		if iter == bestIter {
			rowClass = " class=\"best\""
		}

		buf.WriteString("<tr" + rowClass + ">")
		buf.WriteString(fmt.Sprintf("<td>%d</td>", iter))
		buf.WriteString(fmt.Sprintf("<td><span class=\"score %s\">%d/%d</span></td>", scoreClass(trainCorrect, trainRuns), trainCorrect, trainRuns))
		buf.WriteString(fmt.Sprintf("<td><span class=\"score %s\">%d/%d</span></td>", scoreClass(testCorrect, testRuns), testCorrect, testRuns))
		buf.WriteString("<td class=\"desc\">" + html.EscapeString(asString(h["description"])) + "</td>")

		for _, q := range trainQueries {
			buf.WriteString(renderResultCell(trainByQuery[asString(q["query"])], false))
		}
		for _, q := range testQueries {
			buf.WriteString(renderResultCell(testByQuery[asString(q["query"])], true))
		}
		buf.WriteString("</tr>")
	}

	buf.WriteString("</tbody></table></body></html>")
	return buf.String()
}

func renderResultCell(result map[string]any, isTest bool) string {
	if result == nil {
		if isTest {
			return `<td class="result">-</td>`
		}
		return `<td class="result">-</td>`
	}
	pass := asBoolDefault(result["pass"], false)
	triggers := int(asFloat(result["triggers"]))
	runs := int(asFloat(result["runs"]))
	icon := "✗"
	css := "fail"
	if pass {
		icon = "✓"
		css = "pass"
	}
	return fmt.Sprintf(`<td class="result %s">%s<span class="rate">%d/%d</span></td>`, css, icon, triggers, runs)
}

func aggregateCorrectRuns(results []map[string]any) (int, int) {
	correct := 0
	total := 0
	for _, r := range results {
		runs := int(asFloat(r["runs"]))
		triggers := int(asFloat(r["triggers"]))
		total += runs
		if asBoolDefault(r["should_trigger"], true) {
			correct += triggers
		} else {
			correct += runs - triggers
		}
	}
	return correct, total
}

func scoreClass(correct, total int) string {
	if total <= 0 {
		return "bad"
	}
	ratio := float64(correct) / float64(total)
	if ratio >= 0.8 {
		return "good"
	}
	if ratio >= 0.5 {
		return "ok"
	}
	return "bad"
}

func asMapSlice(v any) []map[string]any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func asBoolDefault(v any, fallback bool) bool {
	switch x := v.(type) {
	case nil:
		return fallback
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		if x == "true" || x == "1" {
			return true
		}
		if x == "false" || x == "0" {
			return false
		}
	}
	return fallback
}
