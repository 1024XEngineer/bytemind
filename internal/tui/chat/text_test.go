package chat

import (
	"strings"
	"testing"
)

func TestWrapPlainTextAndLineSmart(t *testing.T) {
	text := "hello world from bytemind"
	got := WrapPlainText(text, 6)
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected wrapped text to span multiple lines, got %q", got)
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len([]rune(line)) > 6 {
			t.Fatalf("expected line width <= 6, got %q", line)
		}
	}

	if single := WrapLineSmart("abc", 0); len(single) != 1 || single[0] != "abc" {
		t.Fatalf("expected width<=0 to return original line, got %#v", single)
	}
	if wide := WrapLineSmart("你好", 1); len(wide) != 2 {
		t.Fatalf("expected wide runes to split safely, got %#v", wide)
	}
}

func TestTidyAssistantSpacingAndBody(t *testing.T) {
	in := "line\n## Heading\n- item\n```go\nx := 1\n```\nend"
	tidied := TidyAssistantSpacing(in)
	if !strings.Contains(tidied, "\n\n## Heading") {
		t.Fatalf("expected heading to be separated by blank line, got %q", tidied)
	}

	rendered := RenderAssistantBody("## Title\n- [x] **done**\n[Doc](https://x.test)", 80)
	if strings.Contains(rendered, "##") || strings.Contains(rendered, "**") {
		t.Fatalf("expected markdown tokens stripped in assistant body, got %q", rendered)
	}
	if !strings.Contains(rendered, "Doc (https://x.test)") {
		t.Fatalf("expected links to be normalized, got %q", rendered)
	}
}

func TestMarkdownHelpers(t *testing.T) {
	if !NeedsLeadingBlankLine("# h1") || NeedsLeadingBlankLine("plain text") {
		t.Fatalf("unexpected heading blank-line decision")
	}

	if got := NormalizeAssistantMarkdownLine("> ## Heading"); got != "Heading" {
		t.Fatalf("expected quote+heading normalized, got %q", got)
	}
	if got := NormalizeAssistantMarkdownLine("| a | b |"); got != "a | b" {
		t.Fatalf("expected markdown table line normalized, got %q", got)
	}
	if got := NormalizeAssistantMarkdownLine("| --- | :---: |"); got != "" {
		t.Fatalf("expected table divider removed, got %q", got)
	}

	if marker, rest, ok := SplitOrderedListItem("12. step"); !ok || marker != "12." || rest != "step" {
		t.Fatalf("unexpected ordered list parse result marker=%q rest=%q ok=%v", marker, rest, ok)
	}
	if _, _, ok := SplitOrderedListItem("x. nope"); ok {
		t.Fatalf("expected invalid ordered marker to fail parse")
	}

	if !IsMarkdownTableDivider("| --- | :---: |") || IsMarkdownTableDivider("| a | b |") {
		t.Fatalf("unexpected table divider detection")
	}
	if got := StripMarkdownLinks("see [Doc](https://x.test) and ![img](https://img.test)"); got != "see Doc (https://x.test) and img" {
		t.Fatalf("unexpected markdown link stripping: %q", got)
	}
	if !LooksLikeMarkdownTable("| a | b |") || LooksLikeMarkdownTable("not table") {
		t.Fatalf("unexpected markdown table shape detection")
	}
}
