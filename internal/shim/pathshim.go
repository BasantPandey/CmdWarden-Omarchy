package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

const bootstrapFileName = "shim-env-bootstrap.sh"

// ShimDir is the CmdWarden-owned directory Path Shims live in, prepended on
// PATH by the bootstrap this file manages.
func ShimDir() (string, error) {
	dataDir, err := xdgpaths.DataDir()
	if err != nil {
		return "", fmt.Errorf("shim: %w", err)
	}
	dir := filepath.Join(dataDir, "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("shim: creating %s: %w", dir, err)
	}
	return dir, nil
}

func bootstrapScriptPath() (string, error) {
	dataDir, err := xdgpaths.DataDir()
	if err != nil {
		return "", fmt.Errorf("shim: %w", err)
	}
	return filepath.Join(dataDir, bootstrapFileName), nil
}

const bootstrapScriptTemplate = `# CmdWarden-Omarchy Path Shim directory. Prepended so a pacman-provenance
# gated tool's Shim resolves ahead of the real binary. Safe to source
# multiple times; safe to delete (cw shim uninstall removes it once no Path
# Shims remain).
case ":$PATH:" in
  *":%s:"*) ;;
  *) PATH="%s:$PATH" ;;
esac
export PATH
`

const beginMarker = "# >>> cmdwarden shim path >>>"
const endMarker = "# <<< cmdwarden shim path <<<"

// environmentDPath is where systemd --user reads simple NAME=value
// environment assignments for the whole graphical session — everything it
// starts (terminals, and everything terminals spawn) inherits them. This is
// the one mechanism here that reaches a bash -c '...' invocation with no rc
// file at all: ~/.bashrc is *not* read for non-interactive non-login
// shells, but bash *does* read $BASH_ENV unconditionally if set. Omarchy's
// own equivalent for this exact gap is a PAM-level PATH line (see
// install/config/ssh-command-path.sh) — out of reach for a per-user tool
// with no root, so BASH_ENV is the closest per-user equivalent.
//
// Like the rest of environment.d, this only takes effect for a *new*
// systemd --user session (login/relogin) — not instantaneously for shells
// already running when a Shim is installed, same as Omarchy's own
// OMARCHY_PATH via this mechanism.
func environmentDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("shim: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "environment.d", "50-cmdwarden-shim.conf"), nil
}

// ensurePathBootstrap writes the bootstrap script (idempotent — static,
// deterministic content) and makes sure it's sourced from every shell
// startup point Omarchy's own env-bootstrap uses, per ticket #7: a login
// profile, the interactive bashrc chain, and the uwsm session's env.d
// (present even in a non-interactive/non-login shell — the whole point).
// Present, no-op if already installed.
func ensurePathBootstrap() error {
	shimDir, err := ShimDir()
	if err != nil {
		return err
	}
	scriptPath, err := bootstrapScriptPath()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(bootstrapScriptTemplate, shimDir, shimDir)
	if err := os.WriteFile(scriptPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("shim: writing %s: %w", scriptPath, err)
	}

	sourceLine := fmt.Sprintf(`[ -r %q ] && . %q`, scriptPath, scriptPath)
	for _, target := range bootstrapTargets() {
		if err := appendBlockIfMissing(target, sourceLine); err != nil {
			return err
		}
	}

	envDPath, err := environmentDPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(envDPath), 0o755); err != nil {
		return fmt.Errorf("shim: creating %s: %w", filepath.Dir(envDPath), err)
	}
	if err := os.WriteFile(envDPath, []byte("BASH_ENV="+scriptPath+"\n"), 0o644); err != nil {
		return fmt.Errorf("shim: writing %s: %w", envDPath, err)
	}
	return nil
}

// removePathBootstrap deletes the bootstrap script and its sourcing block
// from every file ensurePathBootstrap may have touched. Called once no Path
// Shims remain, so uninstalling the last one leaves no trace.
func removePathBootstrap() error {
	scriptPath, err := bootstrapScriptPath()
	if err != nil {
		return err
	}
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("shim: removing %s: %w", scriptPath, err)
	}
	for _, target := range bootstrapTargets() {
		if err := removeBlock(target); err != nil {
			return err
		}
	}

	envDPath, err := environmentDPath()
	if err != nil {
		return err
	}
	if err := os.Remove(envDPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("shim: removing %s: %w", envDPath, err)
	}
	return nil
}

// bootstrapTargets mirrors Omarchy's own env-bootstrap sourcing chain
// (profile.d + bashrc chain + uwsm session env.d — see
// /usr/share/omarchy/default/bash/env-bootstrap's own header comment for
// the system-wide equivalent), adapted to user-owned files since cw never
// writes under /etc or /usr.
func bootstrapTargets() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".profile"),                                 // login shells
		filepath.Join(home, ".bashrc"),                                  // interactive bash
		filepath.Join(home, ".zshrc"),                                   // interactive zsh, if present
		filepath.Join(home, ".config", "uwsm", "env.d", "50-cmdwarden"), // Hyprland/uwsm session
	}
}

// appendBlockIfMissing appends a marker-delimited block sourcing the
// bootstrap script, unless target already has one (idempotent). It creates
// target's parent directory if needed (for ~/.config/uwsm/env.d, which may
// not exist yet) but never creates target itself unless a block is actually
// being added — an absent ~/.zshrc, for instance, is left absent.
func appendBlockIfMissing(target, sourceLine string) error {
	existing, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("shim: reading %s: %w", target, err)
	}
	if strings.Contains(string(existing), beginMarker) {
		return nil // already installed
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("shim: creating %s: %w", filepath.Dir(target), err)
	}

	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("shim: opening %s: %w", target, err)
	}
	defer f.Close()

	block := "\n" + beginMarker + "\n" + sourceLine + "\n" + endMarker + "\n"
	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("shim: writing to %s: %w", target, err)
	}
	return nil
}

// removeBlock deletes the marker-delimited block from target, if present.
// A missing target, or one with no block, is not an error.
func removeBlock(target string) error {
	data, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("shim: reading %s: %w", target, err)
	}

	start := strings.Index(string(data), beginMarker)
	if start == -1 {
		return nil
	}
	end := strings.Index(string(data), endMarker)
	if end == -1 || end < start {
		return fmt.Errorf("shim: %s has a begin marker but no matching end marker — refusing to touch it", target)
	}
	end += len(endMarker)

	// Also drop the leading newline appendBlockIfMissing added before the
	// begin marker, so repeated install/uninstall doesn't accumulate
	// blank lines.
	if start > 0 && data[start-1] == '\n' {
		start--
	}
	if end < len(data) && data[end] == '\n' {
		end++
	}

	updated := string(data[:start]) + string(data[end:])
	if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("shim: writing %s: %w", target, err)
	}
	return nil
}
