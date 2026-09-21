// Package shimexec is what every installed Shim script actually runs (via
// `cw shim-exec`): resolve the caller's real identity, classify the
// command, ask policy for a decision, fall through to the Approval Gate on
// a prompt, and exec the real binary if — and only if — allowed. It fails
// closed on every error path: anything that stops it from getting a clean
// answer denies rather than lets the command through.
package shimexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/audit"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghclassify"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/policy"
)

const (
	identityTimeout = 5 * time.Second
	gateTimeout     = 6 * time.Minute
	vaultTimeout    = 5 * time.Second
)

// Run is the shim's whole decision + dispatch flow. It returns the process
// exit code to use; on the auto-allow/allow-once/session-grant paths it
// never returns at all — a successful exec replaces the process image
// (syscall.Exec), so signals and the real binary's own exit code pass
// straight through rather than being reinterpreted by an intermediate Go
// process.
func Run(tool, realPath string, args []string, stderr io.Writer) int {
	ctx := context.Background()

	identityKey, err := agentclient.ResolveIdentity(ctx, identityTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "cw shim-exec: could not resolve identity, denying: %v\n", err)
		return 1
	}

	class := contracts.ClassUnknown
	if tool == "gh" {
		class = ghclassify.Classify(args)
	}

	entry, enrolled, err := policy.GetEntry(identityKey.String())
	if err != nil {
		fmt.Fprintf(stderr, "cw shim-exec: could not read policy, denying: %v\n", err)
		return 1
	}
	outcome := policy.Decide(entry.Level, class)
	commandLine := tool + " " + strings.Join(args, " ")

	decision, reasonCode := resolveDecision(ctx, outcome, identityKey.String(), tool, commandLine, class, entry, enrolled)

	if err := logDecision(decision, reasonCode, tool, class, entry, enrolled, identityKey.String()); err != nil {
		// Ticket #10's fail-closed contract: a decision that can't be
		// durably audited must not be allowed to proceed, even if the
		// decision itself was an allow.
		fmt.Fprintf(stderr, "cw shim-exec: audit write failed, failing closed: %v\n", err)
		return 1
	}

	switch decision {
	case contracts.DecisionAutoAllow, contracts.DecisionAllowOnce, contracts.DecisionSessionGrant, contracts.DecisionSessionAllow:
		execReal(ctx, realPath, tool, args, stderr)
		return 1 // only reached if exec itself failed
	default:
		fmt.Fprintf(stderr, "CmdWarden-Omarchy: %s denied (%s)\n", tool, reasonCode)
		return 1
	}
}

// resolveDecision turns a policy pre-check outcome into an actual gate
// decision, popping the real Approval Gate on OutcomePrompt.
func resolveDecision(ctx context.Context, outcome contracts.PolicyOutcome, identityKey, tool, commandLine string, class contracts.CommandClass, entry policy.Entry, enrolled bool) (contracts.Decision, string) {
	switch outcome {
	case contracts.OutcomeAutoAllow:
		return contracts.DecisionAutoAllow, "policy-auto-allow"
	case contracts.OutcomeDeny:
		if !enrolled {
			return contracts.DecisionDeny, "unenrolled-launcher"
		}
		return contracts.DecisionDeny, "policy-level-deny"
	case contracts.OutcomePrompt:
		offerSession := enrolled && class != contracts.ClassSecretReveal
		decision, err := agentclient.RequestGate(ctx, gateTimeout, identityKey, tool, commandLine, class, string(entry.Level), offerSession)
		if err != nil {
			return contracts.DecisionUnavailable, "gate-error"
		}
		switch decision {
		case contracts.DecisionAllowOnce:
			return decision, "user-approved-once"
		case contracts.DecisionSessionGrant:
			return decision, "user-allowed-session"
		case contracts.DecisionDeny:
			return decision, "user-denied"
		default:
			return contracts.DecisionUnavailable, "gate-unavailable"
		}
	default:
		return contracts.DecisionUnavailable, "unrecognized-policy-outcome"
	}
}

// execReal execs the real binary, replacing this process image entirely so
// signals and the exit code pass straight through. For gh specifically, per
// the ported Windows CmdWarden design ("on allow: child always gets
// GH_TOKEN from vault"), it releases the vaulted token and injects it —
// overriding any ambient GH_TOKEN, so the vault is authoritative once gh is
// harden'd. If release fails (e.g. gh was never `cw harden gh`-ed), that's
// not fatal to an already-decided allow: gh just falls back to its own
// normal token resolution, same as if it were never harden'd at all.
func execReal(ctx context.Context, realPath, tool string, args []string, stderr io.Writer) {
	env := os.Environ()
	if tool == "gh" {
		secretName := contracts.GHVaultSecretName(ghauth.DefaultHost)
		if token, err := agentclient.ReleaseSecret(ctx, vaultTimeout, secretName); err != nil {
			fmt.Fprintf(stderr, "cw shim-exec: could not release %s from the vault, falling back to gh's own token resolution: %v\n", secretName, err)
		} else {
			// glibc's getenv (and most libc implementations) returns the
			// *first* match in envp, so a naive append wouldn't actually
			// override an ambient GH_TOKEN — the existing one has to go.
			env = withoutEnv(env, "GH_TOKEN")
			env = append(env, "GH_TOKEN="+token)
		}
	}

	argv := append([]string{tool}, args...)
	if err := syscall.Exec(realPath, argv, env); err != nil {
		fmt.Fprintf(stderr, "cw shim-exec: executing %s: %v\n", realPath, err)
	}
}

// withoutEnv returns env with every "key=..." entry removed.
func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}

// logDecision writes the audit row for this gate decision.
func logDecision(decision contracts.Decision, reasonCode, tool string, class contracts.CommandClass, entry policy.Entry, enrolled bool, identityKey string) error {
	enrollmentKind := "unenrolled"
	if enrolled {
		enrollmentKind = string(entry.Kind)
	}
	return audit.Log(contracts.AuditRecord{
		Decision:       decision,
		ReasonCode:     reasonCode,
		Tool:           tool,
		CommandClass:   class,
		PolicyLevel:    string(entry.Level),
		IdentityKey:    identityKey,
		LauncherKind:   string(entry.Kind),
		EnrollmentKind: enrollmentKind,
	})
}
