// Package selfpath finds a sibling CmdWarden-Omarchy binary relative to
// whichever one is currently running — the pattern every component that
// needs to hand another one's absolute path to a spawned process (systemd
// unit ExecStart=, a Shim script's exec target, the Approval Gate's `cw
// gate respond` callback) uses instead of trusting PATH resolution inside
// an environment it doesn't control.
package selfpath

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Sibling finds a binary named name next to the currently running
// executable first (the normal case when every CmdWarden-Omarchy binary is
// built into the same bin/ directory), then falls back to PATH.
func Sibling(name string) (string, error) {
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("selfpath: could not find %q next to the running executable or on PATH", name)
}
