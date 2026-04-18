package extensions

import "strings"

type ExtensionKind string

const (
	ExtensionMCP   ExtensionKind = "mcp"
	ExtensionSkill ExtensionKind = "skill"
)

type ExtensionScope string

const (
	ExtensionScopeBuiltin ExtensionScope = "builtin"
	ExtensionScopeUser    ExtensionScope = "user"
	ExtensionScopeProject ExtensionScope = "project"
	ExtensionScopeRemote  ExtensionScope = "remote"
)

type ExtensionStatus string

const (
	ExtensionStatusUnknown  ExtensionStatus = "unknown"
	ExtensionStatusPending  ExtensionStatus = "pending"
	ExtensionStatusLoaded   ExtensionStatus = "loaded"
	ExtensionStatusActive   ExtensionStatus = "active"
	ExtensionStatusReady    ExtensionStatus = "ready"
	ExtensionStatusDegraded ExtensionStatus = "degraded"
	ExtensionStatusFailed   ExtensionStatus = "failed"
	ExtensionStatusStopped  ExtensionStatus = "stopped"
)

type ExtensionSource struct {
	Scope ExtensionScope
	Ref   string
}

type CapabilitySet struct {
	Prompts   int
	Resources int
	Tools     int
	Commands  int
}

type Manifest struct {
	Name         string
	Version      string
	Title        string
	Description  string
	Kind         ExtensionKind
	Source       ExtensionSource
	Capabilities CapabilitySet
}

type HealthSnapshot struct {
	Status       ExtensionStatus
	Message      string
	LastError    ErrorCode
	CheckedAtUTC string
}

type ExtensionEvent struct {
	Type        string
	ExtensionID string
	Kind        ExtensionKind
	Status      ExtensionStatus
	Reason      string
	ErrorCode   ErrorCode
	OccurredAt  string
	Message     string
}

type ExtensionInfo struct {
	ID           string
	Name         string
	Kind         ExtensionKind
	Version      string
	Title        string
	Description  string
	Source       ExtensionSource
	Status       ExtensionStatus
	Capabilities CapabilitySet
	Manifest     Manifest
	Health       HealthSnapshot
}

func (info ExtensionInfo) Valid() bool {
	if strings.TrimSpace(info.ID) == "" {
		return false
	}
	if strings.TrimSpace(info.Name) == "" {
		return false
	}
	switch info.Kind {
	case ExtensionMCP, ExtensionSkill:
	default:
		return false
	}
	switch info.Source.Scope {
	case ExtensionScopeBuiltin, ExtensionScopeUser, ExtensionScopeProject, ExtensionScopeRemote:
	default:
		return false
	}
	return strings.TrimSpace(info.Source.Ref) != ""
}

func (info ExtensionInfo) IsZero() bool {
	return strings.TrimSpace(info.ID) == "" &&
		strings.TrimSpace(info.Name) == "" &&
		strings.TrimSpace(string(info.Kind)) == "" &&
		strings.TrimSpace(info.Version) == "" &&
		strings.TrimSpace(info.Title) == "" &&
		strings.TrimSpace(info.Description) == "" &&
		strings.TrimSpace(string(info.Source.Scope)) == "" &&
		strings.TrimSpace(info.Source.Ref) == "" &&
		strings.TrimSpace(string(info.Status)) == "" &&
		info.Capabilities == (CapabilitySet{}) &&
		info.Manifest == (Manifest{}) &&
		info.Health == (HealthSnapshot{})
}
