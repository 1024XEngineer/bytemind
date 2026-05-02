package skills

import frontmatterpkg "github.com/1024XEngineer/bytemind/internal/frontmatter"

func parseFrontmatterMarkdown(content string) (map[string]string, string) {
	return frontmatterpkg.ParseMarkdown(content, frontmatterpkg.ParseOptions{})
}

func parseFrontmatterFields(raw string) map[string]string {
	return frontmatterpkg.ParseFields(raw, frontmatterpkg.ParseOptions{})
}

func trimOuterQuotes(value string) string {
	return frontmatterpkg.TrimOuterQuotes(value)
}
