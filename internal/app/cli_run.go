package app

import "io"

// RunCLI executes the default ByteMind CLI wiring.
func RunCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return DispatchCLI(args, stdin, stdout, stderr, DispatchHandlers{
		RunTUI:      RunTUIArgs,
		RunOneShot:  RunOneShotArgs,
		RunWorker:   RunWorkerArgs,
		RunInstall:  RunInstall,
		RunMCP:      RunMCP,
		RenderUsage: RenderUsage,
	})
}
