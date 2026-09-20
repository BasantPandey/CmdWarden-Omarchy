// Package contracts holds the domain types shared between the cw CLI, the
// cmdwarden-agent daemon, and the Approval Gate UI. It has no dependency on
// any of them (no D-Bus, no CLI framework, no QML) — it mirrors the role of
// CmdWarden's own Contracts layer, adapted to Go as a plain shared package
// instead of a separate module, since this repo is small enough that a
// module split would be pure ceremony.
package contracts

import (
	"fmt"
	"time"
)

// AppName / AgentName are the process/service names used for D-Bus well-known
// names, systemd unit names, and state directory naming across the project.
const (
	AppName   = "cmdwarden"
	AgentName = "cmdwarden-agent"
)

// CommandClass is the risk classification of a gated tool invocation, per
// the policy matrix (Deny/Read/Trusted/Full policy levels × these classes).
type CommandClass string

const (
	// ClassRead covers read-mostly commands: no side effects, no secrets.
	ClassRead CommandClass = "read"
	// ClassWrite covers side-effecting or auth-state-changing commands.
	ClassWrite CommandClass = "write"
	// ClassSecretReveal covers commands that can print/export a live credential.
	ClassSecretReveal CommandClass = "secret-reveal"
	// ClassUnknown covers anything the classifier doesn't recognize. It only
	// auto-allows under the Full policy level — an unmatched command must
	// never silently behave as ClassRead.
	ClassUnknown CommandClass = "unknown"
)

// Session Agent transport constants, shared by the agent (server side) and
// the cw CLI (client side). The agent has two transports:
//
//   - A systemd --user socket-activated Unix socket (see SocketFileName),
//     whose only job is to (a) give systemd something to lazily activate the
//     agent on, and (b) answer a trivial line-based PING even before the
//     agent's D-Bus name registration completes.
//   - The real D-Bus session bus, where the agent requests DBusServiceName
//     and exports DBusObjectPath/DBusInterface — this is the actual RPC
//     surface every later ticket (identity, policy, vault, gate, audit)
//     builds on.
const (
	DBusServiceName = "org.cmdwarden.Agent1"
	DBusObjectPath  = "/org/cmdwarden/Agent1"
	DBusInterface   = "org.cmdwarden.Agent1"

	SocketFileName  = "cmdwarden-agent.sock"
	SocketUnitName  = "cmdwarden-agent.socket"
	ServiceUnitName = "cmdwarden-agent.service"
)

// Decision is the outcome of a gate check, per the Windows CmdWarden audit
// spec's event list — auditable events are gate decisions only.
type Decision string

const (
	DecisionAutoAllow    Decision = "auto-allow"
	DecisionAllowOnce    Decision = "allow-once"
	DecisionDeny         Decision = "deny"
	DecisionUnavailable  Decision = "unavailable"
	DecisionSessionGrant Decision = "session-grant"
	DecisionSessionAllow Decision = "session-allow"
)

// AuditRecord is one NDJSON row of the gate-decision audit log. Field names
// and json tags mirror the Windows CmdWarden audit spec
// (https://github.com/BasantPandey/CmdWarden/blob/main/docs/spec/cmdwarden.md#9-audit)
// verbatim, so anyone who already knows that spec can read this log without
// relearning field names. It deliberately has no field capable of holding a
// secret value or a full command line — that's enforced by this struct's
// shape, not by caller discipline.
type AuditRecord struct {
	Timestamp      time.Time    `json:"ts"`
	Decision       Decision     `json:"decision"`
	ReasonCode     string       `json:"reason_code"`
	Tool           string       `json:"tool"`
	CommandClass   CommandClass `json:"command_class"`
	PolicyLevel    string       `json:"policy_level"`
	IdentityKey    string       `json:"launcher_policy_key"`
	LauncherKind   string       `json:"launcher_kind"`
	EnrollmentKind string       `json:"enrollment_kind"`
	SecretName     string       `json:"secret_name,omitempty"`

	// Optional, per the spec's "optional purpose/path/pid".
	Purpose string `json:"purpose,omitempty"`
	Path    string `json:"path,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// PolicyLevel is how much an enrolled Launcher is trusted, per the
// Windows CmdWarden policy matrix (auto-allow by level x class).
type PolicyLevel string

const (
	LevelDeny    PolicyLevel = "Deny"
	LevelRead    PolicyLevel = "Read"
	LevelTrusted PolicyLevel = "Trusted"
	LevelFull    PolicyLevel = "Full"
)

// LauncherKind is how a Launcher was enrolled — enrollment is explicit
// only, and the kind picks the default PolicyLevel (see
// DefaultLevelForKind): ai-harness -> Read, terminal -> Trusted.
type LauncherKind string

const (
	KindAIHarness LauncherKind = "ai-harness"
	KindTerminal  LauncherKind = "terminal"
)

// DefaultLevelForKind returns the PolicyLevel a newly enrolled Launcher of
// this kind starts at.
func DefaultLevelForKind(kind LauncherKind) (PolicyLevel, error) {
	switch kind {
	case KindAIHarness:
		return LevelRead, nil
	case KindTerminal:
		return LevelTrusted, nil
	default:
		return "", fmt.Errorf("contracts: unrecognized launcher kind %q (want %q or %q)", kind, KindAIHarness, KindTerminal)
	}
}

// PolicyOutcome is a policy pre-check's answer for one (level, class) pair —
// distinct from Decision (audit.go), which records what a full gate flow
// (including a human's Approval Gate answer) actually did. A Deny outcome
// here means "never even ask" (the level itself is Deny, or the Launcher
// isn't enrolled at all); Prompt means "not auto-allowed, ask the Approval
// Gate."
type PolicyOutcome string

const (
	OutcomeAutoAllow PolicyOutcome = "auto-allow"
	OutcomePrompt    PolicyOutcome = "prompt"
	OutcomeDeny      PolicyOutcome = "deny"
)

// GHVaultSecretName is the vault entry name cw's gh integration (import,
// harden, gate) stores and releases gh's token under.
func GHVaultSecretName(hostname string) string {
	return "gh:" + hostname
}

// Channel is a Launcher's Provenance Channel: which package manager's
// install tree its resolved binary path lives under, if any.
type Channel string

const (
	ChannelPacman    Channel = "pacman"
	ChannelMise      Channel = "mise"
	ChannelUnmanaged Channel = "unmanaged"
)

// IdentityKey identifies a Launcher: <provenance channel>:<tool> for a
// pacman- or mise-provenance binary (e.g. "mise:claude", "pacman:foot"), or
// a path+hash identity for a binary with no recognized package-manager
// provenance (an Unmanaged Launcher). Same-channel version bumps (e.g. mise
// upgrading a tool in place) resolve to the same IdentityKey because Tool is
// the channel's own stable name for it (a mise plugin name, a pacman
// package name) rather than anything version- or path-specific.
type IdentityKey struct {
	Channel Channel
	Tool    string
	// Path is the resolved Launcher binary's absolute path, kept for
	// diagnostics — it is not part of String()'s identity for
	// pacman/mise channels (that's the point: it can change across a
	// version bump without changing the identity).
	Path string
	// Hash is a sha256 hex digest of the binary's contents, populated
	// only when Channel == ChannelUnmanaged, where there's no
	// channel-provided stable name to key off instead.
	Hash string
}

// String renders the Identity Key in its canonical <channel>:<tool> form
// (or unmanaged:<tool>@<hash12> for an Unmanaged Launcher).
func (k IdentityKey) String() string {
	if k.Channel == ChannelUnmanaged {
		h := k.Hash
		if len(h) > 12 {
			h = h[:12]
		}
		return fmt.Sprintf("unmanaged:%s@%s", k.Tool, h)
	}
	return fmt.Sprintf("%s:%s", k.Channel, k.Tool)
}
