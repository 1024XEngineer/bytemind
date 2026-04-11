package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type skillDoc struct {
	Name        string
	Description string
	Content     string
	Frontmatter map[string]string
}

func parseSkillMDDir(skillDir string) (skillDoc, error) {
	path := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return skillDoc{}, err
	}
	return parseSkillMDContent(string(data))
}

func parseSkillMDContent(content string) (skillDoc, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skillDoc{}, errors.New("SKILL.md missing frontmatter (no opening ---)")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return skillDoc{}, errors.New("SKILL.md missing frontmatter (no closing ---)")
	}

	fmLines := lines[1:end]
	frontmatter := parseSimpleFrontmatter(fmLines)
	name := strings.TrimSpace(frontmatter["name"])
	description := strings.TrimSpace(frontmatter["description"])

	return skillDoc{
		Name:        name,
		Description: description,
		Content:     content,
		Frontmatter: frontmatter,
	}, nil
}

func parseSimpleFrontmatter(lines []string) map[string]string {
	out := make(map[string]string)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])

		if value == "|" || value == ">" || value == "|-" || value == ">-" {
			var parts []string
			j := i + 1
			for ; j < len(lines); j++ {
				next := lines[j]
				nextTrim := strings.TrimSpace(next)
				if nextTrim == "" {
					parts = append(parts, "")
					continue
				}
				if strings.HasPrefix(next, "  ") || strings.HasPrefix(next, "\t") {
					parts = append(parts, strings.TrimSpace(next))
					continue
				}
				break
			}
			out[key] = strings.TrimSpace(strings.Join(parts, " "))
			i = j - 1
			continue
		}

		out[key] = trimQuotes(value)
	}
	return out
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func readJSONFile(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty json file")
	}
	return json.Unmarshal(data, dst)
}

func writeJSONFile(path string, src any) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func parseBoolEnv(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return false
	}
	ok, err := strconv.ParseBool(v)
	return err == nil && ok
}

func findProjectRootWithClaudeDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	curr := wd
	for {
		if info, err := os.Stat(filepath.Join(curr, ".claude")); err == nil && info.IsDir() {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return wd
}

func openBrowser(target string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}

func callClaudeText(prompt string, model string, timeoutSeconds int) (string, error) {
	args := []string{"-p", "--output-format", "text"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("claude", args...)
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = env
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("claude -p exited with error: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), nil
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return "", fmt.Errorf("claude -p timed out after %ds", timeoutSeconds)
	}
}

func mustAbs(path string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
