package app

import (
	"fmt"
	"io"
)

type DispatchHandlers struct {
	RunTUI      func(args []string, stdin io.Reader, stdout, stderr io.Writer) error
	RunOneShot  func(args []string, stdin io.Reader, stdout, stderr io.Writer) error
	RunWorker   func(args []string, stdin io.Reader, stdout, stderr io.Writer) error
	RunInstall  func(args []string, stdout, stderr io.Writer) error
	RenderUsage func(w io.Writer)
}

func DispatchCLI(args []string, stdin io.Reader, stdout, stderr io.Writer, handlers DispatchHandlers) error {
	if handlers.RunTUI == nil || handlers.RunOneShot == nil || handlers.RunWorker == nil || handlers.RunInstall == nil || handlers.RenderUsage == nil {
		return fmt.Errorf("cli dispatch handlers are incomplete")
	}
	if len(args) == 0 {
		return handlers.RunTUI(nil, stdin, stdout, stderr)
	}

	switch args[0] {
	case "chat":
		return handlers.RunTUI(args[1:], stdin, stdout, stderr)
	case "tui":
		return handlers.RunTUI(args[1:], stdin, stdout, stderr)
	case "run":
		return handlers.RunOneShot(args[1:], stdin, stdout, stderr)
	case "worker":
		return handlers.RunWorker(args[1:], stdin, stdout, stderr)
	case "install":
		return handlers.RunInstall(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		handlers.RenderUsage(stdout)
		return nil
	default:
		return handlers.RunTUI(args, stdin, stdout, stderr)
	}
}
