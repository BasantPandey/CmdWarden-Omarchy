package shim

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDriftCleanForFreshInstall(t *testing.T) {
	isolateStore(t)
	putFakeCWOnPath(t)

	miseDataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", miseDataDir)
	toolPath := filepath.Join(miseDataDir, "installs", "testtool", "1.0.0", "testtool")
	writeFakeBinary(t, toolPath, "REAL")

	// Put the tool's directory on PATH so exec.LookPath("testtool") finds
	// it, mirroring how a real shell would resolve it after harden.
	t.Setenv("PATH", filepath.Dir(toolPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := Install("testtool", toolPath); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	drifts, err := DetectDrift()
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if len(drifts) != 0 {
		t.Errorf("expected no drift right after install, got %+v", drifts)
	}
}

func TestDetectDriftFlagsNewVersionAfterUpgrade(t *testing.T) {
	isolateStore(t)
	putFakeCWOnPath(t)

	miseDataDir := t.TempDir()
	t.Setenv("MISE_DATA_DIR", miseDataDir)
	toolDir := filepath.Join(miseDataDir, "installs", "testtool")
	oldPath := filepath.Join(toolDir, "1.0.0", "testtool")
	writeFakeBinary(t, oldPath, "REAL_OLD")

	// A stable "current" symlink is how a real mise install keeps the
	// launcher-visible path constant across version bumps; PATH points at
	// the symlink, not the version directory.
	stablePath := filepath.Join(toolDir, "current", "testtool")
	if err := os.MkdirAll(filepath.Dir(stablePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(oldPath, stablePath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(stablePath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := Install("testtool", stablePath); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Simulate an upgrade: a new version directory appears with a fresh,
	// unshimmed binary, and the stable symlink is repointed at it — the
	// old (now-shimmed) version directory is left behind, orphaned.
	newPath := filepath.Join(toolDir, "1.1.0", "testtool")
	writeFakeBinary(t, newPath, "REAL_NEW")
	if err := os.Remove(stablePath); err != nil {
		t.Fatalf("removing old symlink: %v", err)
	}
	if err := os.Symlink(newPath, stablePath); err != nil {
		t.Fatalf("re-symlinking: %v", err)
	}

	drifts, err := DetectDrift()
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if len(drifts) != 1 || drifts[0].Tool != "testtool" {
		t.Fatalf("expected exactly one drift for testtool, got %+v", drifts)
	}
}
