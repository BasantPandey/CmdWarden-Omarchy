package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/identity"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/selfpath"
)

// Install shims tool, whose real binary currently resolves to targetPath,
// branching by targetPath's Provenance Channel (see internal/identity):
// Occupied Shim for mise, Path Shim for pacman. A targetPath that resolves
// as Unmanaged can't be meaningfully pinned this way and is rejected.
func Install(tool, targetPath string) (Pin, error) {
	if _, alreadyPinned, err := GetPin(tool); err != nil {
		return Pin{}, err
	} else if alreadyPinned {
		return Pin{}, fmt.Errorf("shim: %q is already shimmed (run `cw shim uninstall --tool %s` first to replace it)", tool, tool)
	}

	key, err := identity.ClassifyBinary(targetPath)
	if err != nil {
		return Pin{}, fmt.Errorf("shim: resolving %s's Provenance Channel: %w", targetPath, err)
	}

	cwPath, err := selfpath.Sibling("cw")
	if err != nil {
		return Pin{}, fmt.Errorf("shim: %w", err)
	}

	switch key.Channel {
	case contracts.ChannelMise:
		return installOccupied(tool, targetPath, key, cwPath)
	case contracts.ChannelPacman:
		return installPath(tool, targetPath, key, cwPath)
	default:
		return Pin{}, fmt.Errorf("shim: %s resolves as an Unmanaged Launcher (%s) — cw only shims pacman- or mise-provenance binaries", targetPath, key)
	}
}

// installOccupied renames the real binary aside to "<path>.cmdwarden-real"
// and writes the Shim script at its exact (symlink-resolved) file path —
// every resolution path that used to reach the real binary now reaches the
// Shim, with no PATH involved at all.
func installOccupied(tool, targetPath string, key contracts.IdentityKey, cwPath string) (Pin, error) {
	resolved, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return Pin{}, fmt.Errorf("shim: resolving symlinks for %s: %w", targetPath, err)
	}
	realPath := resolved + ".cmdwarden-real"

	if _, err := os.Stat(realPath); err == nil {
		return Pin{}, fmt.Errorf("shim: %s already exists — %s looks like it's already Shimmed outside cw's own records", realPath, resolved)
	}

	if err := os.Rename(resolved, realPath); err != nil {
		return Pin{}, fmt.Errorf("shim: moving the real binary aside: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(renderScript(cwPath, tool, realPath)), 0o755); err != nil {
		// Best-effort revert so a failed install doesn't leave the tool
		// gone entirely.
		_ = os.Rename(realPath, resolved)
		return Pin{}, fmt.Errorf("shim: writing shim script: %w", err)
	}

	pin := Pin{
		Mode:           ModeOccupied,
		Channel:        key.Channel,
		ChannelTool:    key.Tool,
		OriginalPath:   resolved,
		RealBinaryPath: realPath,
		ShimPath:       resolved,
		InstalledAt:    time.Now().UTC(),
	}
	if err := savePin(tool, pin); err != nil {
		return Pin{}, err
	}
	pin.Tool = tool
	return pin, nil
}

// installPath leaves the real (pacman-owned) binary completely untouched
// and installs a same-named Shim script in CmdWarden's own directory,
// prepended on PATH (see pathshim.go) so ordinary PATH resolution finds it
// first.
func installPath(tool, targetPath string, key contracts.IdentityKey, cwPath string) (Pin, error) {
	if err := ensurePathBootstrap(); err != nil {
		return Pin{}, err
	}
	shimDir, err := ShimDir()
	if err != nil {
		return Pin{}, err
	}
	shimScriptPath := filepath.Join(shimDir, tool)

	if err := os.WriteFile(shimScriptPath, []byte(renderScript(cwPath, tool, targetPath)), 0o755); err != nil {
		return Pin{}, fmt.Errorf("shim: writing shim script: %w", err)
	}

	pin := Pin{
		Mode:           ModePath,
		Channel:        key.Channel,
		ChannelTool:    key.Tool,
		OriginalPath:   targetPath,
		RealBinaryPath: targetPath,
		ShimPath:       shimScriptPath,
		InstalledAt:    time.Now().UTC(),
	}
	if err := savePin(tool, pin); err != nil {
		return Pin{}, err
	}
	pin.Tool = tool
	return pin, nil
}

// Uninstall reverses whichever Shim shape is installed for tool: restores
// the real binary to its original name (Occupied) or just removes the Shim
// script (Path — the real binary was never touched). It also tears down the
// PATH bootstrap files once no Path Shims remain, so uninstalling the last
// one leaves no trace in the user's shell startup files.
func Uninstall(tool string) error {
	pin, ok, err := GetPin(tool)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("shim: %q is not shimmed", tool)
	}

	switch pin.Mode {
	case ModeOccupied:
		if err := os.Rename(pin.RealBinaryPath, pin.ShimPath); err != nil {
			return fmt.Errorf("shim: restoring the real binary: %w", err)
		}
	case ModePath:
		if err := os.Remove(pin.ShimPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("shim: removing shim script: %w", err)
		}
	default:
		return fmt.Errorf("shim: %q has an unrecognized shim mode %q", tool, pin.Mode)
	}

	if err := deletePin(tool); err != nil {
		return err
	}

	remaining, err := ListPins()
	if err != nil {
		return err
	}
	stillHasPathShims := false
	for _, p := range remaining {
		if p.Mode == ModePath {
			stillHasPathShims = true
			break
		}
	}
	if !stillHasPathShims {
		if err := removePathBootstrap(); err != nil {
			return err
		}
	}
	return nil
}
