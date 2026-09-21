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

// Launcher is a resolved Identity Key together with the OS process it was
// found at — the PID/StartTime pair a session grant (see internal/agentd's
// session tracking) keys off of to know when "until it exits" has happened,
// and to survive PID reuse (a start time recorded at grant time that no
// longer matches the same PID means it's a different process now).
type Launcher struct {
	Key       contracts.IdentityKey
	PID       int
	StartTime uint64
}

// Resolve walks the ancestry of callerPID (not including callerPID itself —
// it's cw's or the Shim's own process, never the Launcher) and returns the
// first non-shell ancestor found.
func Resolve(callerPID int) (Launcher, error) {
	pid := callerPID
	for depth := 0; depth < maxWalkDepth; depth++ {
		ppid, err := parentOf(pid)
		if err != nil {
			return Launcher{}, fmt.Errorf("identity: reading parent of pid %d: %w", pid, err)
		}
		if ppid <= 1 {
			return Launcher{}, fmt.Errorf("identity: no recognized Launcher found walking ancestry from pid %d (reached the top of the process tree)", callerPID)
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
		key, err := classify(exePath, name)
		if err != nil {
			return Launcher{}, err
		}
		startTime, err := StartTime(pid)
		if err != nil {
			return Launcher{}, fmt.Errorf("identity: reading start time of pid %d: %w", pid, err)
		}
		return Launcher{Key: key, PID: pid, StartTime: startTime}, nil
	}
	return Launcher{}, fmt.Errorf("identity: ancestry walk from pid %d exceeded %d hops without finding a Launcher", callerPID, maxWalkDepth)
}

// IsAlive reports whether pid is still running the same process StartTime
// recorded — false either if the process is gone, or if the PID has since
// been reused by an unrelated process (a new process always gets a
// different start time).
func IsAlive(pid int, startTime uint64) bool {
	current, err := StartTime(pid)
	if err != nil {
		return false
	}
	return current == startTime
}

// StartTime reads a process's start time (field 22 of /proc/<pid>/stat, in
// clock ticks since boot) — a cheap, kernel-provided way to tell "the same
// process" from "a different process that got the same PID after the first
// one exited."
func StartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// The process name field (2nd, in parentheses) can itself contain
	// spaces or parentheses, so scan for the *last* ")" rather than
	// splitting on spaces naively.
	closeParen := strings.LastIndexByte(string(data), ')')
	if closeParen == -1 {
		return 0, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	fields := strings.Fields(string(data)[closeParen+1:])
	// After the ")", fields are: state(1) ppid(2) pgrp(3) session(4)
	// tty_nr(5) tpgid(6) flags(7) ... starttime is field 22 overall, i.e.
	// index 22-3=19 in this post-")" slice (1-indexed state is index 0).
	const startTimeIndex = 19
	if len(fields) <= startTimeIndex {
		return 0, fmt.Errorf("unexpected /proc/%d/stat field count", pid)
	}
	return strconv.ParseUint(fields[startTimeIndex], 10, 64)
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
