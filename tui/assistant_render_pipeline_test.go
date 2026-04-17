package tui

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestPreprocessAndPostprocessSemanticTags(t *testing.T) {
	originalNoColor := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = originalNoColor }()

	raw := "<info>hello</info> then <error>boom</error>"
	prepared, records := preprocessSemanticTags(raw)

	if strings.Contains(prepared, "<info>") || strings.Contains(prepared, "<error>") {
		t.Fatalf("expected semantic xml tags to be removed from prepared text, got %q", prepared)
	}
	if len(records) != 2 {
		t.Fatalf("expected two tag records, got %d", len(records))
	}

	rendered := postprocessSemanticTags(prepared, records)
	if !strings.Contains(rendered, "hello") || !strings.Contains(rendered, "boom") {
		t.Fatalf("expected restored semantic content after postprocess, got %q", rendered)
	}
	if strings.Contains(rendered, "ZZBYTEMINDTAG") {
		t.Fatalf("expected placeholders to be fully replaced, got %q", rendered)
	}
}

func TestBlueGlamourStyleOverridesCoreBlueThemeSlots(t *testing.T) {
	cfg := blueGlamourStyle()

	if cfg.Document.BlockPrefix != "" {
		t.Fatalf("expected document block prefix override, got %q", cfg.Document.BlockPrefix)
	}
	if cfg.Document.Color == nil || *cfg.Document.Color != "#D8E4F0" {
		t.Fatalf("expected document color override, got %+v", cfg.Document.Color)
	}
	if cfg.BlockQuote.IndentToken == nil || *cfg.BlockQuote.IndentToken != "\u258e " {
		t.Fatalf("expected block quote indent token override, got %+v", cfg.BlockQuote.IndentToken)
	}
	if cfg.Strong.Color == nil || *cfg.Strong.Color != "#F6FBFF" {
		t.Fatalf("expected strong text color override, got %+v", cfg.Strong.Color)
	}
	if cfg.Emph.Color == nil || *cfg.Emph.Color != "#8FDFFF" {
		t.Fatalf("expected emphasis color override, got %+v", cfg.Emph.Color)
	}
	if cfg.LinkText.Color == nil || *cfg.LinkText.Color != "#7FE6FF" {
		t.Fatalf("expected link text color override, got %+v", cfg.LinkText.Color)
	}
	if cfg.Code.Color == nil || *cfg.Code.Color != "#E6B873" {
		t.Fatalf("expected inline code color override, got %+v", cfg.Code.Color)
	}
	if cfg.Code.BackgroundColor != nil {
		t.Fatalf("expected inline code background to be unset, got %+v", cfg.Code.BackgroundColor)
	}
	if cfg.CodeBlock.BackgroundColor == nil || *cfg.CodeBlock.BackgroundColor != "#000000" {
		t.Fatalf("expected code block background override, got %+v", cfg.CodeBlock.BackgroundColor)
	}
	if cfg.CodeBlock.Chroma == nil {
		t.Fatal("expected code block chroma overrides")
	}
	if cfg.CodeBlock.Chroma.Keyword.Color == nil || *cfg.CodeBlock.Chroma.Keyword.Color != "#7CB8FF" {
		t.Fatalf("expected code keyword color override, got %+v", cfg.CodeBlock.Chroma.Keyword.Color)
	}
	if cfg.CodeBlock.Chroma.KeywordType.Color == nil || *cfg.CodeBlock.Chroma.KeywordType.Color != "#9B8CFF" {
		t.Fatalf("expected code type color override, got %+v", cfg.CodeBlock.Chroma.KeywordType.Color)
	}
	if cfg.CodeBlock.Chroma.NameFunction.Color == nil || *cfg.CodeBlock.Chroma.NameFunction.Color != "#7EE0B5" {
		t.Fatalf("expected code function color override, got %+v", cfg.CodeBlock.Chroma.NameFunction.Color)
	}
	if cfg.CodeBlock.Chroma.LiteralString.Color == nil || *cfg.CodeBlock.Chroma.LiteralString.Color != "#E6B873" {
		t.Fatalf("expected code string color override, got %+v", cfg.CodeBlock.Chroma.LiteralString.Color)
	}
	if cfg.CodeBlock.Chroma.LiteralNumber.Color == nil || *cfg.CodeBlock.Chroma.LiteralNumber.Color != "#8DE1C5" {
		t.Fatalf("expected code number color override, got %+v", cfg.CodeBlock.Chroma.LiteralNumber.Color)
	}
	if cfg.CodeBlock.Chroma.Comment.Color == nil || *cfg.CodeBlock.Chroma.Comment.Color != "#6F8197" {
		t.Fatalf("expected code comment color override, got %+v", cfg.CodeBlock.Chroma.Comment.Color)
	}
	if cfg.Item.BlockPrefix != "\u2022 " {
		t.Fatalf("expected list item prefix override, got %q", cfg.Item.BlockPrefix)
	}
	if cfg.Heading.Color == nil || *cfg.Heading.Color != "#6CB6FF" {
		t.Fatalf("expected heading color override, got %+v", cfg.Heading.Color)
	}
	if cfg.H1.Prefix != "\u258d " {
		t.Fatalf("expected h1 prefix override, got %q", cfg.H1.Prefix)
	}
	if cfg.H2.Prefix != "\u25c6 " {
		t.Fatalf("expected h2 prefix override, got %q", cfg.H2.Prefix)
	}
	if cfg.H3.Prefix != "\u25b8 " {
		t.Fatalf("expected h3 prefix override, got %q", cfg.H3.Prefix)
	}
	if cfg.Table.ColumnSeparator == nil || *cfg.Table.ColumnSeparator != "\u2502" {
		t.Fatalf("expected table column separator override, got %+v", cfg.Table.ColumnSeparator)
	}
}
