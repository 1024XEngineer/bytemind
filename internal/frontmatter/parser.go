package frontmatter

import "strings"

type ParseOptions struct {
	TreatEmptyValueAsMultiline bool
}

func ParseMarkdown(content string, options ParseOptions) (map[string]string, string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	if !strings.HasPrefix(content, "---\n") {
		return map[string]string{}, strings.TrimSpace(content)
	}

	rest := strings.TrimPrefix(content, "---\n")
	sep := "\n---\n"
	idx := strings.Index(rest, sep)
	if idx < 0 {
		return map[string]string{}, strings.TrimSpace(content)
	}

	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+len(sep):])
	return ParseFields(frontmatter, options), body
}

func ParseFields(raw string, options ParseOptions) map[string]string {
	fields := map[string]string{}
	lines := strings.Split(raw, "\n")

	var multiKey string
	var multi []string
	flushMulti := func() {
		if multiKey == "" {
			return
		}
		fields[multiKey] = strings.TrimSpace(strings.Join(multi, "\n"))
		multiKey = ""
		multi = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if multiKey != "" {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || trimmed == "" {
				multi = append(multi, strings.TrimSpace(line))
				continue
			}
			flushMulti()
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		if value == "|" || value == ">" {
			multiKey = key
			multi = multi[:0]
			continue
		}
		if options.TreatEmptyValueAsMultiline && value == "" {
			multiKey = key
			multi = multi[:0]
			continue
		}
		fields[key] = TrimOuterQuotes(value)
	}

	flushMulti()
	return fields
}

func TrimOuterQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) || (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}
	return value
}
