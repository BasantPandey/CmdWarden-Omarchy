package harden

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
