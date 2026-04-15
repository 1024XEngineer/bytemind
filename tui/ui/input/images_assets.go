package tui

import (
	"regexp"

	"bytemind/internal/llm"
	"bytemind/internal/session"
	tuiruntime "bytemind/internal/tui/runtime"
	tuiservices "bytemind/internal/tui/services"
)

var imagePlaceholderPattern = regexp.MustCompile(`\[Image #(\d+)\]`)
var imageMentionPattern = regexp.MustCompile(`(?i)@([^\s@]+?\.(?:png|jpe?g|webp|gif))`)
var inlineWindowsImagePathPattern = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n\t"'<>|]*?\.(?:png|jpe?g|webp|gif)`)
var inlineUnixImagePathPattern = regexp.MustCompile(`(?i)/(?:[^\r\n\t"'<>|/]+/)*[^\r\n\t"'<>|/]+\.(?:png|jpe?g|webp|gif)`)

type inputMutationClass string

const (
	inputMutationOrdinary    inputMutationClass = "ordinary"
	inputMutationPasteEmpty  inputMutationClass = "paste_empty"
	inputMutationPasteFilled inputMutationClass = "paste_filled"

	ctrlVKeyName    = "ctrl+v"
	ctrlVMarkerRune = "\x16"
)

type mentionImageSpan struct {
	Start   int
	End     int
	AssetID llm.AssetID
	Raw     string
}

type imagePathSpan struct {
	Start int
	End   int
	Path  string
}

func nextSessionImageID(sess *session.Session) int {
	return tuiruntime.NextSessionImageID(sess)
}

func (m *model) imageInputService() *tuiservices.ImageInputController {
	if m == nil {
		return nil
	}
	if m.imageInputController == nil {
		m.imageInputController = tuiservices.NewImageInputController()
	}
	return m.imageInputController
}
