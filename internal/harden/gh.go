// Package harden wires together the pieces #2-#7 already built into the
// end-to-end `cw harden <tool>` / `cw unharden <tool>` flow. gh is the only
// tool this spike vertical covers (see internal/ghclassify's package doc).
package harden

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentclient"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shim"
)

const rpcTimeout = 15 * time.Second

// HardenGH resolves gh's Provenance Channel, imports its active token into
// the vault, installs the Shim (Occupied or Path, per gh's channel), and
// records the pin.
//
// Re-running it after gh has moved (e.g. a mise version bump orphaned the
// old pin — see internal/shim's Pin Drift) needs no manual unharden first:
// an existing pin is removed before re-installing, so harden is always safe
// to just run again.
func HardenGH(ctx context.Context, hostname string) (shim.Pin, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return shim.Pin{}, fmt.Errorf("harden: gh not found on PATH: %w", err)
	}

	if _, ok, err := shim.GetPin("gh"); err != nil {
		return shim.Pin{}, err
	} else if ok {
		if err := shim.Uninstall("gh"); err != nil {
			return shim.Pin{}, fmt.Errorf("harden: removing gh's existing pin before re-harden: %w", err)
		}
	}

	source, err := agentclient.ImportGHToken(ctx, rpcTimeout, hostname)
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
