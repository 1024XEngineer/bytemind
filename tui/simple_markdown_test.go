package tui

import (
	"testing"
)

func TestSimpleMarkdownRenderer(t *testing.T) {
	renderer := NewSimpleMarkdownRenderer(80)

	// Test basic markdown
	markdown := "# Heading 1\n\nThis is a paragraph with **bold** and *italic* text.\n\n## Heading 2\n\n- Item 1\n- Item 2\n- Item 3\n\n```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```\n\n> This is a quote\n"

	result := renderer.Render(markdown)
	if result == "" {
		t.Error("Render returned empty string")
	}

	// Test caching
	result2 := renderer.Render(markdown)
	if result != result2 {
		t.Error("Caching failed, results differ")
	}
}
