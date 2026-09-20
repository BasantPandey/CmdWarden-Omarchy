package cliapp

import "github.com/spf13/cobra"

// The register* functions below are filled in by later tickets (identity,
// policy, vault, shim/harden, approval gate, audit). Kept as explicit
// no-ops here so the command tree in root.go has a single, stable call site
// to extend instead of main.go growing a new import per ticket.

func registerHardenCommands(root *cobra.Command) {}
func registerGateCommands(root *cobra.Command)   {}
