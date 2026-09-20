// Package contracts holds the domain types shared between the cw CLI, the
// cmdwarden-agent daemon, and the Approval Gate UI. It has no dependency on
// any of them (no D-Bus, no CLI framework, no QML) — it mirrors the role of
// CmdWarden's own Contracts layer, adapted to Go as a plain shared package
// instead of a separate module, since this repo is small enough that a
// module split would be pure ceremony.
package contracts

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
