package subagents

import frontmatterpkg "github.com/1024XEngineer/bytemind/internal/frontmatter"

var subagentFrontmatterOptions = frontmatterpkg.ParseOptions{
	TreatEmptyValueAsMultiline: true,
}

func parseFrontmatterMarkdown(content string) (map[string]string, string) {
	return frontmatterpkg.ParseMarkdown(content, subagentFrontmatterOptions)
}

func parseFrontmatterFields(raw string) map[string]string {
	return frontmatterpkg.ParseFields(raw, subagentFrontmatterOptions)
}

func trimOuterQuotes(value string) string {
	return frontmatterpkg.TrimOuterQuotes(value)
}
