package agent

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	extensionspkg "bytemind/internal/extensions"
	toolspkg "bytemind/internal/tools"
)

func (r *Runner) syncExtensionTools(ctx context.Context, force bool) error {
	if r == nil {
		return nil
	}
	registry, ok := r.registry.(*toolspkg.Registry)
	if !ok || registry == nil {
		return nil
	}
	resolver, ok := r.extensions.(extensionspkg.ToolResolver)
	if !ok {
		return nil
	}

	now := time.Now()
	r.extensionSyncMu.Lock()
	if !force && !r.extensionSyncDirty && !r.extensionSyncAt.IsZero() && now.Sub(r.extensionSyncAt) < r.extensionSyncTTL {
		r.extensionSyncMu.Unlock()
		return nil
	}
	currentKeys := cloneExtensionToolKeys(r.extensionToolKeys)
	startSyncGen := r.extensionSyncGen
	r.extensionSyncMu.Unlock()

	resolvedTools, resolveErr := resolver.ResolveAllTools(ctx)
	if resolveErr != nil && (errors.Is(resolveErr, context.Canceled) || errors.Is(resolveErr, context.DeadlineExceeded)) {
		return resolveErr
	}

	nextKeys := map[string]map[string]struct{}{}
	var firstErr error
	for _, extensionTool := range resolvedTools {
		if extensionTool.Source != extensionspkg.ExtensionMCP {
			continue
		}
		extensionID := strings.TrimSpace(extensionTool.ExtensionID)
		toolName := strings.TrimSpace(extensionTool.Tool.Definition().Function.Name)
		if stable, err := extensionspkg.StableToolKey(extensionTool.Source, extensionID, toolName); err == nil && stable != "" {
			if current := currentKeys[extensionID]; current != nil {
				if _, already := current[stable]; already {
					if _, ok := registry.Get(stable); ok {
						if _, ok := nextKeys[extensionID]; !ok {
							nextKeys[extensionID] = map[string]struct{}{}
						}
						nextKeys[extensionID][stable] = struct{}{}
						continue
					}
				}
			}
		}
		binding, err := extensionspkg.RegisterBridgedTool(registry, extensionTool)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		extensionID = strings.TrimSpace(binding.ExtensionID)
		if extensionID == "" {
			extensionID = strings.TrimSpace(extensionTool.ExtensionID)
		}
		if extensionID == "" {
			continue
		}
		if _, ok := nextKeys[extensionID]; !ok {
			nextKeys[extensionID] = map[string]struct{}{}
		}
		nextKeys[extensionID][binding.StableKey] = struct{}{}
	}

	r.extensionSyncMu.Lock()
	defer r.extensionSyncMu.Unlock()

	for extensionID, current := range r.extensionToolKeys {
		next := nextKeys[extensionID]
		for stable := range current {
			if _, keep := next[stable]; keep {
				continue
			}
			if err := unregisterStableToolKey(registry, stable); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	for extensionID, next := range nextKeys {
		if _, ok := r.extensionToolKeys[extensionID]; ok {
			continue
		}
		// Ensure deterministic map copy for test visibility.
		copied := make([]string, 0, len(next))
		for stable := range next {
			copied = append(copied, stable)
		}
		sort.Strings(copied)
		rebuilt := make(map[string]struct{}, len(copied))
		for _, stable := range copied {
			rebuilt[stable] = struct{}{}
		}
		nextKeys[extensionID] = rebuilt
	}

	r.extensionToolKeys = nextKeys
	r.extensionSyncAt = time.Now()
	r.extensionSyncDirty = r.extensionSyncGen != startSyncGen

	if firstErr != nil {
		return firstErr
	}
	return resolveErr
}

func cloneExtensionToolKeys(input map[string]map[string]struct{}) map[string]map[string]struct{} {
	if input == nil {
		return nil
	}
	out := make(map[string]map[string]struct{}, len(input))
	for extensionID, stableSet := range input {
		if stableSet == nil {
			out[extensionID] = nil
			continue
		}
		copied := make(map[string]struct{}, len(stableSet))
		for stable := range stableSet {
			copied[stable] = struct{}{}
		}
		out[extensionID] = copied
	}
	return out
}

func unregisterStableToolKey(registry *toolspkg.Registry, stable string) error {
	if registry == nil {
		return nil
	}
	err := registry.Unregister(stable)
	if err == nil {
		return nil
	}
	var registryErr *toolspkg.RegistryError
	if errors.As(err, &registryErr) && registryErr.Code == toolspkg.RegistryErrorNotFound {
		return nil
	}
	return err
}

func (r *Runner) invalidateExtensionTools(extensionID string) {
	if r == nil {
		return
	}
	if invalidator, ok := r.extensions.(extensionspkg.Invalidator); ok {
		invalidator.Invalidate(extensionID)
	}
	r.extensionSyncMu.Lock()
	defer r.extensionSyncMu.Unlock()
	r.extensionSyncDirty = true
	r.extensionSyncGen++
}
