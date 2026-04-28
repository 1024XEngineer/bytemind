package extensions

import (
	"fmt"
	"time"
)

type lifecycleTransition struct {
	from   ExtensionStatus
	to     ExtensionStatus
	event  string
	reason string
	code   ErrorCode
}

var mcpLifecycleTransitions = map[ExtensionStatus]map[ExtensionStatus]struct{}{
	ExtensionStatusLoaded: {
		ExtensionStatusReady:   {},
		ExtensionStatusStopped: {},
	},
	ExtensionStatusReady: {
		ExtensionStatusActive:  {},
		ExtensionStatusStopped: {},
	},
	ExtensionStatusActive: {
		ExtensionStatusDegraded: {},
		ExtensionStatusFailed:   {},
		ExtensionStatusStopped:  {},
	},
	ExtensionStatusDegraded: {
		ExtensionStatusReady:   {},
		ExtensionStatusStopped: {},
	},
	ExtensionStatusFailed: {
		ExtensionStatusReady:   {},
		ExtensionStatusStopped: {},
	},
	ExtensionStatusStopped: {},
}

func CanTransitionMCP(from, to ExtensionStatus) bool {
	next, ok := mcpLifecycleTransitions[from]
	if !ok {
		return false
	}
	_, allowed := next[to]
	return allowed
}

func ValidateMCPTransition(from, to ExtensionStatus) error {
	if CanTransitionMCP(from, to) {
		return nil
	}
	return wrapError(ErrCodeInvalidTransition, fmt.Sprintf("invalid mcp lifecycle transition: %s -> %s", from, to), nil)
}

func activateTransition(info ExtensionInfo) (ExtensionInfo, ExtensionEvent, error) {
	return applyTransition(info, lifecycleTransition{
		from:   ExtensionStatusLoaded,
		to:     ExtensionStatusActive,
		event:  "activate",
		reason: "extension activated",
	})
}

func degradeTransition(info ExtensionInfo, reason string, code ErrorCode) (ExtensionInfo, ExtensionEvent, error) {
	if reason == "" {
		reason = "extension degraded"
	}
	return applyTransition(info, lifecycleTransition{
		from:   ExtensionStatusActive,
		to:     ExtensionStatusDegraded,
		event:  "degraded",
		reason: reason,
		code:   code,
	})
}

func recoverTransition(info ExtensionInfo, reason string) (ExtensionInfo, ExtensionEvent, error) {
	if reason == "" {
		reason = "extension recovered"
	}
	return applyTransition(info, lifecycleTransition{
		from:   ExtensionStatusDegraded,
		to:     ExtensionStatusActive,
		event:  "recover",
		reason: reason,
	})
}

func stopTransition(info ExtensionInfo, reason string) (ExtensionInfo, ExtensionEvent, error) {
	if reason == "" {
		reason = "extension stopped"
	}
	switch info.Status {
	case ExtensionStatusLoaded, ExtensionStatusActive, ExtensionStatusDegraded:
		return applyTransition(info, lifecycleTransition{
			from:   info.Status,
			to:     ExtensionStatusStopped,
			event:  "unload",
			reason: reason,
		})
	default:
		return ExtensionInfo{}, ExtensionEvent{}, wrapError(ErrCodeInvalidTransition, fmt.Sprintf("invalid lifecycle transition: %s -> %s", info.Status, ExtensionStatusStopped), nil)
	}
}

func applyTransition(info ExtensionInfo, transition lifecycleTransition) (ExtensionInfo, ExtensionEvent, error) {
	if info.Status != transition.from {
		return ExtensionInfo{}, ExtensionEvent{}, wrapError(ErrCodeInvalidTransition, fmt.Sprintf("invalid lifecycle transition: %s -> %s", info.Status, transition.to), nil)
	}
	next := cloneExtensionInfo(info)
	next.Status = transition.to
	next.Health.Status = transition.to
	next.Health.Message = transition.reason
	next.Health.LastError = transition.code
	next.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	event := ExtensionEvent{
		Type:        transition.event,
		ExtensionID: next.ID,
		Kind:        next.Kind,
		Status:      next.Status,
		Reason:      transition.reason,
		ErrorCode:   transition.code,
		OccurredAt:  next.Health.CheckedAtUTC,
		Message:     transition.reason,
	}
	return next, event, nil
}
