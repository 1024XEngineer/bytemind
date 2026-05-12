package skills

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
)

//go:embed bug-investigation github-pr repo-onboarding review skill-creator write-rfc
var builtinFS embed.FS

func loadBuiltinFromEmbedded(scope Scope) ([]Skill, []Diagnostic) {
	return loadSkillsFromFS(scope, builtinFS, ".")
}

func loadSkillsFromFS(scope Scope, fsys fs.FS, root string) ([]Skill, []Diagnostic) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, nil
	}
	skills := make([]Skill, 0, len(entries))
	diags := make([]Diagnostic, 0, 4)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := path.Join(root, entry.Name())
		skill, ok, skillDiags := loadSkillFromFSDir(scope, fsys, skillDir, entry.Name())
		diags = append(diags, skillDiags...)
		if ok {
			skills = append(skills, skill)
		}
	}
	return skills, diags
}

func loadSkillFromFSDir(scope Scope, fsys fs.FS, skillDir, dirName string) (Skill, bool, []Diagnostic) {
	manifestPath := path.Join(skillDir, "skill.json")
	skillPath := path.Join(skillDir, "SKILL.md")

	hasManifest := fileExistsFS(fsys, manifestPath)
	hasSkill := fileExistsFS(fsys, skillPath)
	if !hasManifest && !hasSkill {
		return Skill{}, false, nil
	}

	var manifest skillManifest
	diags := make([]Diagnostic, 0, 2)
	if hasManifest {
		data, err := fs.ReadFile(fsys, manifestPath)
		if err != nil {
			return Skill{}, false, []Diagnostic{{
				Scope:   scope,
				Path:    manifestPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("failed to read skill.json: %v", err),
			}}
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Skill{}, false, []Diagnostic{{
				Scope:   scope,
				Path:    manifestPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("invalid skill.json: %v", err),
			}}
		}
	}

	frontmatter := map[string]string{}
	body := ""
	if hasSkill {
		data, err := fs.ReadFile(fsys, skillPath)
		if err != nil {
			diags = append(diags, Diagnostic{
				Scope:   scope,
				Path:    skillPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("failed to read SKILL.md: %v", err),
			})
			hasSkill = false
		} else {
			frontmatter, body = parseFrontmatterMarkdown(string(data))
		}
	}

	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = strings.TrimSpace(frontmatter["name"])
	}
	if name == "" {
		name = dirName
	}
	if !validSkillName.MatchString(name) {
		diags = append(diags, Diagnostic{
			Scope:   scope,
			Path:    skillDir,
			Skill:   name,
			Level:   "error",
			Message: "invalid skill name",
		})
		return Skill{}, false, diags
	}

	description := strings.TrimSpace(manifest.Description)
	if description == "" {
		description = strings.TrimSpace(frontmatter["description"])
	}
	if description == "" {
		description = extractDescription(body)
	}
	if description == "" {
		description = "No description provided."
	}

	title := strings.TrimSpace(manifest.Title)
	if title == "" {
		title = name
	}

	whenToUse := strings.TrimSpace(frontmatter["when_to_use"])
	if whenToUse == "" {
		whenToUse = strings.TrimSpace(frontmatter["when-to-use"])
	}

	entry := manifest.Entry
	if strings.TrimSpace(entry.Slash) == "" {
		entry.Slash = "/" + name
	} else if !strings.HasPrefix(entry.Slash, "/") {
		entry.Slash = "/" + strings.TrimSpace(entry.Slash)
	}

	toolPolicy, policyDiag := buildToolPolicy(manifest.Tools.Policy, manifest.Tools.Items, frontmatter)
	if policyDiag != nil {
		policyDiag.Scope = scope
		policyDiag.Path = skillDir
		policyDiag.Skill = name
		diags = append(diags, *policyDiag)
	}

	prompts := make([]PromptRef, 0, len(manifest.Prompts))
	for _, prompt := range manifest.Prompts {
		prompts = append(prompts, PromptRef{
			ID:   strings.TrimSpace(prompt.ID),
			Path: strings.TrimSpace(prompt.Path),
		})
	}
	resources := make([]ResourceRef, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, ResourceRef{
			ID:       strings.TrimSpace(resource.ID),
			URI:      strings.TrimSpace(resource.URI),
			Optional: resource.Optional,
		})
	}
	args := make([]Arg, 0, len(manifest.Args))
	for _, arg := range manifest.Args {
		if strings.TrimSpace(arg.Name) == "" {
			continue
		}
		args = append(args, Arg{
			Name:        strings.TrimSpace(arg.Name),
			Type:        strings.TrimSpace(arg.Type),
			Required:    arg.Required,
			Description: strings.TrimSpace(arg.Description),
			Default:     strings.TrimSpace(arg.Default),
		})
	}

	aliases := uniqueStrings([]string{
		name,
		dirName,
		entry.Slash,
		strings.TrimPrefix(entry.Slash, "/"),
	})

	skill := Skill{
		Name:         name,
		Version:      strings.TrimSpace(manifest.Version),
		Title:        title,
		Description:  description,
		WhenToUse:    whenToUse,
		Scope:        scope,
		SourceDir:    skillDir,
		Instruction:  strings.TrimSpace(body),
		Entry:        entry,
		Prompts:      prompts,
		Resources:    resources,
		ToolPolicy:   toolPolicy,
		Args:         args,
		Aliases:      aliases,
		DiscoveredAt: time.Now().UTC(),
	}
	if !hasSkill {
		skill.Instruction = ""
	}
	return skill, true, diags
}

func fileExistsFS(fsys fs.FS, path string) bool {
	info, err := fs.Stat(fsys, path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
