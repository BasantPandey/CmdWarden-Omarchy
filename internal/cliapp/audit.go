package cliapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/audit"
)

func registerAuditCommands(root *cobra.Command) {
	var follow bool

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show the gate-decision audit log",
		RunE: func(cmd *cobra.Command, args []string) error {
			records, err := audit.ReadAll()
			if err != nil {
				return err
			}
			for _, rec := range records {
				printRecord(cmd, rec)
			}

			if !follow {
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return followAudit(ctx, cmd)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep tailing the log for new gate decisions")

	cmd.AddCommand(&cobra.Command{
		Use:   "prune",
		Short: fmt.Sprintf("Drop audit records older than the retention window (%s)", audit.RetentionWindow),
		RunE: func(cmd *cobra.Command, args []string) error {
			kept, dropped, err := audit.Prune(audit.RetentionWindow)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "audit: kept %d, dropped %d\n", kept, dropped)
			return nil
		},
	})

	root.AddCommand(cmd)
}

func printRecord(cmd *cobra.Command, rec any) {
	line, err := json.Marshal(rec)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "audit: failed to format record: %v\n", err)
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(line))
}

func followAudit(ctx context.Context, cmd *cobra.Command) error {
	stop := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stop)
	}()
	return audit.Follow(cmd.OutOrStdout(), stop)
}
