package shim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestUninstallUnshimmedToolFails(t *testing.T) {
	isolateStore(t)
	if err := Uninstall("never-shimmed"); err == nil {
		t.Fatal("expected Uninstall to fail for a tool that was never shimmed")
	}
}
