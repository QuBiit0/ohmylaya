// Command ohmylaya installs and supervises a local Laya decision engine and
// exposes it to coding agents.
package main

import (
	"os"

	"github.com/QuBiit0/ohmylaya/internal/cli"
	"github.com/QuBiit0/ohmylaya/internal/update"
)

func main() {
	if exe, err := os.Executable(); err == nil {
		update.SwapPending(exe)
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
