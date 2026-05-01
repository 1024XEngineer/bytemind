package skills

import frontmatterpkg "bytemind/internal/frontmatter"

func parseFrontmatterMarkdown(content string) (map[string]string, string) {
	return frontmatterpkg.ParseMarkdown(content, frontmatterpkg.ParseOptions{})
}

func parseFrontmatterFields(raw string) map[string]string {
	return frontmatterpkg.ParseFields(raw, frontmatterpkg.ParseOptions{})
}

func trimOuterQuotes(value string) string {
	return frontmatterpkg.TrimOuterQuotes(value)
}
