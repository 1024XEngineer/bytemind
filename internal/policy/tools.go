package policy

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/skills"
)

const EmptyAllowlistSentinel = "__bytemind__no_tools__"

// ResolveToolSets maps active skill tool policy into allow/deny sets.
func ResolveToolSets(policy skills.ToolPolicy) (map[string]struct{}, map[string]struct{}) {
	items := policy.Items
	switch policy.Policy {
	case skills.ToolPolicyAllowlist:
		if len(items) == 0 {
			return map[string]struct{}{EmptyAllowlistSentinel: {}}, nil
		}
		allow := toSet(items)
		if allow == nil {
			return map[string]struct{}{EmptyAllowlistSentinel: {}}, nil
		}
		return allow, nil
	case skills.ToolPolicyDenylist:
		return nil, toSet(items)
	default:
		return nil, nil
	}
}

func SortedToolNames(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}
