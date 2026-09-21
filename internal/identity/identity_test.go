package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func TestMiseToolStableAcrossVersionBump(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", dataDir)

	oldPath := filepath.Join(dataDir, "installs", "claude", "2.1.274", "claude")
	newPath := filepath.Join(dataDir, "installs", "claude", "2.1.280", "claude")

	oldTool, ok := miseTool(oldPath)
	if !ok || oldTool != "claude" {
		t.Fatalf("miseTool(%q) = (%q, %v), want (claude, true)", oldPath, oldTool, ok)
	}
	newTool, ok := miseTool(newPath)
	if !ok || newTool != "claude" {
		t.Fatalf("miseTool(%q) = (%q, %v), want (claude, true)", newPath, newTool, ok)
	}
	if oldTool != newTool {
		t.Errorf("mise tool name changed across a version bump: %q -> %q", oldTool, newTool)
	}
}

func TestMiseToolRejectsPathOutsideInstallTree(t *testing.T) {
	t.Setenv("MISE_DATA_DIR", t.TempDir())

	if _, ok := miseTool("/usr/bin/foot"); ok {
		t.Error("miseTool matched a path outside the mise install tree")
	}
}

func TestHashFileMatchesSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	content := []byte("pretend this is a binary")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}
	want := sha256.Sum256(content)
	if got != hex.EncodeToString(want[:]) {
		t.Errorf("hashFile = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}

func TestClassifyUnmanagedWhenNoChannelClaimsIt(t *testing.T) {
	// An empty MISE_DATA_DIR guarantees no mise match; a fixture path
	// under a temp dir is guaranteed not to be pacman-owned either.
	t.Setenv("MISE_DATA_DIR", filepath.Join(t.TempDir(), "unused"))

	path := filepath.Join(t.TempDir(), "mytool")
	if err := os.WriteFile(path, []byte("binary content"), 0o755); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	key, err := classify(path, "mytool")
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	if key.Channel != contracts.ChannelUnmanaged {
		t.Errorf("Channel = %q, want %q", key.Channel, contracts.ChannelUnmanaged)
	}
	if key.Tool != "mytool" {
		t.Errorf("Tool = %q, want mytool", key.Tool)
	}
	if key.Hash == "" {
		t.Error("Unmanaged Launcher must have a non-empty Hash")
	}
	if key.String() == "" {
		t.Error("String() must not be empty")
	}
}

func TestClassifyMiseTakesPrecedence(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", dataDir)
	path := filepath.Join(dataDir, "installs", "gh", "2.100.0", "gh")

	key, err := classify(path, "gh")
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	if key.Channel != contracts.ChannelMise || key.Tool != "gh" {
		t.Errorf("classify(%q) = %+v, want Channel=mise Tool=gh", path, key)
	}
	if key.String() != "mise:gh" {
		t.Errorf("String() = %q, want mise:gh", key.String())
	}
}

func TestNotLaunchersSkipsCommonShells(t *testing.T) {
	for _, name := range []string{"bash", "zsh", "sh", "dash", "fish", "tmux", "sudo"} {
		if !notLaunchers[name] {
			t.Errorf("expected %q to be treated as a non-Launcher shell/wrapper", name)
		}
	}
	if notLaunchers["foot"] || notLaunchers["claude"] || notLaunchers["gh"] {
		t.Error("a real Launcher/tool name must not be in the shell skip-list")
	}
}

// TestResolveWalksPastShellsToRealLauncher spawns test-binary -> sh -> sleep,
// then resolves identity starting from the sleep process. If shell-skipping
// works, resolution should land on this test binary itself (compiled to a
// throwaway build directory, so it's Unmanaged) rather than on the
// intermediate shell.
func TestResolveWalksPastShellsToRealLauncher(t *testing.T) {
	if _, err := os.Stat("/proc/self/status"); err != nil {
		t.Skip("no /proc on this system")
	}

	// "sleep 30; true" (not just "sleep 30") deliberately defeats the
	// shell's tail-call exec optimization, where a single trailing
	// command replaces the shell's own process image instead of forking
	// — we need sh to stay alive as sleep's actual parent for this test.
	cmd := exec.Command("sh", "-c", "sleep 30; true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sh -c sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Give the shell a moment to exec sleep as its child.
	var sleepPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		children, err := findChildren(cmd.Process.Pid)
		if err == nil && len(children) > 0 {
			sleepPID = children[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sleepPID == 0 {
		t.Fatal("sleep never appeared as a child of sh")
	}

	launcher, err := Resolve(sleepPID)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if launcher.Key.Tool == "sh" || launcher.Key.Tool == "dash" || launcher.Key.Tool == "bash" {
		t.Errorf("Resolve stopped at the shell instead of walking past it: %+v", launcher)
	}
	if launcher.PID == 0 {
		t.Error("expected a non-zero resolved Launcher PID")
	}
	if !IsAlive(launcher.PID, launcher.StartTime) {
		t.Error("expected IsAlive to report the just-resolved Launcher as alive")
	}
}

func TestStartTimeDetectsPIDReuseAsNotAlive(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc on this system")
	}
	if !IsAlive(os.Getpid(), mustStartTime(t, os.Getpid())) {
		t.Error("expected the current process to report alive with its own start time")
	}
	if IsAlive(os.Getpid(), mustStartTime(t, os.Getpid())+1) {
		t.Error("expected a mismatched start time to report not alive")
	}
}

func mustStartTime(t *testing.T, pid int) uint64 {
	t.Helper()
	st, err := StartTime(pid)
	if err != nil {
		t.Fatalf("StartTime failed: %v", err)
	}
	return st
}

// findChildren does a one-shot scan of /proc for pid's direct children —
// good enough for this test's narrow use, not a general-purpose API.
func findChildren(pid int) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var children []int
	for _, e := range entries {
		childPID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a /proc/<pid> entry
		}
		ppid, err := parentOf(childPID)
		if err != nil {
			continue
		}
		if ppid == pid {
			children = append(children, childPID)
		}
	}
	return children, nil
}
