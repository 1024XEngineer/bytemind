package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var newDescriptionRe = regexp.MustCompile(`(?s)<new_description>(.*?)</new_description>`)

type improveParams struct {
	SkillName          string
	SkillContent       string
	CurrentDescription string
	EvalResults        map[string]any
	History            []map[string]any
	Model              string
	TestResults        map[string]any
	LogDir             string
	Iteration          int
}

func runImproveDescription(args []string) error {
	fs := flag.NewFlagSet("improve-description", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	evalResultsPath := fs.String("eval-results", "", "Eval results JSON path")
	skillPath := fs.String("skill-path", "", "Skill directory")
	historyPath := fs.String("history", "", "History JSON path")
	model := fs.String("model", "", "Model for improvement")
	verbose := fs.Bool("verbose", false, "Verbose logs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*evalResultsPath) == "" || strings.TrimSpace(*skillPath) == "" || strings.TrimSpace(*model) == "" {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools improve-description --eval-results <file> --skill-path <dir> --model <model>")
	}

	var evalResults map[string]any
	if err := readJSONFile(*evalResultsPath, &evalResults); err != nil {
		return err
	}
	doc, err := parseSkillMDDir(*skillPath)
	if err != nil {
		return err
	}
	history := make([]map[string]any, 0)
	if strings.TrimSpace(*historyPath) != "" {
		if err := readJSONFile(*historyPath, &history); err != nil {
			return err
		}
	}
	currentDescription := asString(evalResults["description"])
	if strings.TrimSpace(currentDescription) == "" {
		currentDescription = doc.Description
	}

	if *verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Current: %s\n", currentDescription)
	}

	newDesc, _, err := improveDescription(improveParams{
		SkillName:          doc.Name,
		SkillContent:       doc.Content,
		CurrentDescription: currentDescription,
		EvalResults:        evalResults,
		History:            history,
		Model:              *model,
	})
	if err != nil {
		return err
	}

	if *verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Improved: %s\n", newDesc)
	}

	summary, _ := evalResults["summary"].(map[string]any)
	output := map[string]any{
		"description": newDesc,
		"history": append(history, map[string]any{
			"description": currentDescription,
			"passed":      int(asFloat(summary["passed"])),
			"failed":      int(asFloat(summary["failed"])),
			"total":       int(asFloat(summary["total"])),
			"results":     evalResults["results"],
		}),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func improveDescription(params improveParams) (string, map[string]any, error) {
	evalResults := params.EvalResults
	results := asMapSlice(evalResults["results"])
	failedTriggers := make([]map[string]any, 0)
	falseTriggers := make([]map[string]any, 0)
	for _, r := range results {
		should := asBoolDefault(r["should_trigger"], true)
		passed := asBoolDefault(r["pass"], false)
		if should && !passed {
			failedTriggers = append(failedTriggers, r)
		}
		if !should && !passed {
			falseTriggers = append(falseTriggers, r)
		}
	}

	summary, _ := evalResults["summary"].(map[string]any)
	trainScore := fmt.Sprintf("%d/%d", int(asFloat(summary["passed"])), int(asFloat(summary["total"])))
	scoresSummary := "Train: " + trainScore
	if params.TestResults != nil {
		testSummary, _ := params.TestResults["summary"].(map[string]any)
		scoresSummary = fmt.Sprintf("Train: %s, Test: %d/%d", trainScore, int(asFloat(testSummary["passed"])), int(asFloat(testSummary["total"])))
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("You are optimizing a skill description for a Claude Code skill called \"%s\".\n\n", params.SkillName))
	b.WriteString("Current description:\n")
	b.WriteString("<current_description>\n\"")
	b.WriteString(params.CurrentDescription)
	b.WriteString("\"\n</current_description>\n\n")
	b.WriteString("Current scores (")
	b.WriteString(scoresSummary)
	b.WriteString("):\n")

	if len(failedTriggers) > 0 {
		b.WriteString("FAILED TO TRIGGER (should have triggered but did not):\n")
		for _, r := range failedTriggers {
			b.WriteString(fmt.Sprintf("- \"%s\" (triggered %d/%d times)\n", asString(r["query"]), int(asFloat(r["triggers"])), int(asFloat(r["runs"]))))
		}
		b.WriteString("\n")
	}
	if len(falseTriggers) > 0 {
		b.WriteString("FALSE TRIGGERS (triggered but should not):\n")
		for _, r := range falseTriggers {
			b.WriteString(fmt.Sprintf("- \"%s\" (triggered %d/%d times)\n", asString(r["query"]), int(asFloat(r["triggers"])), int(asFloat(r["runs"]))))
		}
		b.WriteString("\n")
	}

	if len(params.History) > 0 {
		b.WriteString("PREVIOUS ATTEMPTS (avoid repeating same style):\n\n")
		for _, h := range params.History {
			train := fmt.Sprintf("%d/%d", int(asFloat(h["train_passed"])+asFloat(h["passed"])), int(asFloat(h["train_total"])+asFloat(h["total"])))
			b.WriteString(fmt.Sprintf("<attempt train=%s>\n", train))
			b.WriteString("Description: \"")
			b.WriteString(asString(h["description"]))
			b.WriteString("\"\n")
			for _, r := range asMapSlice(h["results"]) {
				status := "FAIL"
				if asBoolDefault(r["pass"], false) {
					status = "PASS"
				}
				b.WriteString(fmt.Sprintf("  [%s] \"%s\" (%d/%d)\n", status, truncateString(asString(r["query"]), 80), int(asFloat(r["triggers"])), int(asFloat(r["runs"]))))
			}
			if note := strings.TrimSpace(asString(h["note"])); note != "" {
				b.WriteString("Note: " + note + "\n")
			}
			b.WriteString("</attempt>\n\n")
		}
	}

	b.WriteString("Skill content for context:\n<skill_content>\n")
	b.WriteString(params.SkillContent)
	b.WriteString("\n</skill_content>\n\n")
	b.WriteString("Write a new description that generalizes from failure patterns, is concise (100-200 words preferred, hard limit 1024 chars), and focuses on user intent. ")
	b.WriteString("Respond only with the new description wrapped in <new_description>...</new_description>.\n")

	prompt := b.String()
	response, err := callClaudeText(prompt, params.Model, 300)
	if err != nil {
		return "", nil, err
	}
	description := extractNewDescription(response)
	transcript := map[string]any{
		"iteration":          params.Iteration,
		"prompt":             prompt,
		"response":           response,
		"parsed_description": description,
		"char_count":         len(description),
		"over_limit":         len(description) > 1024,
	}

	if len(description) > 1024 {
		shortenPrompt := fmt.Sprintf("%s\n\nA previous attempt produced this description (%d chars, over limit):\n\"%s\"\nRewrite under 1024 chars and respond only in <new_description> tags.", prompt, len(description), description)
		shortenResp, err := callClaudeText(shortenPrompt, params.Model, 300)
		if err == nil {
			shortened := extractNewDescription(shortenResp)
			if strings.TrimSpace(shortened) != "" {
				description = shortened
			}
			transcript["rewrite_prompt"] = shortenPrompt
			transcript["rewrite_response"] = shortenResp
			transcript["rewrite_description"] = description
			transcript["rewrite_char_count"] = len(description)
		}
	}
	transcript["final_description"] = description

	if strings.TrimSpace(params.LogDir) != "" {
		if err := os.MkdirAll(params.LogDir, 0o755); err == nil {
			name := fmt.Sprintf("improve_iter_%d.json", params.Iteration)
			if params.Iteration <= 0 {
				name = "improve_iter_unknown.json"
			}
			_ = writeJSONFile(filepath.Join(params.LogDir, name), transcript)
		}
	}

	return description, transcript, nil
}

func extractNewDescription(text string) string {
	m := newDescriptionRe.FindStringSubmatch(text)
	if len(m) >= 2 {
		return strings.TrimSpace(strings.Trim(m[1], "\"'"))
	}
	return strings.TrimSpace(strings.Trim(text, "\"'"))
}
