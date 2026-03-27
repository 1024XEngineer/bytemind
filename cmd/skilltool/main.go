package main

import (
	"flag"
	"fmt"
	"os"

	"bytemind/internal/skilltool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "office" {
		printUsage()
		return nil
	}

	switch args[1] {
	case "unpack":
		return runOfficeUnpack(args[2:])
	case "pack":
		return runOfficePack(args[2:])
	default:
		printUsage()
		return nil
	}
}

func runOfficeUnpack(args []string) error {
	fs := flag.NewFlagSet("office unpack", flag.ContinueOnError)
	input := fs.String("in", "", "Input office file (.docx/.pptx/.xlsx)")
	output := fs.String("out", "", "Output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("office unpack requires -in and -out")
	}
	return skilltool.UnpackOfficeArchive(*input, *output)
}

func runOfficePack(args []string) error {
	fs := flag.NewFlagSet("office pack", flag.ContinueOnError)
	input := fs.String("in", "", "Input directory")
	output := fs.String("out", "", "Output office file (.docx/.pptx/.xlsx)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("office pack requires -in and -out")
	}
	return skilltool.PackOfficeArchive(*input, *output)
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run ./cmd/skilltool office unpack -in file.docx -out work/docx")
	fmt.Println("  go run ./cmd/skilltool office pack -in work/docx -out output.docx")
}
