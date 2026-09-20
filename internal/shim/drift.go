package shim

import (
	"fmt"
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
