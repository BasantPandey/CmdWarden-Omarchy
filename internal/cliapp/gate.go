package cliapp

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/policy"
)

// gateCallTimeout has to comfortably exceed the agent's own internal
// gateTimeout (5 minutes) — this is a human-in-the-loop D-Bus call.
const gateCallTimeout = 6 * time.Minute

func registerGateCommands(root *cobra.Command) {
	gateCmd := &cobra.Command{
		Use:   "gate",
		Short: "Trigger and respond to the Approval Gate",
	}

	var (
		testTool     string
		testCommand  string
		testClass    string
		testIdentity string
	)
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Trigger a real Approval Gate popup with the caller's real identity and policy level (fake command)",
		RunE: func(cmd *cobra.Command, args []string) error {
			identityKey := testIdentity
			if identityKey == "" {
				resolved, err := resolveOwnIdentity(cmd)
				if err != nil {
					return err
				}
				identityKey = resolved
			}
			class := contracts.CommandClass(testClass)
			switch class {
			case contracts.ClassRead, contracts.ClassWrite, contracts.ClassSecretReveal, contracts.ClassUnknown:
			default:
				return fmt.Errorf("cw gate test: --class must be one of read|write|secret-reveal|unknown, got %q", testClass)
			}

			level, enrolled, err := policy.Resolve(identityKey)
			if err != nil {
				return err
			}
			// Per the Windows CmdWarden Approval Gate spec: Allow for
			// Session is offered only for enrolled Launchers, and never
			// for secret-reveal.
			offerSession := enrolled && class != contracts.ClassSecretReveal

			fmt.Fprintf(cmd.OutOrStdout(), "gate: popping Approval Gate for %s running %s (class=%s, policy=%s)...\n", identityKey, testTool, class, level)
			decision, err := agentclient.RequestGate(cmd.Context(), gateCallTimeout, identityKey, testTool, testCommand, class, string(level), offerSession)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "gate: decision = %s\n", decision)
			return nil
		},
	}
	testCmd.Flags().StringVar(&testTool, "tool", "gh", "fake tool name to display")
	testCmd.Flags().StringVar(&testCommand, "command", "gh pr create --title demo --body demo", "fake command line to display")
	testCmd.Flags().StringVar(&testClass, "class", "write", "command class to display: read|write|secret-reveal|unknown")
	testCmd.Flags().StringVar(&testIdentity, "identity", "", "identity key to display (defaults to the caller, resolved via the agent)")
	gateCmd.AddCommand(testCmd)

	var (
		respondRequestID string
		respondDecision  string
	)
	respondCmd := &cobra.Command{
		Use:    "respond",
		Short:  "Submit a human's Approval Gate answer (invoked by the gate UI, not for direct use)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return agentclient.SubmitGateDecision(cmd.Context(), defaultRPCTimeout, respondRequestID, respondDecision)
		},
	}
	respondCmd.Flags().StringVar(&respondRequestID, "request", "", "pending gate request id")
	respondCmd.Flags().StringVar(&respondDecision, "decision", "", "deny|allow-once|allow-session")
	_ = respondCmd.MarkFlagRequired("request")
	_ = respondCmd.MarkFlagRequired("decision")
	gateCmd.AddCommand(respondCmd)

	root.AddCommand(gateCmd)
}
