package cliapp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/policy"
)

func registerPolicyCommands(root *cobra.Command) {
	policyCmd := &cobra.Command{
		Use:   "policy",
		Short: "Enroll Launchers and manage their policy levels",
	}

	var (
		enrollKind string
		enrollKey  string
	)
	enrollCmd := &cobra.Command{
		Use:   "enroll",
		Short: "Explicitly enroll a Launcher (defaults to the calling Launcher)",
		RunE: func(cmd *cobra.Command, args []string) error {
			key := enrollKey
			if key == "" {
				resolved, err := resolveOwnIdentity(cmd)
				if err != nil {
					return err
				}
				key = resolved
			}
			entry, err := policy.Enroll(key, contracts.LauncherKind(enrollKind))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "policy: enrolled %q as %s (level %s)\n", key, entry.Kind, entry.Level)
			return nil
		},
	}
	enrollCmd.Flags().StringVar(&enrollKind, "kind", "", "terminal|ai-harness (required)")
	enrollCmd.Flags().StringVar(&enrollKey, "key", "", "identity key to enroll (defaults to the calling Launcher, resolved via the agent)")
	_ = enrollCmd.MarkFlagRequired("kind")
	policyCmd.AddCommand(enrollCmd)

	policyCmd.AddCommand(&cobra.Command{
		Use:   "unenroll <identity-key>",
		Short: "Remove a Launcher's enrollment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := policy.Unenroll(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "policy: unenrolled %q\n", args[0])
			return nil
		},
	})

	policyCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List enrolled Launchers and their policy levels",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := policy.List()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "policy: no Launchers enrolled")
				return nil
			}
			for _, e := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%-30s kind=%-11s level=%s\n", e.IdentityKey, e.Kind, e.Level)
			}
			return nil
		},
	})

	policyCmd.AddCommand(&cobra.Command{
		Use:   "set <identity-key> <Deny|Read|Trusted|Full>",
		Short: "Change an already-enrolled Launcher's policy level",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := policy.Set(args[0], contracts.PolicyLevel(args[1]))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "policy: %q is now level %s\n", entry.IdentityKey, entry.Level)
			return nil
		},
	})

	var (
		checkIdentity string
		checkClass    string
	)
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Dry-run the policy decision for an identity (default: the caller) and a command class",
		RunE: func(cmd *cobra.Command, args []string) error {
			identityKey := checkIdentity
			if identityKey == "" {
				resolved, err := resolveOwnIdentity(cmd)
				if err != nil {
					return err
				}
				identityKey = resolved
			}
			class := contracts.CommandClass(checkClass)
			switch class {
			case contracts.ClassRead, contracts.ClassWrite, contracts.ClassSecretReveal, contracts.ClassUnknown:
			default:
				return fmt.Errorf("cw policy check: --class must be one of read|write|secret-reveal|unknown, got %q", checkClass)
			}

			level, enrolled, err := policy.Resolve(identityKey)
			if err != nil {
				return err
			}
			outcome := policy.Decide(level, class)

			fmt.Fprintf(cmd.OutOrStdout(), "identity: %s\n", identityKey)
			fmt.Fprintf(cmd.OutOrStdout(), "enrolled: %v\n", enrolled)
			fmt.Fprintf(cmd.OutOrStdout(), "level:    %s\n", level)
			fmt.Fprintf(cmd.OutOrStdout(), "class:    %s\n", class)
			fmt.Fprintf(cmd.OutOrStdout(), "decision: %s\n", outcome)
			return nil
		},
	}
	checkCmd.Flags().StringVar(&checkIdentity, "identity", "", "identity key to check (defaults to the caller, resolved via the agent)")
	checkCmd.Flags().StringVar(&checkClass, "class", "", "command class to check: read|write|secret-reveal|unknown (required)")
	_ = checkCmd.MarkFlagRequired("class")
	policyCmd.AddCommand(checkCmd)

	root.AddCommand(policyCmd)
}

func resolveOwnIdentity(cmd *cobra.Command) (string, error) {
	key, err := agentclient.ResolveIdentity(cmd.Context(), vaultRPCTimeout)
	if err != nil {
		return "", fmt.Errorf("resolving the calling Launcher's identity: %w", err)
	}
	return key.String(), nil
}
