package tools

import (
	"runtime"
	"strings"
)

type sandboxEnvOptions struct {
	GOOS          string
	RequiredMode  bool
	DropSensitive bool
	AlwaysDrop    map[string]struct{}
	ForceSet      map[string]string
}

func buildSandboxEnv(base []string, opts sandboxEnvOptions) []string {
	goos := strings.ToLower(strings.TrimSpace(opts.GOOS))
	if goos == "" {
		goos = runtime.GOOS
	}
	allow := map[string]struct{}{}
	if opts.RequiredMode {
		allow = requiredSandboxEnvAllowlist(goos)
	}

	drop := normalizeEnvNameSet(opts.AlwaysDrop)
	force := normalizeEnvValueMap(opts.ForceSet)

	type kv struct {
		name  string
		value string
	}
	collected := make([]kv, 0, len(base)+len(force))
	seen := map[string]struct{}{}

	for _, raw := range base {
		name, value, ok := splitEnvKV(raw)
		if !ok {
			continue
		}
		if _, blocked := drop[name]; blocked {
			continue
		}
		if opts.DropSensitive && isSensitiveEnvName(name) {
			continue
		}
		if opts.RequiredMode {
			if _, ok := allow[name]; !ok {
				continue
			}
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		collected = append(collected, kv{name: name, value: value})
	}

	for name, value := range force {
		replaced := false
		for i := range collected {
			if collected[i].name == name {
				collected[i].value = value
				replaced = true
				break
			}
		}
		if !replaced {
			collected = append(collected, kv{name: name, value: value})
		}
	}

	out := make([]string, 0, len(collected))
	for _, item := range collected {
		out = append(out, item.name+"="+item.value)
	}
	return out
}

func normalizeEnvNameSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(source))
	for key := range source {
		key = normalizeEnvName(key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func normalizeEnvValueMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		key = normalizeEnvName(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func splitEnvKV(raw string) (name, value string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	name, value, ok = strings.Cut(raw, "=")
	if !ok {
		return "", "", false
	}
	name = normalizeEnvName(name)
	if name == "" {
		return "", "", false
	}
	return name, value, true
}

func normalizeEnvName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToUpper(name)
}

func requiredSandboxEnvAllowlist(goos string) map[string]struct{} {
	allowed := map[string]struct{}{
		"PATH":     {},
		"HOME":     {},
		"TMP":      {},
		"TEMP":     {},
		"TMPDIR":   {},
		"TERM":     {},
		"LANG":     {},
		"LC_ALL":   {},
		"LC_CTYPE": {},
		"SHELL":    {},
		"PWD":      {},
		"USER":     {},
		"USERNAME": {},
	}
	if goos == "windows" {
		allowed["SYSTEMROOT"] = struct{}{}
		allowed["WINDIR"] = struct{}{}
		allowed["COMSPEC"] = struct{}{}
		allowed["PATHEXT"] = struct{}{}
		allowed["USERPROFILE"] = struct{}{}
	}
	return allowed
}

func isSensitiveEnvName(name string) bool {
	name = normalizeEnvName(name)
	if name == "" {
		return false
	}
	sensitiveHints := []string{
		"API_KEY",
		"TOKEN",
		"SECRET",
		"PASSWORD",
		"PASSWD",
	}
	for _, hint := range sensitiveHints {
		if strings.Contains(name, hint) {
			return true
		}
	}
	return false
}
