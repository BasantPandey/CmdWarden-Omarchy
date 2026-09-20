// Package version carries the build-time version metadata shared by every
// CmdWarden-Omarchy entrypoint (cw, cmdwarden-agent).
package version

// Version is overridden at build time via -ldflags "-X ...version.Version=...".
var Version = "0.0.0-dev"

// Commit is overridden at build time via -ldflags "-X ...version.Commit=...".
var Commit = "unknown"
