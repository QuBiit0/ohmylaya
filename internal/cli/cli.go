// Package cli routes command-line arguments to subcommands.
package cli

import (
	"fmt"
	"io"

	"github.com/QuBiit0/ohmylaya/internal/buildinfo"
)

const usage = `Usage: ohmylaya <command> [flags]

Commands:
  version     Print the version and exit
  help        Show this help

Run 'ohmylaya help <command>' for details on a command.
`

// Exit codes follow the convention used by most Go CLIs.
const (
	exitOK    = 0
	exitUsage = 2
)

// Run executes the command line and returns the process exit code.
// It never calls os.Exit so it stays testable.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return exitOK
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "ohmylaya %s\n", buildinfo.Version)
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "ohmylaya: unknown command %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
}
