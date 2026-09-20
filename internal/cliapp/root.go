// Package cliapp assembles the cw CLI's command tree. It is imported only by
// cmd/cw; the agent (cmd/cmdwarden-agent) never imports it, keeping the CLI
// and the agent's own dependency graphs separate per the Contracts/Agent/Cli
// split.
package cliapp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/version"
)

// NewRootCommand builds the cw command tree. Subcommands register themselves
// via the returned command's AddCommand in this function, so later tickets
// extend this one place rather than main.go.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "cw",
		Aliases:       []string{"cmdwarden"},
		Short:         "CmdWarden-Omarchy — gh-only Compat Mode spike vertical",
		Long:          "cw is the CmdWarden-Omarchy CLI: an Inspired Twin of Windows CmdWarden for Omarchy (Arch Linux + Hyprland).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCommand())
	registerAgentCommands(root)
	registerWhoamiCommand(root)
	registerPolicyCommands(root)
	registerVaultCommands(root)
	registerHardenCommands(root)
	registerGateCommands(root)
	registerAuditCommands(root)
	registerDoctorCommand(root)

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the cw version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s (commit %s)\n", contracts.AppName, version.Version, version.Commit)
			return nil
		},
	}
}
