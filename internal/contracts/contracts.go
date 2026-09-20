// Package contracts holds the domain types shared between the cw CLI, the
// cmdwarden-agent daemon, and the Approval Gate UI. It has no dependency on
// any of them (no D-Bus, no CLI framework, no QML) — it mirrors the role of
// CmdWarden's own Contracts layer, adapted to Go as a plain shared package
// instead of a separate module, since this repo is small enough that a
// module split would be pure ceremony.
package contracts

import "time"

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

// GHVaultSecretName is the vault entry name cw's gh integration (import,
// harden, gate) stores and releases gh's token under.
func GHVaultSecretName(hostname string) string {
	return "gh:" + hostname
}
