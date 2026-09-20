package shim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

func TestEnsurePathBootstrapWritesScriptAndSourcesFromEveryTarget(t *testing.T) {
	home := isolateHome(t)

	if err := ensurePathBootstrap(); err != nil {
		t.Fatalf("ensurePathBootstrap failed: %v", err)
	}

	scriptPath, err := bootstrapScriptPath()
	if err != nil {
		t.Fatalf("bootstrapScriptPath failed: %v", err)
	}
	if _, err := os.Stat(scriptPath); err != nil {
		t.Errorf("expected bootstrap script at %s: %v", scriptPath, err)
	}

	for _, target := range bootstrapTargets() {
		data, err := os.ReadFile(target)
		if err != nil {
			t.Errorf("expected %s to exist and contain the bootstrap block: %v", target, err)
			continue
		}
		if !strings.Contains(string(data), beginMarker) || !strings.Contains(string(data), endMarker) {
			t.Errorf("%s does not contain the cmdwarden marker block", target)
		}
	}

	envDPath, err := environmentDPath()
	if err != nil {
		t.Fatalf("environmentDPath failed: %v", err)
	}
	data, err := os.ReadFile(envDPath)
	if err != nil {
		t.Fatalf("expected environment.d file at %s: %v", envDPath, err)
	}
	if !strings.HasPrefix(string(data), "BASH_ENV=") {
		t.Errorf("environment.d file = %q, want a BASH_ENV= line", data)
	}
	_ = home
}

func TestEnsurePathBootstrapIsIdempotent(t *testing.T) {
	isolateHome(t)

	if err := ensurePathBootstrap(); err != nil {
		t.Fatalf("first ensurePathBootstrap failed: %v", err)
	}
	if err := ensurePathBootstrap(); err != nil {
		t.Fatalf("second ensurePathBootstrap failed: %v", err)
	}

	for _, target := range bootstrapTargets() {
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading %s: %v", target, err)
		}
		if count := strings.Count(string(data), beginMarker); count != 1 {
			t.Errorf("%s has %d begin markers after two installs, want 1", target, count)
		}
	}
}

func TestRemovePathBootstrapCleansUpEveryTarget(t *testing.T) {
	isolateHome(t)

	if err := ensurePathBootstrap(); err != nil {
		t.Fatalf("ensurePathBootstrap failed: %v", err)
	}
	// Simulate a user's own content around the block, to prove removal is
	// surgical rather than truncating the whole file.
	for _, target := range bootstrapTargets() {
		existing, _ := os.ReadFile(target)
		withUserContent := "# my own alias\nalias ll='ls -la'\n" + string(existing)
		if err := os.WriteFile(target, []byte(withUserContent), 0o644); err != nil {
			t.Fatalf("seeding user content into %s: %v", target, err)
		}
	}

	if err := removePathBootstrap(); err != nil {
		t.Fatalf("removePathBootstrap failed: %v", err)
	}

	scriptPath, err := bootstrapScriptPath()
	if err != nil {
		t.Fatalf("bootstrapScriptPath failed: %v", err)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("expected bootstrap script to be removed")
	}

	envDPath, err := environmentDPath()
	if err != nil {
		t.Fatalf("environmentDPath failed: %v", err)
	}
	if _, err := os.Stat(envDPath); !os.IsNotExist(err) {
		t.Errorf("expected environment.d file to be removed")
	}

	for _, target := range bootstrapTargets() {
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading %s: %v", target, err)
		}
		content := string(data)
		if strings.Contains(content, beginMarker) || strings.Contains(content, endMarker) {
			t.Errorf("%s still contains the cmdwarden marker block after removal", target)
		}
		if !strings.Contains(content, "alias ll=") {
			t.Errorf("%s lost the user's own content during removal", target)
		}
	}
}
