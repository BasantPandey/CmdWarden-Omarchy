// Package xdgpaths resolves the XDG Base Directory paths CmdWarden-Omarchy
// stores its own state under, per the Linux state-path adaptation of
// Windows CmdWarden's spec. Every other package that needs a path on disk
// (agent runtime socket, policy enrollment store, audit log, shim pins)
// goes through here rather than re-deriving XDG env vars itself.
package xdgpaths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appDirName = "cmdwarden"

// RuntimeSocketPath returns $XDG_RUNTIME_DIR/<name> directly — deliberately
// not nested under a cmdwarden subdirectory, and deliberately not creating
// anything. It's used for the agent's systemd socket-activation unit, whose
// ListenStream=%t/<name> binds before cmdwarden-agent (the only thing that
// could otherwise create a subdirectory) has ever run, so the parent
// directory must already exist unconditionally — which $XDG_RUNTIME_DIR
// itself, unlike a subdirectory of it, always does.
func RuntimeSocketPath(name string) (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return "", fmt.Errorf("xdgpaths: XDG_RUNTIME_DIR is not set")
	}
	return filepath.Join(base, name), nil
}

// StateDir returns $XDG_STATE_HOME/cmdwarden (default ~/.local/state/cmdwarden),
// creating it (mode 0700) if needed. This is where persistent-but-not-config
// state lives: the audit log, policy enrollment store, shim pin records.
func StateDir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("xdgpaths: resolving home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return ensureDir(filepath.Join(base, appDirName))
}

// DataDir returns $XDG_DATA_HOME/cmdwarden (default ~/.local/share/cmdwarden),
// creating it (mode 0700) if needed. This is where installed artifacts that
// aren't config or state live — the Path Shim directory and its PATH
// bootstrap script.
func DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("xdgpaths: resolving home directory: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return ensureDir(filepath.Join(base, appDirName))
}

// ConfigDir returns $XDG_CONFIG_HOME/cmdwarden (default ~/.config/cmdwarden),
// creating it (mode 0700) if needed.
func ConfigDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("xdgpaths: resolving home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return ensureDir(filepath.Join(base, appDirName))
}

// UserSystemdUnitDir returns $XDG_CONFIG_HOME/systemd/user (default
// ~/.config/systemd/user), creating it if needed — where cw agent install
// writes cmdwarden-agent.socket/.service.
func UserSystemdUnitDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("xdgpaths: resolving home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return ensureDir(filepath.Join(base, "systemd", "user"))
}

func ensureDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("xdgpaths: creating %s: %w", dir, err)
	}
	return dir, nil
}
