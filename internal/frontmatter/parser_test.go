package frontmatter

import (
	"reflect"
	"testing"
)

func TestParseMarkdownWithoutFrontmatterReturnsTrimmedBody(t *testing.T) {
	fields, body := ParseMarkdown("  hello world \n", ParseOptions{})
	if len(fields) != 0 {
		t.Fatalf("expected empty fields, got %#v", fields)
	}
	if body != "hello world" {
		t.Fatalf("expected trimmed body, got %q", body)
	}
}

func TestParseMarkdownParsesFieldsAndBody(t *testing.T) {
	content := "---\nname: review\ndescription: \"Code review\"\n---\n\nbody text\n"
	fields, body := ParseMarkdown(content, ParseOptions{})

	expected := map[string]string{
		"name":        "review",
		"description": "Code review",
	}
	if !reflect.DeepEqual(fields, expected) {
		t.Fatalf("unexpected fields: got %#v want %#v", fields, expected)
	}
	if body != "body text" {
		t.Fatalf("expected parsed body, got %q", body)
	}
}

func TestParseFieldsTreatsEmptyAsMultilineWhenEnabled(t *testing.T) {
	raw := "name: review\nnotes:\n  first\n  second\n"
	fields := ParseFields(raw, ParseOptions{TreatEmptyValueAsMultiline: true})
	if fields["notes"] != "first\nsecond" {
		t.Fatalf("expected multiline notes, got %q", fields["notes"])
	}
}

func TestParseFieldsTreatsEmptyAsEmptyStringWhenDisabled(t *testing.T) {
	raw := "name: review\nnotes:\n  first\n"
	fields := ParseFields(raw, ParseOptions{})
	if value, ok := fields["notes"]; !ok || value != "" {
		t.Fatalf("expected empty string note field, got %#v", fields)
	}
}

func TestTrimOuterQuotes(t *testing.T) {
	cases := map[string]string{
		"plain":    "plain",
		"\"x\"":    "x",
		"'y'":      "y",
		"\"unterm": "\"unterm",
	}
	for input, want := range cases {
		if got := TrimOuterQuotes(input); got != want {
			t.Fatalf("unexpected trim result for %q: got %q want %q", input, got, want)
		}
	}
}
