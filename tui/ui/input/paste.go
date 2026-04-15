package tui

import (
	"regexp"
	"time"

	tuiruntime "bytemind/internal/tui/runtime"
)

const (
	maxStoredPastedContents     = 10
	longPasteLineThreshold      = 10
	longPasteCharThreshold      = 500
	pasteQuickCharThreshold     = 80
	pasteBurstImmediateMinChars = 12
	flattenedPasteCharThreshold = 180
	pasteBurstCharThreshold     = 120
	pasteBurstWindow            = 80 * time.Millisecond
	pasteContinuationWindow     = 900 * time.Millisecond
	maxSinglePastedCharLength   = 200000
	pasteSoftWrapWidth          = 72
)

var compressedPasteMarkerPattern = regexp.MustCompile(`^\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]$`)
var compressedPasteMarkerPrefixPattern = regexp.MustCompile(`^\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]`)
var compressedPasteMarkerChainPrefixPattern = regexp.MustCompile(`^(?:(?:\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\])\s*)+`)
var compressedPasteMarkerAnyPattern = regexp.MustCompile(`\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]`)
var compressedPasteMarkerDetailsPattern = regexp.MustCompile(`\[(?:Paste|Pasted)\s+#(\d+)\s+~(\d+)\s+lines\]`)
var pastedRefPattern = regexp.MustCompile(`\[(?:Paste|Pasted)(?:\s+#(\d+))?(?:\s+~(\d+)\s+lines)?(?:\s+line(\d+)(?:~line(\d+))?)?\]`)

type pastedContent = tuiruntime.PastedContent

type compressedMarkerLoc struct {
	id    string
	start int
	end   int
	ok    bool
}
