package cliapp

import "github.com/spf13/cobra"

// The register* functions below are filled in by later tickets (agent
// bootstrap, identity, policy, vault, shim/harden, approval gate, audit,
// doctor). Kept as explicit no-ops here so the command tree in root.go has a
// single, stable call site to extend instead of main.go growing a new import
// per ticket.

func registerAgentCommands(root *cobra.Command)  {}
func registerWhoamiCommand(root *cobra.Command)  {}
func registerPolicyCommands(root *cobra.Command) {}
func registerVaultCommands(root *cobra.Command)  {}
func registerHardenCommands(root *cobra.Command) {}
func registerGateCommands(root *cobra.Command)   {}
func registerAuditCommands(root *cobra.Command)  {}
func registerDoctorCommand(root *cobra.Command)  {}
