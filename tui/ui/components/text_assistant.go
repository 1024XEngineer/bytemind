package tui

import "strings"

var assistantInlineTokenReplacer = strings.NewReplacer(
	"**", "",
	"__", "",
	"~~", "",
	"`", "",
)
