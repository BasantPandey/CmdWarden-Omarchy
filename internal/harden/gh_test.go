package harden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/shim"
)

type memKeyring struct {
	holdErr error
	saveErr error
	saved   map[string]string
}

func (m *memKeyring) Hold() error                            { return m.holdErr }
func (m *memKeyring) Close() error                           { return nil }
func (m *memKeyring) GHToken(string) (string, string, error) { return "", "", nil }
func (m *memKeyring) Save(name, value string) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if m.saved == nil {
		m.saved = map[string]string{}
	}
	m.saved[name] = value
	return nil
}

func TestHardenGHImportFailureLeavesNoPin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(key, "")
	}

	ghDir := t.TempDir()
	ghPath := filepath.Join(ghDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\necho REAL_GH\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	configDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", configDir)
	hosts := filepath.Join(configDir, "hosts.yml")

	origOpener := openImportKeyring
	t.Cleanup(func() { openImportKeyring = origOpener })

	cases := []struct {
		name    string
		hold    error
		saveErr error
		env     string
		hosts   string
		want    error
	}{
		{name: "missing", hold: ghauth.ErrKeyringMissing, env: "env-token-must-not-be-stored", want: ghauth.ErrKeyringMissing},
		{name: "locked", hold: ghauth.ErrKeyringLocked, env: "env-token-must-not-be-stored", want: ghauth.ErrKeyringLocked},
		{name: "wrong wallet", hold: ghauth.ErrKeyringWrongWallet, env: "env-token-must-not-be-stored", want: ghauth.ErrKeyringWrongWallet},
		{name: "plaintext hosts.yml", hosts: "github.com:\n  oauth_token: plaintext-oauth-token-value\n  user: octocat\n", want: ghauth.ErrPlaintextHosts},
		{name: "vault write failed", saveErr: errors.New("vault write failed"), env: "env-token-value", want: errors.New("vault write failed")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", tc.env)
			var before []byte
			var beforeInfo os.FileInfo
			if tc.hosts != "" {
				before = []byte(tc.hosts)
				if err := os.WriteFile(hosts, before, 0o600); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(hosts)
				if err != nil {
					t.Fatal(err)
				}
				beforeInfo = info
			} else if err := os.Remove(hosts); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}

			kr := &memKeyring{holdErr: tc.hold, saveErr: tc.saveErr}
			openImportKeyring = func() (ghauth.Keyring, error) { return kr, nil }

			_, err := HardenGH(context.Background(), "github.com")
			if err == nil {
				t.Fatal("HardenGH succeeded")
			}
			if !errors.Is(err, tc.want) && !strings.Contains(err.Error(), tc.want.Error()) {
				t.Fatalf("HardenGH err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "not running") {
				t.Fatalf("err = %v, import decision was not reached", err)
			}
			if tc.want == ghauth.ErrPlaintextHosts && strings.Contains(err.Error(), "plaintext-oauth-token-value") {
				t.Fatalf("error copied the token: %v", err)
			}
			if strings.Contains(tc.env, "must-not") && strings.Contains(err.Error(), tc.env) {
				t.Fatalf("error copied the environment token: %v", err)
			}
			if len(kr.saved) != 0 {
				t.Fatal("token was stored on a failed import")
			}

			if _, ok, pinErr := shim.GetPin("gh"); pinErr != nil {
				t.Fatalf("GetPin: %v", pinErr)
			} else if ok {
				t.Fatal("shim pin saved after a failed import")
			}
			got, readErr := os.ReadFile(ghPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Contains(got, []byte("REAL_GH")) || bytes.Contains(got, []byte("shim-exec")) {
				t.Fatalf("gh binary was shimmed: %q", got)
			}
			if _, statErr := os.Stat(ghPath + ".cmdwarden-real"); !os.IsNotExist(statErr) {
				t.Fatalf("real binary was moved aside, stat err=%v", statErr)
			}

			if tc.hosts == "" {
				if _, statErr := os.Stat(hosts); !os.IsNotExist(statErr) {
					t.Fatalf("hosts.yml was created, stat err=%v", statErr)
				}
				return
			}
			after, readErr := os.ReadFile(hosts)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("hosts.yml bytes changed:\n%s", after)
			}
			info, statErr := os.Stat(hosts)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if !info.ModTime().Equal(beforeInfo.ModTime()) {
				t.Fatal("hosts.yml was rewritten")
			}
		})
	}
}

// TestHardenGHReplacesPathPinWithOccupied starts from the old pacman Path
// pin, where LookPath returns the PATH shim. HardenGH must uninstall that
// shim and occupy the real binary instead of installing over the deleted
// shim path.
func TestHardenGHReplacesPathPinWithOccupied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdgConfig := filepath.Join(home, ".config")
	xdgData := filepath.Join(home, ".local", "share")
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("MISE_DATA_DIR", filepath.Join(t.TempDir(), "unused-mise"))
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	for _, key := range []string{"GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(key, "")
	}
	t.Setenv("GH_TOKEN", "env-token-value")

	binDir := t.TempDir()
	cw := "#!/bin/sh\necho FAKE_CW \"$@\"\n"
	pacman := "#!/bin/sh\necho \"$2 is owned by github-cli 1.0.0-1\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "cw"), []byte(cw), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "pacman"), []byte(pacman), 0o755); err != nil {
		t.Fatal(err)
	}

	realDir := t.TempDir()
	realPath := filepath.Join(realDir, "gh")
	if err := os.WriteFile(realPath, []byte("#!/bin/sh\necho REAL_GH \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	shimDir := filepath.Join(xdgData, "cmdwarden", "shims")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shimScript := filepath.Join(shimDir, "gh")
	if err := os.WriteFile(shimScript, []byte("#!/bin/sh\necho PATH_SHIM \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The path shim is first, so a lookup before uninstall returns the
	// script Uninstall deletes — not the real binary behind it.
	t.Setenv("PATH", strings.Join([]string{shimDir, binDir, realDir, os.Getenv("PATH")}, string(os.PathListSeparator)))
	found, err := exec.LookPath("gh")
	if err != nil {
		t.Fatalf("LookPath: %v", err)
	}
	if found != shimScript {
		t.Fatalf("LookPath(gh) = %s, want the path shim %s", found, shimScript)
	}

	storeDir := filepath.Join(xdgConfig, "cmdwarden")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(map[string]shim.Pin{
		"gh": {
			Mode:           shim.ModePath,
			Channel:        contracts.ChannelPacman,
			ChannelTool:    "github-cli",
			OriginalPath:   realPath,
			RealBinaryPath: realPath,
			ShimPath:       shimScript,
			InstalledAt:    time.Now().UTC(),
		},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "shims.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	origOpener := openImportKeyring
	t.Cleanup(func() { openImportKeyring = origOpener })
	openImportKeyring = func() (ghauth.Keyring, error) { return &memKeyring{}, nil }

	pin, err := HardenGH(context.Background(), "github.com")
	if err != nil {
		t.Fatalf("HardenGH: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if pin.Mode != shim.ModeOccupied {
		t.Fatalf("Mode = %q, want occupied", pin.Mode)
	}
	if pin.OriginalPath != resolved || pin.ShimPath != resolved {
		t.Fatalf("occupied path = original %q shim %q, want %q", pin.OriginalPath, pin.ShimPath, resolved)
	}
	realAside := resolved + ".cmdwarden-real"
	if pin.RealBinaryPath != realAside {
		t.Fatalf("RealBinaryPath = %q, want %q", pin.RealBinaryPath, realAside)
	}
	moved, err := os.ReadFile(realAside)
	if err != nil {
		t.Fatalf("reading moved real binary: %v", err)
	}
	if !bytes.Contains(moved, []byte("REAL_GH")) {
		t.Fatalf("moved file = %q, want the real binary", moved)
	}
	occupied, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(occupied, []byte("shim-exec")) {
		t.Fatalf("file at the real path = %q, want the occupied shim", occupied)
	}
	if _, err := os.Stat(shimScript); !os.IsNotExist(err) {
		t.Fatalf("path shim %s still present, err=%v", shimScript, err)
	}

	bootstrap := filepath.Join(xdgData, "cmdwarden", "shim-env-bootstrap.sh")
	if _, err := os.Stat(bootstrap); !os.IsNotExist(err) {
		t.Fatalf("PATH bootstrap %s exists, err=%v", bootstrap, err)
	}
	envD := filepath.Join(home, ".config", "environment.d", "50-cmdwarden-shim.conf")
	if _, err := os.Stat(envD); !os.IsNotExist(err) {
		t.Fatalf("environment.d PATH prepend %s exists, err=%v", envD, err)
	}
	for _, rel := range []string{".profile", ".bashrc", ".zshrc", ".config/uwsm/env.d/50-cmdwarden"} {
		data, err := os.ReadFile(filepath.Join(home, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if strings.Contains(string(data), "# >>> cmdwarden shim path >>>") {
			t.Fatalf("%s contains a PATH prepend block", rel)
		}
	}
}
