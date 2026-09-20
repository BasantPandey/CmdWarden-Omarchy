// Package shim installs and removes the two Shim shapes CmdWarden-Omarchy
// uses to intercept a gated tool's invocation, branching by the target
// binary's Provenance Channel (see internal/identity):
//
//   - Occupied Shim (mise-provenance): the real binary is renamed aside to
//     "<path>.cmdwarden-real" and a tiny shell script takes its exact file
//     path — every resolution path that used to reach the real binary now
//     reaches the Shim instead, with no PATH tricks needed.
//   - Path Shim (pacman-provenance): the real binary is left completely
//     untouched. A same-named script is installed in a CmdWarden-owned
//     directory that's prepended on PATH (see pathshim.go), so ordinary
//     PATH lookup finds the Shim first.
//
// Both shapes exec `cw shim-exec` (see cmd's shim-exec handler), which does
// the actual identity/policy/gate dispatch and finally execs the real
// binary if allowed.
package shim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

const pinFileName = "shims.json"

// Mode is which Shim shape is installed for a tool.
type Mode string

const (
	ModeOccupied Mode = "occupied"
	ModePath     Mode = "path"
)

// Pin records one tool's Shim install, so `cw doctor` can later re-resolve
// the tool's current winning Provenance Channel and flag Pin Drift if a
// second install method has since appeared.
type Pin struct {
	Tool string `json:"-"`

	Mode Mode `json:"mode"`
	// Channel/ChannelTool are the Provenance Channel and its stable name
	// (mise plugin or pacman package) *at harden time* — what Resolve
	// found when this Pin was installed.
	Channel     contracts.Channel `json:"channel"`
	ChannelTool string            `json:"channel_tool"`
	// OriginalPath is the resolved binary path the Shim was installed
	// over (Occupied: the exact path the Shim now occupies; Path: the
	// path PATH resolution found at harden time, kept for Pin Drift
	// comparison and doctor diagnostics).
	OriginalPath string `json:"original_path"`
	// RealBinaryPath is where the real binary actually lives now
	// (Occupied: "<OriginalPath>.cmdwarden-real"; Path: unchanged,
	// same as OriginalPath, since Path Shims never move anything).
	RealBinaryPath string    `json:"real_binary_path"`
	ShimPath       string    `json:"shim_path"`
	InstalledAt    time.Time `json:"installed_at"`
}

var storeMu sync.Mutex

func Path() (string, error) {
	dir, err := xdgpaths.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("shim: %w", err)
	}
	return filepath.Join(dir, pinFileName), nil
}

func load() (map[string]Pin, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Pin{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("shim: reading %s: %w", path, err)
	}
	var pins map[string]Pin
	if err := json.Unmarshal(data, &pins); err != nil {
		return nil, fmt.Errorf("shim: parsing %s: %w", path, err)
	}
	if pins == nil {
		pins = map[string]Pin{}
	}
	return pins, nil
}

func save(pins map[string]Pin) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return fmt.Errorf("shim: marshaling store: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "shims-*.json.tmp")
	if err != nil {
		return fmt.Errorf("shim: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("shim: writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("shim: syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("shim: closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("shim: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("shim: replacing %s: %w", path, err)
	}
	return nil
}

func savePin(tool string, pin Pin) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	pins, err := load()
	if err != nil {
		return err
	}
	pins[tool] = pin
	return save(pins)
}

func deletePin(tool string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	pins, err := load()
	if err != nil {
		return err
	}
	delete(pins, tool)
	return save(pins)
}

// GetPin returns tool's Pin, or (Pin{}, false) if it isn't Shimmed.
func GetPin(tool string) (Pin, bool, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	pins, err := load()
	if err != nil {
		return Pin{}, false, err
	}
	pin, ok := pins[tool]
	if ok {
		pin.Tool = tool
	}
	return pin, ok, nil
}

// ListPins returns every Shimmed tool's Pin, sorted by tool name.
func ListPins() ([]Pin, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	pins, err := load()
	if err != nil {
		return nil, err
	}
	list := make([]Pin, 0, len(pins))
	for tool, p := range pins {
		p.Tool = tool
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Tool < list[j].Tool })
	return list, nil
}
