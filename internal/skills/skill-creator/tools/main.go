package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcommand {
	case "quick-validate":
		err = runQuickValidate(args)
	case "package-skill":
		err = runPackageSkill(args)
	case "aggregate-benchmark":
		err = runAggregateBenchmark(args)
	case "generate-report":
		err = runGenerateReport(args)
	case "run-eval":
		err = runRunEval(args)
	case "improve-description":
		err = runImproveDescription(args)
	case "run-loop":
		err = runRunLoop(args)
	case "generate-review":
		err = runGenerateReview(args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		printUsage()
		err = fmt.Errorf("unknown subcommand: %s", subcommand)
	}

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlagSet(name string, args []string) (*flag.FlagSet, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return fs, nil
}

func printUsage() {
	_, _ = fmt.Fprintln(os.Stderr, "Skill Creator Go tools")
	_, _ = fmt.Fprintln(os.Stderr, "Usage: go run ./internal/skills/skill-creator/tools <subcommand> [flags]")
	_, _ = fmt.Fprintln(os.Stderr, "Subcommands:")
	_, _ = fmt.Fprintln(os.Stderr, "  quick-validate")
	_, _ = fmt.Fprintln(os.Stderr, "  package-skill")
	_, _ = fmt.Fprintln(os.Stderr, "  aggregate-benchmark")
	_, _ = fmt.Fprintln(os.Stderr, "  generate-report")
	_, _ = fmt.Fprintln(os.Stderr, "  run-eval")
	_, _ = fmt.Fprintln(os.Stderr, "  improve-description")
	_, _ = fmt.Fprintln(os.Stderr, "  run-loop")
	_, _ = fmt.Fprintln(os.Stderr, "  generate-review")
}
