// Package identity resolves a process's Launcher Identity Key by walking
// /proc ancestry from a given starting PID up to the first ancestor that
// isn't a generic shell/wrapper process — that ancestor *is* the Launcher,
// whatever it turns out to be (a terminal emulator, an AI coding harness,
// or anything else) — and then classifies that Launcher's resolved binary
// path by Provenance Channel (pacman, mise, or Unmanaged).
//
// This is pure OS-inspection logic: no D-Bus, no agent state. The agent
// calls Resolve with the real D-Bus caller's PID (obtained via
// GetConnectionUnixProcessID, not anything the caller self-reports) so the
// result can't be spoofed by a lying client — see internal/agentd's
// ResolveIdentity.
package identity

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// maxWalkDepth bounds the /proc ancestry walk so a pathological process
// tree (or a /proc read loop) can't hang or spin forever.
const maxWalkDepth = 64

// notLaunchers are generic shell/wrapper processes we walk straight past —
// none of them is ever itself "the Launcher," the interesting thing is
// whatever invoked them.
var notLaunchers = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true,
	"fish": true, "tcsh": true, "csh": true, "tmux": true, "screen": true,
	"sudo": true, "doas": true, "su": true, "env": true, "nohup": true,
	"setsid": true, "xargs": true, "script": true, "login": true,
}

// Resolve walks the ancestry of callerPID (not including callerPID itself —
// it's cw's or the Shim's own process, never the Launcher) and returns the
// Identity Key of the first non-shell ancestor found.
func Resolve(callerPID int) (contracts.IdentityKey, error) {
	pid := callerPID
	for depth := 0; depth < maxWalkDepth; depth++ {
		ppid, err := parentOf(pid)
		if err != nil {
			return contracts.IdentityKey{}, fmt.Errorf("identity: reading parent of pid %d: %w", pid, err)
		}
		if ppid <= 1 {
			return contracts.IdentityKey{}, fmt.Errorf("identity: no recognized Launcher found walking ancestry from pid %d (reached the top of the process tree)", callerPID)
		}
		pid = ppid

		exePath, err := exeOf(pid)
		if err != nil {
			// Process exited mid-walk, or we can't read it (permission,
			// kernel thread with no exe, ...) — keep walking up rather
			// than failing the whole resolution.
			continue
		}
		name := filepath.Base(exePath)
		if notLaunchers[name] {
			continue
		}
		return classify(exePath, name)
	}
	return contracts.IdentityKey{}, fmt.Errorf("identity: ancestry walk from pid %d exceeded %d hops without finding a Launcher", callerPID, maxWalkDepth)
}

// ClassifyBinary resolves the Provenance Channel of a specific binary path
// directly, without any ancestry walk — used by the Shim installer (#7) to
// decide Occupied vs. Path Shim for a given tool's resolved path, the same
// classification Resolve uses for a Launcher found by ancestry.
func ClassifyBinary(path string) (contracts.IdentityKey, error) {
	return classify(path, filepath.Base(path))
}

func classify(exePath, name string) (contracts.IdentityKey, error) {
	if tool, ok := miseTool(exePath); ok {
		return contracts.IdentityKey{Channel: contracts.ChannelMise, Tool: tool, Path: exePath}, nil
	}
	if pkg, ok := pacmanPackage(exePath); ok {
		return contracts.IdentityKey{Channel: contracts.ChannelPacman, Tool: pkg, Path: exePath}, nil
	}
	hash, err := hashFile(exePath)
	if err != nil {
		return contracts.IdentityKey{}, fmt.Errorf("identity: hashing unmanaged Launcher %s: %w", exePath, err)
	}
	return contracts.IdentityKey{Channel: contracts.ChannelUnmanaged, Tool: name, Path: exePath, Hash: hash}, nil
}

// miseDataDir mirrors mise's own default: $MISE_DATA_DIR, else
// ~/.local/share/mise.
func miseDataDir() string {
	if dir := os.Getenv("MISE_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "mise")
}

// miseTool reports whether exePath lives under mise's install tree
// (<data dir>/installs/<tool>/<version>/...) and, if so, extracts <tool> —
// the plugin name, which stays the same across a version bump even though
// the path's version segment changes.
func miseTool(exePath string) (string, bool) {
	dataDir := miseDataDir()
	if dataDir == "" {
		return "", false
	}
	root := filepath.Join(dataDir, "installs") + string(filepath.Separator)
	if !strings.HasPrefix(exePath, root) {
		return "", false
	}
	rest := strings.TrimPrefix(exePath, root)
	tool, _, found := strings.Cut(rest, string(filepath.Separator))
	if !found || tool == "" {
		return "", false
	}
	return tool, true
}

var pacmanOwnerRE = regexp.MustCompile(`is owned by (\S+) `)

// pacmanPackage reports whether exePath is owned by an installed pacman
// package and, if so, that package's name. A missing pacman binary (e.g.
// running this on non-Arch CI) or an unowned path both just mean "no,"
// never an error — most Launchers on any given machine won't be pacman's.
func pacmanPackage(exePath string) (string, bool) {
	out, err := exec.Command("pacman", "-Qo", exePath).Output()
	if err != nil {
		return "", false
	}
	m := pacmanOwnerRE.FindSubmatch(out)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func exeOf(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
}

// parentOf reads PPid out of /proc/<pid>/status, which — unlike
// /proc/<pid>/stat's positional fields — can't be misparsed by a process
// name containing spaces or parentheses.
func parentOf(pid int) (int, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if rest, ok := strings.CutPrefix(line, "PPid:"); ok {
			ppid, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				return 0, fmt.Errorf("parsing PPid line %q: %w", line, err)
			}
			return ppid, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("no PPid line in /proc/%d/status", pid)
}
