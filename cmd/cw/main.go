// Command cw is the CmdWarden-Omarchy CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/cliapp"
)

func main() {
	if err := cliapp.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cw:", err)
		os.Exit(1)
	}
}
