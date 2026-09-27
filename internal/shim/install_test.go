package shim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// putFakeCWOnPath drops a trivial "cw" script that just echoes its argv
// onto PATH, so tests can install a real Shim script (which bakes in
// selfpath.Sibling("cw")'s result) without needing the real cw binary or a
// running agent.
func putFakeCWOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho FAKE_CW \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cw"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake cw: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeFakeBinary(t *testing.T, path, marker string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "#!/bin/sh\necho " + marker + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}
}

func TestInstallOccupiedShimForMiseProvenance(t *testing.T) {
	isolateStore(t)
	putFakeCWOnPath(t)

	miseDataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", miseDataDir)
	toolPath := filepath.Join(miseDataDir, "installs", "testtool", "1.0.0", "testtool")
	writeFakeBinary(t, toolPath, "REAL_TESTTOOL")

	pin, err := Install("testtool", toolPath)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if pin.Mode != ModeOccupied {
		t.Errorf("Mode = %q, want %q", pin.Mode, ModeOccupied)
	}
	realPath := toolPath + ".cmdwarden-real"
	if _, err := os.Stat(realPath); err != nil {
		t.Errorf("expected real binary at %s: %v", realPath, err)
	}

	// The original path is now the shim script.
	out, err := exec.Command("sh", toolPath, "arg1", "arg2").CombinedOutput()
	if err != nil {
		t.Fatalf("running installed shim script: %v (%s)", err, out)
	}
	got := string(out)
	for _, want := range []string{"FAKE_CW", "shim-exec", "--tool", "testtool", realPath} {
		if !strings.Contains(got, want) {
			t.Errorf("shim script output %q does not contain %q", got, want)
		}
	}

	if err := Uninstall("testtool"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Errorf("expected %s to be gone after uninstall (renamed back)", realPath)
	}
	restoredOut, err := exec.Command(toolPath, "x").CombinedOutput()
	if err != nil {
		t.Fatalf("running restored real binary: %v (%s)", err, restoredOut)
	}
	if !strings.Contains(string(restoredOut), "REAL_TESTTOOL") {
		t.Errorf("restored binary output = %q, want it to contain REAL_TESTTOOL", restoredOut)
	}
}

func TestInstallRejectsAlreadyShimmedTool(t *testing.T) {
	isolateStore(t)
	putFakeCWOnPath(t)

	miseDataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", miseDataDir)
	toolPath := filepath.Join(miseDataDir, "installs", "testtool", "1.0.0", "testtool")
	writeFakeBinary(t, toolPath, "REAL_TESTTOOL")

	if _, err := Install("testtool", toolPath); err != nil {
		t.Fatalf("first Install failed: %v", err)
	}
	if _, err := Install("testtool", toolPath); err == nil {
		t.Fatal("expected second Install of the same tool to fail")
	}
}

func TestInstallRejectsUnmanagedBinary(t *testing.T) {
	isolateStore(t)
	putFakeCWOnPath(t)
	t.Setenv("MISE_DATA_DIR", filepath.Join(t.TempDir(), "unused"))

	toolPath := filepath.Join(t.TempDir(), "randomtool")
	writeFakeBinary(t, toolPath, "REAL")

	if _, err := Install("randomtool", toolPath); err == nil {
		t.Fatal("expected Install to reject an Unmanaged (neither pacman nor mise) binary")
	}
}

// putFakePacmanOnPath makes identity.ClassifyBinary report pacman ownership
// for every path, the same way it decides pacman provenance: `pacman -Qo`.
func putFakePacmanOnPath(t *testing.T, pkg string) {
	t.Helper()
	dir := t.TempDir()
	// pacman -Qo <path> → $1=-Qo, $2=path. The trailing version keeps the
	// space identity's owner regex requires after the package name.
	script := "#!/bin/sh\necho \"$2 is owned by " + pkg + " 1.0.0-1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake pacman: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func isolateUserDirs(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return home
}

func assertNoPathPrepend(t *testing.T, home string) {
	t.Helper()
	scriptPath := filepath.Join(home, ".local", "share", "cmdwarden", bootstrapFileName)
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("PATH bootstrap %s exists, err=%v", scriptPath, err)
	}
	envD := filepath.Join(home, ".config", "environment.d", "50-cmdwarden-shim.conf")
	if _, err := os.Stat(envD); !os.IsNotExist(err) {
		t.Errorf("environment.d PATH prepend %s exists, err=%v", envD, err)
	}
	for _, rel := range []string{".profile", ".bashrc", ".zshrc", ".config/uwsm/env.d/50-cmdwarden"} {
		data, err := os.ReadFile(filepath.Join(home, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Errorf("reading %s: %v", rel, err)
			continue
		}
		if strings.Contains(string(data), beginMarker) {
			t.Errorf("%s contains a PATH prepend block", rel)
		}
	}
}

func TestInstallOccupiedShimForPacmanProvenance(t *testing.T) {
	home := isolateUserDirs(t)
	putFakeCWOnPath(t)
	putFakePacmanOnPath(t, "github-cli")
	t.Setenv("MISE_DATA_DIR", filepath.Join(t.TempDir(), "unused-mise"))

	toolPath := filepath.Join(t.TempDir(), "gh")
	writeFakeBinary(t, toolPath, "REAL_GH")

	pin, err := Install("gh", toolPath)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if pin.Mode != ModeOccupied {
		t.Fatalf("Mode = %q, want %q", pin.Mode, ModeOccupied)
	}
	if pin.Channel != contracts.ChannelPacman || pin.ChannelTool != "github-cli" {
		t.Errorf("channel = %s:%s, want pacman:github-cli", pin.Channel, pin.ChannelTool)
	}
	resolved, err := filepath.EvalSymlinks(toolPath)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if pin.OriginalPath != resolved || pin.ShimPath != resolved {
		t.Errorf("shim path = original %q shim %q, want both %q", pin.OriginalPath, pin.ShimPath, resolved)
	}
	realPath := resolved + ".cmdwarden-real"
	if pin.RealBinaryPath != realPath {
		t.Errorf("RealBinaryPath = %q, want %q", pin.RealBinaryPath, realPath)
	}
	if _, err := os.Stat(realPath); err != nil {
		t.Errorf("expected the real binary moved aside to %s: %v", realPath, err)
	}

	// The original absolute path is the shim. A clean sh -c with a reset
	// PATH still executes that path, so it hits the shim rather than the
	// real binary. No PATH prepend is required for that.
	cmd := exec.Command("sh", "-c", "\"$1\" arg1", "sh", toolPath)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running occupied path from a clean sh -c: %v (%s)", err, out)
	}
	got := string(out)
	for _, want := range []string{"FAKE_CW", "shim-exec", "--tool", "gh", realPath} {
		if !strings.Contains(got, want) {
			t.Errorf("shim output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "REAL_GH") {
		t.Errorf("original path ran the real binary: %q", got)
	}
	assertNoPathPrepend(t, home)

	if err := Uninstall("gh"); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if _, ok, err := GetPin("gh"); err != nil {
		t.Fatalf("GetPin: %v", err)
	} else if ok {
		t.Fatal("pin still recorded after uninstall")
	}
	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Errorf("expected %s to be gone after uninstall (renamed back)", realPath)
	}
	restored, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("reading restored binary: %v", err)
	}
	if !strings.Contains(string(restored), "REAL_GH") {
		t.Errorf("restored file = %q, want the original binary", restored)
	}
	assertNoPathPrepend(t, home)
}

func TestUninstallUnshimmedToolFails(t *testing.T) {
	isolateStore(t)
	if err := Uninstall("never-shimmed"); err == nil {
		t.Fatal("expected Uninstall to fail for a tool that was never shimmed")
	}
}
