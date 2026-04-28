package app

import (
	"fmt"
	"io"
)

func RenderVersion(w io.Writer) {
	fmt.Fprintln(w, CurrentVersion())
}
