// Package harden wires together the pieces #2-#7 already built into the
// end-to-end `cw harden <tool>` / `cw unharden <tool>` flow. gh is the only
// tool this spike vertical covers (see internal/ghclassify's package doc).
package harden

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shim"
)

// openImportKeyring opens the Secret Service session HardenGH imports into.
// It is the real opener in production. Tests replace it with a fake or
// absent keyring so a failure is the keyring or plaintext error rather than
// an unreachable agent.
var openImportKeyring = ghauth.OpenKeyring

const rpcTimeout = 15 * time.Second

// HardenGH resolves gh's Provenance Channel, imports its active token into
// the vault, installs the Shim (Occupied for mise and pacman), and records
// the pin.
//
// The import runs before the shim is installed, and it is the same
// ghauth.Import the agent's ImportGHToken method runs. A failed import —
// missing, locked, or wrong-wallet Secret Service, or a plaintext
// hosts.yml token — returns before Install, so no pin is saved. Re-running
// after gh has moved (e.g. a mise version bump orphaned the old pin — see
// internal/shim's Pin Drift) needs no manual unharden first: an existing
// pin is removed before re-installing, so harden is always safe to just
// run again.
//
// A Path pin wins LookPath, and Uninstall deletes that shim file. The
// binary to occupy is the pin's real path, resolved before that delete.
// Any other gh is resolved after Uninstall, so an Occupied shim is not
// what gets installed over.
func HardenGH(ctx context.Context, hostname string) (shim.Pin, error) {
	if err := ctx.Err(); err != nil {
		return shim.Pin{}, err
	}

	existing, pinned, err := shim.GetPin("gh")
	if err != nil {
		return shim.Pin{}, err
	}
	// Capture the real binary before Uninstall. For a Path pin, LookPath
	// returns the shim script, and Uninstall then deletes that file.
	ghPath := ""
	if pinned && existing.Mode == shim.ModePath {
		ghPath = existing.RealBinaryPath
	}
	if pinned {
		if err := shim.Uninstall("gh"); err != nil {
			return shim.Pin{}, fmt.Errorf("harden: removing gh's existing pin before re-harden: %w", err)
		}
	}
	if ghPath == "" {
		ghPath, err = exec.LookPath("gh")
		if err != nil {
			return shim.Pin{}, fmt.Errorf("harden: gh not found on PATH: %w", err)
		}
	}
	if _, err := os.Stat(ghPath); err != nil {
		return shim.Pin{}, fmt.Errorf("harden: gh binary %s: %w", ghPath, err)
	}

	kr, err := openImportKeyring()
	if err != nil {
		return shim.Pin{}, fmt.Errorf("harden: importing gh's active token: %w", err)
	}
	defer kr.Close()

	source, err := ghauth.Import(hostname, kr)
	if err != nil {
		return shim.Pin{}, fmt.Errorf("harden: importing gh's active token: %w", err)
	}

	pin, err := shim.Install("gh", ghPath)
	if err != nil {
		return shim.Pin{}, fmt.Errorf("harden: installing the Shim (the token was already imported from %s into the vault — run `cw unharden gh` to remove it): %w", source, err)
	}
	return pin, nil
}

// UnhardenGH reverses HardenGH: Shim removed, original binary restored,
// vault entry deleted. It's best-effort across both steps — a missing pin
// (already unharden'd) or a missing vault entry (already deleted, or the
// token was never imported) are not errors, but a real failure in either
// step is reported after attempting both, not on the first error.
func UnhardenGH(ctx context.Context, hostname string) error {
	var errs []error

	if _, ok, err := shim.GetPin("gh"); err != nil {
		errs = append(errs, err)
	} else if ok {
		if err := shim.Uninstall("gh"); err != nil {
			errs = append(errs, fmt.Errorf("removing shim: %w", err))
		}
	}

	if err := agentclient.DeleteSecret(ctx, rpcTimeout, contracts.GHVaultSecretName(hostname)); err != nil {
		errs = append(errs, fmt.Errorf("deleting vault entry: %w", err))
	}

	return errors.Join(errs...)
}
