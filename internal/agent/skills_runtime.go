package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"bytemind/internal/llm"
	policypkg "bytemind/internal/policy"
	"bytemind/internal/session"
	"bytemind/internal/skills"
)

type activeSkillRuntime struct {
	Skill skills.Skill
	Args  map[string]string
}

func (r *Runner) resolveActiveSkill(sess *session.Session) *activeSkillRuntime {
	if sess == nil || sess.ActiveSkill == nil || r.skillManager == nil {
		return nil
	}

	skill, ok := r.skillManager.Find(sess.ActiveSkill.Name)
	if !ok {
		sess.ActiveSkill = nil
		if r.store != nil {
			_ = r.store.Save(sess)
		}
		return nil
	}

	return &activeSkillRuntime{
		Skill: skill,
		Args:  normalizeSkillArgs(sess.ActiveSkill.Args),
	}
}

func resolveSkillToolSets(active *activeSkillRuntime) (map[string]struct{}, map[string]struct{}) {
	if active == nil {
		return nil, nil
	}
	return policypkg.ResolveToolSets(active.Skill.ToolPolicy)
}

func promptActiveSkill(active *activeSkillRuntime) *PromptActiveSkill {
	if active == nil {
		return nil
	}

	instruction := strings.TrimSpace(active.Skill.Instruction)
	if instruction != "" {
		instruction = trimTextWithEllipsis(instruction, maxActiveSkillInstructionsChars)
	}
	description := trimTextWithEllipsis(strings.TrimSpace(active.Skill.Description), maxActiveSkillDescriptionChars)
	whenToUse := trimTextWithEllipsis(strings.TrimSpace(active.Skill.WhenToUse), maxActiveSkillDescriptionChars)

	return &PromptActiveSkill{
		Name:         active.Skill.Name,
		Description:  description,
		WhenToUse:    whenToUse,
		Instructions: instruction,
		Args:         normalizeSkillArgs(active.Args),
		ToolPolicy:   string(active.Skill.ToolPolicy.Policy),
		Tools:        append([]string(nil), active.Skill.ToolPolicy.Items...),
	}
}

func trimTextWithEllipsis(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func normalizeSkillArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]string, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (r *Runner) normalizeSkillAuthorBrief(brief string) string {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return ""
	}
	if !containsHanRune(brief) {
		return brief
	}

	translated, err := r.translateSkillBriefToEnglish(brief)
	if err != nil {
		return skillAuthorEnglishFallback
	}
	translated = strings.TrimSpace(translated)
	if translated == "" || containsHanRune(translated) {
		return skillAuthorEnglishFallback
	}
	return translated
}

func (r *Runner) translateSkillBriefToEnglish(brief string) (string, error) {
	if strings.TrimSpace(brief) == "" {
		return "", nil
	}
	if r.client == nil {
		return "", fmt.Errorf("llm client is unavailable")
	}
	model := strings.TrimSpace(r.config.Provider.Model)
	if model == "" {
		return "", fmt.Errorf("model is required for translation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	reply, err := r.client.CreateMessage(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			llm.NewTextMessage(llm.RoleSystem, skillAuthorTranslatePrompt),
			llm.NewUserTextMessage(strings.TrimSpace(brief)),
		},
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reply.Text()), nil
}

func containsHanRune(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
