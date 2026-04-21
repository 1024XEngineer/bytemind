package app

import (
	"context"
	"errors"
	"flag"
	"io"

	"bytemind/internal/tools"
)

func RunWorkerArgs(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sandboxStdio := fs.Bool("sandbox-stdio", false, "Run sandbox worker protocol over stdio")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*sandboxStdio {
		return errors.New("worker command requires --sandbox-stdio")
	}
	return tools.RunWorkerProcess(context.Background(), stdin, stdout)
}
