package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	"github.com/1024XEngineer/bytemind/internal/llm"
	policypkg "github.com/1024XEngineer/bytemind/internal/policy"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
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

func resolveSkillToolSets(active *activeSkillRuntime, registry ToolRegistry) (map[string]struct{}, map[string]struct{}, error) {
	if active == nil {
		return nil, nil, nil
	}
	bindings := activeSkillBridgeBindings(active, registry)
	if len(bindings) == 0 {
		allow, deny := policypkg.ResolveToolSets(active.Skill.ToolPolicy)
		return allow, deny, nil
	}
	allow, deny, err := extensionspkg.ResolvePolicyToolSets(active.Skill.ToolPolicy, bindings)
	if err != nil {
		return nil, nil, err
	}
	return allow, deny, nil
}

type extensionToolMetaFinder interface {
	FindByExtensionID(extensionID string) []toolspkg.RegistrationMeta
}

func activeSkillBridgeBindings(active *activeSkillRuntime, registry ToolRegistry) []extensionspkg.BridgeBinding {
	if active == nil || registry == nil {
		return nil
	}
	finder, ok := registry.(extensionToolMetaFinder)
	if !ok {
		return nil
	}
	extensionID := activeSkillExtensionID(active)
	if extensionID == "" {
		return nil
	}
	metas := finder.FindByExtensionID(extensionID)
	if len(metas) == 0 {
		return nil
	}
	bindings := make([]extensionspkg.BridgeBinding, 0, len(metas))
	for _, meta := range metas {
		if meta.Source != toolspkg.RegistrationSourceExtension {
			continue
		}
		stable := strings.TrimSpace(meta.StableToolKey)
		if stable == "" {
			stable = strings.TrimSpace(meta.ToolKey)
		}
		original := strings.TrimSpace(meta.OriginalToolName)
		if stable == "" || original == "" {
			continue
		}
		bindings = append(bindings, extensionspkg.BridgeBinding{
			Source:       bridgeSourceFromStableKey(stable),
			ExtensionID:  strings.TrimSpace(meta.ExtensionID),
			OriginalName: original,
			StableKey:    stable,
		})
	}
	return bindings
}

func activeSkillExtensionID(active *activeSkillRuntime) string {
	if active == nil {
		return ""
	}
	return extensionspkg.SkillExtensionID(active.Skill.Name)
}

func bridgeSourceFromStableKey(stable string) extensionspkg.ExtensionKind {
	stable = strings.TrimSpace(stable)
	if stable == "" {
		return extensionspkg.ExtensionSkill
	}
	segments := strings.SplitN(stable, ":", 2)
	switch strings.ToLower(strings.TrimSpace(segments[0])) {
	case string(extensionspkg.ExtensionMCP):
		return extensionspkg.ExtensionMCP
	case string(extensionspkg.ExtensionSkill):
		return extensionspkg.ExtensionSkill
	default:
		return extensionspkg.ExtensionSkill
	}
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
	model := r.modelID()
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
