package shim

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Drift describes a Shimmed tool whose currently-winning resolution no
// longer matches its harden-time Pin — e.g. a mise version bump created a
// fresh, unshimmed binary at a new path (Occupied), or some other install
// of the same tool name now resolves ahead of the Shim on PATH (Path).
// Either way, the tool is now silently un-gated instead of going through
// the Shim, which is exactly the failure mode Pin Drift detection exists to
// catch — see `cw doctor`.
type Drift struct {
	Tool    string
	Pin     Pin
	Current string // what actually resolves right now, for the diagnostic
	Reason  string
}

// DetectDrift re-resolves every Shimmed tool's current winning path and
// compares it against its Pin.
func DetectDrift() ([]Drift, error) {
	pins, err := ListPins()
	if err != nil {
		return nil, err
	}

	var drifts []Drift
	for _, pin := range pins {
		drift, err := checkOne(pin)
		if err != nil {
			return nil, err
		}
		if drift != nil {
			drifts = append(drifts, *drift)
		}
	}
	return drifts, nil
}

func checkOne(pin Pin) (*Drift, error) {
	resolved, err := exec.LookPath(pin.Tool)
	if err != nil {
		return &Drift{Tool: pin.Tool, Pin: pin, Current: "", Reason: fmt.Sprintf("%s no longer resolves on PATH at all", pin.Tool)}, nil
	}

	switch pin.Mode {
	case ModeOccupied:
		concrete, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			return &Drift{Tool: pin.Tool, Pin: pin, Current: resolved, Reason: fmt.Sprintf("resolving symlinks for %s: %v", resolved, err)}, nil
		}
		if concrete != pin.OriginalPath {
			return &Drift{
				Tool: pin.Tool, Pin: pin, Current: concrete,
				Reason: fmt.Sprintf("now resolves to %s, not the pinned %s (a new install — e.g. a mise version bump — appeared after harden time)", concrete, pin.OriginalPath),
			}, nil
		}
		// A pacman upgrade (and any other in-place overwrite) keeps the
		// same path, so a path comparison alone still looks pinned. The
		// file at that path has to still be the shim.
		if replaced, reason := occupiedFileReplaced(pin); replaced {
			return &Drift{Tool: pin.Tool, Pin: pin, Current: concrete, Reason: reason}, nil
		}
	case ModePath:
		if resolved != pin.ShimPath {
			return &Drift{
				Tool: pin.Tool, Pin: pin, Current: resolved,
				Reason: fmt.Sprintf("now resolves to %s, not the Shim at %s (something else now wins earlier on PATH)", resolved, pin.ShimPath),
			}, nil
		}
	default:
		return &Drift{Tool: pin.Tool, Pin: pin, Current: resolved, Reason: fmt.Sprintf("unrecognized shim mode %q", pin.Mode)}, nil
	}
	return nil, nil
}

// shimMarker is the distinctive token renderScript bakes into every shim.
// A package upgrade that overwrites the occupied path with the real binary
// will not contain it.
const shimMarker = "shim-exec"

// occupiedFileReplaced reports whether the file at the occupied path is no
// longer the shim script Install wrote there.
func occupiedFileReplaced(pin Pin) (bool, string) {
	data, err := os.ReadFile(pin.OriginalPath)
	if err != nil {
		return true, fmt.Sprintf("occupied file at %s was replaced in place (pin drift): %v", pin.OriginalPath, err)
	}
	if !bytes.Contains(data, []byte(shimMarker)) {
		return true, fmt.Sprintf("occupied file at %s was replaced in place (pin drift — the shim no longer occupies that path)", pin.OriginalPath)
	}
	return false, ""
}
