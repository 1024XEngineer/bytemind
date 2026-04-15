package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func RunInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	installDir := fs.String("to", "", "Install directory. Defaults to BYTEMIND_HOME/bin (or ~/.bytemind/bin).")
	binaryName := fs.String("name", "", "Binary name. Defaults to bytemind (bytemind.exe on Windows).")
	addToPath := fs.Bool("add-to-path", true, "Automatically add install directory to user PATH when possible.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("install does not accept positional args: %s", strings.Join(fs.Args(), " "))
	}

	sourcePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	targetPath, err := resolveInstallTarget(*installDir, *binaryName)
	if err != nil {
		return err
	}
	if err := installBinary(sourcePath, targetPath); err != nil {
		return err
	}

	targetDir := filepath.Dir(targetPath)
	fmt.Fprintf(stdout, "Installed Bytemind to %s\n", targetPath)
	if pathContainsDir(os.Getenv("PATH"), targetDir) {
		fmt.Fprintln(stdout, "PATH already includes this directory in this terminal. You can now run: bytemind")
		return nil
	}
	if *addToPath {
		changed, err := addInstallDirToUserPath(targetDir)
		if err == nil {
			if changed {
				fmt.Fprintln(stdout, "Added install directory to user PATH.")
				fmt.Fprintln(stdout, "Open a new terminal, then run: bytemind")
			} else {
				fmt.Fprintln(stdout, "Install directory already exists in user PATH.")
			}
			return nil
		}
		fmt.Fprintf(stdout, "Automatic PATH update failed: %v\n", err)
	}

	fmt.Fprintf(stdout, "Add this directory to PATH to run Bytemind from anywhere:\n%s\n", targetDir)
	printPathHint(stdout, targetDir)
	return nil
}
