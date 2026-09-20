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
