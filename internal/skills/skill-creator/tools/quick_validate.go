package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	kebabNameRe            = regexp.MustCompile(`^[a-z0-9-]+$`)
	allowedFrontmatterKeys = map[string]struct{}{
		"name":          {},
		"description":   {},
		"license":       {},
		"allowed-tools": {},
		"metadata":      {},
		"compatibility": {},
	}
)

type validateResult struct {
	Valid   bool
	Message string
}

func runQuickValidate(args []string) error {
	fs, err := parseFlagSet("quick-validate", args)
	if err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: go run ./internal/skills/skill-creator/tools quick-validate <skill_directory>")
	}

	res := validateSkill(fs.Arg(0))
	_, _ = fmt.Fprintln(os.Stdout, res.Message)
	if !res.Valid {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func validateSkill(skillPath string) validateResult {
	skillPath = mustAbs(strings.TrimSpace(skillPath))
	skillMDPath := filepath.Join(skillPath, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return validateResult{Valid: false, Message: "SKILL.md not found"}
		}
		return validateResult{Valid: false, Message: fmt.Sprintf("read SKILL.md failed: %v", err)}
	}

	doc, err := parseSkillMDContent(string(data))
	if err != nil {
		return validateResult{Valid: false, Message: err.Error()}
	}

	lines := strings.Split(doc.Content, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return validateResult{Valid: false, Message: "Invalid frontmatter format"}
	}
	frontmatterLines := lines[1:end]
	keys := extractTopLevelKeys(frontmatterLines)
	unexpected := make([]string, 0)
	for _, k := range keys {
		if _, ok := allowedFrontmatterKeys[k]; !ok {
			unexpected = append(unexpected, k)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		allowed := make([]string, 0, len(allowedFrontmatterKeys))
		for k := range allowedFrontmatterKeys {
			allowed = append(allowed, k)
		}
		sort.Strings(allowed)
		return validateResult{Valid: false, Message: fmt.Sprintf("Unexpected key(s) in SKILL.md frontmatter: %s. Allowed properties are: %s", strings.Join(unexpected, ", "), strings.Join(allowed, ", "))}
	}

	name := strings.TrimSpace(doc.Frontmatter["name"])
	if name == "" {
		return validateResult{Valid: false, Message: "Missing 'name' in frontmatter"}
	}
	if !kebabNameRe.MatchString(name) {
		return validateResult{Valid: false, Message: fmt.Sprintf("Name '%s' should be kebab-case (lowercase letters, digits, and hyphens only)", name)}
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return validateResult{Valid: false, Message: fmt.Sprintf("Name '%s' cannot start/end with hyphen or contain consecutive hyphens", name)}
	}
	if len(name) > 64 {
		return validateResult{Valid: false, Message: fmt.Sprintf("Name is too long (%d characters). Maximum is 64 characters.", len(name))}
	}

	description := strings.TrimSpace(doc.Frontmatter["description"])
	if description == "" {
		return validateResult{Valid: false, Message: "Missing 'description' in frontmatter"}
	}
	if strings.Contains(description, "<") || strings.Contains(description, ">") {
		return validateResult{Valid: false, Message: "Description cannot contain angle brackets (< or >)"}
	}
	if len(description) > 1024 {
		return validateResult{Valid: false, Message: fmt.Sprintf("Description is too long (%d characters). Maximum is 1024 characters.", len(description))}
	}

	compat := strings.TrimSpace(doc.Frontmatter["compatibility"])
	if len(compat) > 500 {
		return validateResult{Valid: false, Message: fmt.Sprintf("Compatibility is too long (%d characters). Maximum is 500 characters.", len(compat))}
	}

	return validateResult{Valid: true, Message: "Skill is valid!"}
}

func extractTopLevelKeys(lines []string) []string {
	keys := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	return keys
}
