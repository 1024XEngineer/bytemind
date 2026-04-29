package main

import (
	"fmt"
	"io"
	"os"

	"github.com/1024XEngineer/bytemind/internal/app"
)

func main() {
	configureConsoleEncoding()
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return app.RunCLI(args, stdin, stdout, stderr)
}
