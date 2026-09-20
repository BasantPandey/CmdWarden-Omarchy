package cliapp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
)

const vaultRPCTimeout = 10 * defaultRPCTimeout

func registerVaultCommands(root *cobra.Command) {
	vaultCmd := &cobra.Command{
		Use:   "vault",
		Short: "Manage secrets in CmdWarden-Omarchy's own Secret Service vault",
	}

	vaultCmd.AddCommand(&cobra.Command{
		Use:   "save <name>",
		Short: "Save a secret value under <name> (prompts, or reads stdin if piped)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readSecretValue(cmd)
			if err != nil {
				return err
			}
			if err := agentclient.SaveSecret(cmd.Context(), vaultRPCTimeout, args[0], value); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vault: saved %q\n", args[0])
			return nil
		},
	})

	vaultCmd.AddCommand(&cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agentclient.DeleteSecret(cmd.Context(), vaultRPCTimeout, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vault: deleted %q\n", args[0])
			return nil
		},
	})

	var importHostname string
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import a tool's currently active credential into the vault",
	}
	importCmd.AddCommand(&cobra.Command{
		Use:   "gh",
		Short: "Import gh's currently active OAuth token",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := agentclient.ImportGHToken(cmd.Context(), vaultRPCTimeout, importHostname)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vault: imported gh token for %s from %s\n", importHostname, source)
			return nil
		},
	})
	importCmd.PersistentFlags().StringVar(&importHostname, "hostname", ghauth.DefaultHost, "gh host to import the token for")
	vaultCmd.AddCommand(importCmd)

	var (
		execSecret string
		execEnvVar string
	)
	execCmd := &cobra.Command{
		Use:   "exec -- <command> [args...]",
		Short: "Release a secret into a single child process's environment and run it",
		Long: "Releases the named vault secret and runs the given command with it set as\n" +
			"the given environment variable — only in that one child process. The value\n" +
			"is never printed, logged, or left in cw's own shell environment.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if execSecret == "" || execEnvVar == "" {
				return fmt.Errorf("cw vault exec: --secret and --env are required")
			}
			value, err := agentclient.ReleaseSecret(cmd.Context(), vaultRPCTimeout, execSecret)
			if err != nil {
				return err
			}

			child := exec.Command(args[0], args[1:]...)
			child.Env = append(os.Environ(), execEnvVar+"="+value)
			child.Stdin = cmd.InOrStdin()
			child.Stdout = cmd.OutOrStdout()
			child.Stderr = cmd.ErrOrStderr()
			return child.Run()
		},
	}
	execCmd.Flags().StringVar(&execSecret, "secret", "", "vault secret name to release")
	execCmd.Flags().StringVar(&execEnvVar, "env", "", "environment variable name to set in the child process")
	vaultCmd.AddCommand(execCmd)

	root.AddCommand(vaultCmd)
}

// readSecretValue reads the secret to save from stdin when it isn't a
// terminal (scripting/piping), or otherwise prompts interactively with the
// input hidden — never echoing the value either way.
func readSecretValue(cmd *cobra.Command) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), "Enter secret value: ")
		bytes, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("cw vault save: reading value: %w", err)
		}
		return string(bytes), nil
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("cw vault save: reading value from stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
