package cliapp

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
)

const defaultRPCTimeout = 3 * time.Second

func registerAgentCommands(root *cobra.Command) {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the cmdwarden-agent Session Agent",
	}

	agentCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Report whether the Session Agent is running and healthy",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := agentclient.Ping(cmd.Context(), defaultRPCTimeout)
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: healthy")
				return nil
			}
			if err == agentclient.ErrNotRunning {
				fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: not running")
				return nil
			}
			return err
		},
	})

	agentCmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the running Session Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agentclient.Stop(cmd.Context(), defaultRPCTimeout); err != nil {
				if err == agentclient.ErrNotRunning {
					fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: not running")
					return nil
				}
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: stop requested")
			return nil
		},
	})

	agentCmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and enable the cmdwarden-agent systemd --user socket unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agentclient.InstallUnits(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: units installed, socket enabled (lazy-starts on first use)")
			return nil
		},
	})

	agentCmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the cmdwarden-agent systemd --user units",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agentclient.UninstallUnits(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cmdwarden-agent: units removed")
			return nil
		},
	})

	root.AddCommand(agentCmd)
}
