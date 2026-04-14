package tui

import (
	"fmt"
	"os"
)

func warnSetenv(name string, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[bytemind][tui] warning: failed to set %s: %v\n", name, err)
}
