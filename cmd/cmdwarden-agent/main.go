// Command cmdwarden-agent is the CmdWarden-Omarchy Session Agent daemon.
// Transport, lifecycle, and D-Bus service registration are built out in
// ticket #2 (Session Agent bootstrap & health); this is a placeholder entry
// point so the module layout and CI are in place from ticket #1 onward.
package main

import (
	"fmt"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/version"
)

func main() {
	fmt.Printf("%s %s (commit %s)\n", contracts.AgentName, version.Version, version.Commit)
}
