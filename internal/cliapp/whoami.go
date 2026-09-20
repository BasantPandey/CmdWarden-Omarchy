package cliapp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
)

func registerWhoamiCommand(root *cobra.Command) {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Resolve and print this shell's Launcher Identity Key",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := agentclient.ResolveIdentity(cmd.Context(), vaultRPCTimeout)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), key.String())
			if verbose {
				fmt.Fprintf(cmd.OutOrStdout(), "  channel: %s\n", key.Channel)
				fmt.Fprintf(cmd.OutOrStdout(), "  tool:    %s\n", key.Tool)
				fmt.Fprintf(cmd.OutOrStdout(), "  path:    %s\n", key.Path)
				if key.Hash != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  hash:    %s\n", key.Hash)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "also print channel/tool/path/hash")

	root.AddCommand(cmd)
}
