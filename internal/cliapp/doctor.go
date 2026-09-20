package cliapp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shim"
)

// doctorCheck is one independent health check `cw doctor` runs. Later
// tickets (e.g. #7's Pin Drift detection) append to doctorChecks rather than
// growing this file's RunE.
type doctorCheck struct {
	name string
	run  func(cmd *cobra.Command) error
}

var doctorChecks = []doctorCheck{
	{
		name: "Session Agent",
		run: func(cmd *cobra.Command) error {
			return agentclient.WaitHealthy(cmd.Context(), agentWakeTimeout)
		},
	},
	{
		name: "Shim Pin Drift",
		run: func(cmd *cobra.Command) error {
			drifts, err := shim.DetectDrift()
			if err != nil {
				return err
			}
			if len(drifts) == 0 {
				return nil
			}
			var msg string
			for _, d := range drifts {
				msg += fmt.Sprintf("\n  - %s: %s", d.Tool, d.Reason)
			}
			return fmt.Errorf("Pin Drift detected:%s", msg)
		},
	},
}

const agentWakeTimeout = 5 * defaultRPCTimeout

func registerDoctorCommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check (and lazy-start) the Session Agent and other CmdWarden-Omarchy health signals",
		RunE: func(cmd *cobra.Command, args []string) error {
			var failed bool
			for _, check := range doctorChecks {
				if err := check.run(cmd); err != nil {
					failed = true
					fmt.Fprintf(cmd.OutOrStdout(), "[FAIL] %s: %v\n", check.name, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[ OK ] %s: healthy\n", check.name)
			}
			if failed {
				return fmt.Errorf("cw doctor: one or more checks failed")
			}
			return nil
		},
	})
}
