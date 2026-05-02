package agent

import (
	"errors"
	"strings"

	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	"github.com/1024XEngineer/bytemind/internal/session"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
)

var nonBridgedBuiltinTools = map[string]struct{}{
	"list_files":      {},
	"read_file":       {},
	"search_text":     {},
	"web_search":      {},
	"web_fetch":       {},
	"write_file":      {},
	"replace_in_file": {},
	"apply_patch":     {},
	"update_plan":     {},
	"run_shell":       {},
}

type bridgeSessionState struct {
	extensionID string
	toolKeys    map[string]struct{}
}

func (r *Runner) syncActiveSkillBridges(sess *session.Session, active *activeSkillRuntime) error {
	if r == nil {
		return nil
	}
	registry, ok := r.registry.(*toolspkg.Registry)
	if !ok || registry == nil {
		return nil
	}
	sessionKey := bridgeSessionKey(sess)
	if sessionKey == "" {
		return nil
	}

	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	r.ensureBridgeStateLocked()
	r.bridgeSessionTurns[sessionKey]++

	state := r.bridgeSessions[sessionKey]
	if state.toolKeys == nil {
		state.toolKeys = map[string]struct{}{}
	}
	if active == nil {
		return r.clearSessionBridgesLocked(registry, sessionKey)
	}

	extensionID := activeSkillExtensionID(active)
	if extensionID == "" {
		return r.clearSessionBridgesLocked(registry, sessionKey)
	}
	if state.extensionID != extensionID {
		if err := r.releaseBridgeToolsLocked(registry, state.toolKeys); err != nil {
			return err
		}
		state.extensionID = extensionID
		state.toolKeys = map[string]struct{}{}
	}

	desired := map[string]struct{}{}
	for _, item := range active.Skill.ToolPolicy.Items {
		original := normalizeBridgePolicyItem(item)
		if original == "" || !shouldBridgePolicyItem(original) {
			continue
		}
		resolved, exists := registry.Get(original)
		if !exists {
			continue
		}
		binding, err := extensionspkg.RegisterBridgedToolWithOptions(registry, extensionspkg.ExtensionTool{
			Source:      extensionspkg.ExtensionSkill,
			ExtensionID: extensionID,
			Tool:        resolved.Tool,
		}, extensionspkg.BridgeRegisterOptions{
			AllowOriginalNameShadowBuiltin: true,
		})
		if err != nil {
			stableKey, stableErr := extensionspkg.StableToolKey(extensionspkg.ExtensionSkill, extensionID, original)
			if stableErr != nil {
				return err
			}
			if _, ok := registry.Get(stableKey); !ok {
				return err
			}
			binding = extensionspkg.BridgeBinding{
				Source:       extensionspkg.ExtensionSkill,
				ExtensionID:  extensionID,
				OriginalName: original,
				StableKey:    stableKey,
			}
		}
		desired[binding.StableKey] = struct{}{}
	}

	for toolKey := range state.toolKeys {
		if _, keep := desired[toolKey]; keep {
			continue
		}
		if err := r.releaseBridgeToolRefLocked(registry, toolKey); err != nil {
			return err
		}
	}

	for toolKey := range desired {
		if _, exists := state.toolKeys[toolKey]; exists {
			continue
		}
		r.bridgeToolRefCounts[toolKey]++
	}
	state.toolKeys = desired
	r.bridgeSessions[sessionKey] = state

	return nil
}

func (r *Runner) clearSessionSkillBridges(sess *session.Session) {
	if r == nil {
		return
	}
	registry, ok := r.registry.(*toolspkg.Registry)
	if !ok || registry == nil {
		return
	}
	sessionKey := bridgeSessionKey(sess)
	if sessionKey == "" {
		return
	}

	r.bridgeMu.Lock()
	defer r.bridgeMu.Unlock()
	turns := r.bridgeSessionTurns[sessionKey]
	if turns <= 0 {
		return
	}
	turns--
	if turns > 0 {
		r.bridgeSessionTurns[sessionKey] = turns
		return
	}
	delete(r.bridgeSessionTurns, sessionKey)
	_ = r.clearSessionBridgesLocked(registry, sessionKey)
}

func (r *Runner) ensureBridgeStateLocked() {
	if r.bridgeSessions == nil {
		r.bridgeSessions = map[string]bridgeSessionState{}
	}
	if r.bridgeSessionTurns == nil {
		r.bridgeSessionTurns = map[string]int{}
	}
	if r.bridgeToolRefCounts == nil {
		r.bridgeToolRefCounts = map[string]int{}
	}
}

func (r *Runner) clearSessionBridgesLocked(registry *toolspkg.Registry, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	state, ok := r.bridgeSessions[sessionKey]
	if !ok {
		return nil
	}
	if err := r.releaseBridgeToolsLocked(registry, state.toolKeys); err != nil {
		return err
	}
	delete(r.bridgeSessions, sessionKey)
	return nil
}

func (r *Runner) releaseBridgeToolsLocked(registry *toolspkg.Registry, toolKeys map[string]struct{}) error {
	for toolKey := range toolKeys {
		if err := r.releaseBridgeToolRefLocked(registry, toolKey); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) releaseBridgeToolRefLocked(registry *toolspkg.Registry, toolKey string) error {
	toolKey = strings.TrimSpace(toolKey)
	if toolKey == "" {
		return nil
	}
	refs := r.bridgeToolRefCounts[toolKey]
	if refs <= 1 {
		delete(r.bridgeToolRefCounts, toolKey)
		if err := registry.Unregister(toolKey); err != nil {
			var regErr *toolspkg.RegistryError
			if !errors.As(err, &regErr) || regErr.Code != toolspkg.RegistryErrorNotFound {
				return err
			}
		}
		return nil
	}
	r.bridgeToolRefCounts[toolKey] = refs - 1
	return nil
}

func bridgeSessionKey(sess *session.Session) string {
	if sess == nil {
		return ""
	}
	return strings.TrimSpace(sess.ID)
}

func normalizeBridgePolicyItem(item string) string {
	return strings.TrimSpace(item)
}

func shouldBridgePolicyItem(item string) bool {
	if item == "" {
		return false
	}
	if strings.Contains(item, ":") {
		return false
	}
	_, blocked := nonBridgedBuiltinTools[strings.ToLower(item)]
	return !blocked
}
