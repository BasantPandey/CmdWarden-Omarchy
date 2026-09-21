package cliapp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/harden"
)

func registerHardenCommands(root *cobra.Command) {
	var hardenHostname string
	hardenCmd := &cobra.Command{
		Use:   "harden <tool>",
		Short: "Harden a tool end-to-end: import its token, install its Shim, record the pin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "gh" {
				return fmt.Errorf("cw harden: only \"gh\" is supported by this spike vertical, got %q", args[0])
			}
			pin, err := harden.HardenGH(cmd.Context(), hardenHostname)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "harden: gh is now hardened — %s Shim at %s (%s:%s)\n", pin.Mode, pin.ShimPath, pin.Channel, pin.ChannelTool)
			return nil
		},
	}
	hardenCmd.Flags().StringVar(&hardenHostname, "hostname", ghauth.DefaultHost, "gh host whose active token to import")
	root.AddCommand(hardenCmd)

	var unhardenHostname string
	unhardenCmd := &cobra.Command{
		Use:   "unharden <tool>",
		Short: "Reverse cw harden: remove the Shim, restore the original binary, delete the vault entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "gh" {
				return fmt.Errorf("cw unharden: only \"gh\" is supported by this spike vertical, got %q", args[0])
			}
			if err := harden.UnhardenGH(cmd.Context(), unhardenHostname); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "unharden: gh is back to normal — Shim removed, vault entry deleted")
			return nil
		},
	}
	unhardenCmd.Flags().StringVar(&unhardenHostname, "hostname", ghauth.DefaultHost, "gh host whose vault entry to delete")
	root.AddCommand(unhardenCmd)
}
