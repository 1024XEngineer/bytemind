package app

import (
	"io"

	"bytemind/tui"
)

func RunTUIArgs(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return RunTUI(TUIRequest{
		Args:   args,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}, nil)
}

func RunTUI(req TUIRequest, runProgram func(tui.Options) error) error {
	runtime, err := BuildTUIRuntime(req)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()

	if runProgram == nil {
		runProgram = tui.Run
	}
	return runProgram(runtime.Options)
}
