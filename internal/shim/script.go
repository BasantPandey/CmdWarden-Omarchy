package shim

import "fmt"

// scriptTemplate is what every Shim actually is, regardless of shape: a
// tiny, auditable POSIX shell script that hands off to `cw shim-exec`,
// which does the real identity/policy/gate dispatch and finally execs the
// real binary if allowed.
//
// The tool name and the real binary's path are baked in literally at
// install time rather than inferred from $0 at runtime: a shell sets $0 to
// whatever bare string the user typed (e.g. plain "gh"), not the resolved
// path the shim was found at, for anything found via a PATH search — which
// is exactly how both Shim shapes are normally invoked. Baking in the real
// path also means Path Shim mode needs no runtime PATH-searching logic at
// all: a pacman-provenance binary's path is stable across upgrades (pacman
// overwrites in place), so the path recorded at install time keeps working.
const scriptTemplate = `#!/bin/sh
# Installed by CmdWarden-Omarchy. Do not edit by hand — run
# ` + "`cw shim uninstall --tool %[2]s`" + ` to remove it cleanly.
exec %[1]q shim-exec --tool %[2]q --real %[3]q -- "$@"
`

func renderScript(cwPath, tool, realPath string) string {
	return fmt.Sprintf(scriptTemplate, cwPath, tool, realPath)
}
