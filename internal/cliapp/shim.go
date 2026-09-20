package cliapp

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shim"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shimexec"
)

func registerShimCommands(root *cobra.Command) {
	shimCmd := &cobra.Command{
		Use:   "shim",
		Short: "Install and manage Shims (Occupied or Path, by Provenance Channel)",
	}

	var (
		installTool string
		installPath string
	)
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install a Shim over --tool's currently resolved binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := installPath
			if targetPath == "" {
				resolved, err := exec.LookPath(installTool)
				if err != nil {
					return fmt.Errorf("cw shim install: resolving %q on PATH: %w (pass --path to shim an explicit location)", installTool, err)
				}
				targetPath = resolved
			}
			pin, err := shim.Install(installTool, targetPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "shim: installed %s Shim for %q (%s:%s) at %s\n", pin.Mode, installTool, pin.Channel, pin.ChannelTool, pin.ShimPath)
			return nil
		},
	}
	installCmd.Flags().StringVar(&installTool, "tool", "", "tool name to shim (required)")
	installCmd.Flags().StringVar(&installPath, "path", "", "explicit binary path to shim (defaults to the tool's current PATH resolution)")
	_ = installCmd.MarkFlagRequired("tool")
	shimCmd.AddCommand(installCmd)

	var uninstallTool string
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove a tool's Shim and restore normal resolution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := shim.Uninstall(uninstallTool); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "shim: removed %q's Shim\n", uninstallTool)
			return nil
		},
	}
	uninstallCmd.Flags().StringVar(&uninstallTool, "tool", "", "tool name to unshim (required)")
	_ = uninstallCmd.MarkFlagRequired("tool")
	shimCmd.AddCommand(uninstallCmd)

	shimCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List Shimmed tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			pins, err := shim.ListPins()
			if err != nil {
				return err
			}
			if len(pins) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "shim: no tools shimmed")
				return nil
			}
			for _, p := range pins {
				fmt.Fprintf(cmd.OutOrStdout(), "%-12s mode=%-9s channel=%s:%s  shim=%s\n", p.Tool, p.Mode, p.Channel, p.ChannelTool, p.ShimPath)
			}
			return nil
		},
	})

	root.AddCommand(shimCmd)

	// cw shim-exec is what every installed Shim script actually runs — a
	// top-level (not "shim exec") hidden command, with flag parsing turned
	// off past "--" so an arbitrary gated command's own flags are never
	// misread as cw's.
	var (
		execTool string
		execReal string
	)
	execCmd := &cobra.Command{
		Use:                "shim-exec",
		Hidden:             true,
		DisableFlagParsing: false,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if execTool == "" || execReal == "" {
				return fmt.Errorf("cw shim-exec: --tool and --real are required")
			}
			os.Exit(shimexec.Run(execTool, execReal, args, cmd.ErrOrStderr()))
			return nil
		},
	}
	execCmd.Flags().StringVar(&execTool, "tool", "", "tool name being shimmed")
	execCmd.Flags().StringVar(&execReal, "real", "", "real binary path to exec if allowed")
	execCmd.Flags().SetInterspersed(false)
	root.AddCommand(execCmd)
}
