package ghauth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/secretservice"
)

type memKeyring struct {
	holdErr error
	saveErr error
	saved   map[string]string
	ghToken string
	ghUser  string
}

func (m *memKeyring) Hold() error  { return m.holdErr }
func (m *memKeyring) Close() error { return nil }
func (m *memKeyring) GHToken(string) (string, string, error) {
	return m.ghToken, m.ghUser, nil
}
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

func clearTokenEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(key, "")
	}
}

func hostsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	return filepath.Join(dir, "hosts.yml")
}

func TestImportKeyringFailureWritesNothing(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv("GH_TOKEN", "env-token-must-not-be-stored")
	path := hostsPath(t)

	for _, hold := range []error{ErrKeyringMissing, ErrKeyringLocked, ErrKeyringWrongWallet} {
		t.Run(hold.Error(), func(t *testing.T) {
			kr := &memKeyring{holdErr: hold}
			source, err := Import("github.com", kr)
			if err == nil {
				t.Fatal("expected keyring failure")
			}
			if !errors.Is(err, hold) {
				t.Fatalf("Import err = %v, want %v", err, hold)
			}
			if !strings.Contains(err.Error(), "keyring") {
				t.Fatalf("error %q does not name the keyring failure", err)
			}
			if source != "" {
				t.Fatalf("source = %q, want empty", source)
			}
			if len(kr.saved) != 0 {
				t.Fatalf("vault has %d entries after a keyring failure", len(kr.saved))
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("hosts.yml exists after keyring failure, stat err=%v", statErr)
			}
		})
	}
}

func TestImportRejectsPlaintextHostsYAML(t *testing.T) {
	clearTokenEnv(t)
	path := hostsPath(t)
	body := []byte("github.com:\n  oauth_token: plaintext-oauth-token-value\n  user: octocat\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing hosts.yml: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	kr := &memKeyring{}
	source, err := Import("github.com", kr)
	if !errors.Is(err, ErrPlaintextHosts) {
		t.Fatalf("Import err = %v, want plaintext hosts.yml refusal", err)
	}
	if strings.Contains(err.Error(), "plaintext-oauth-token-value") {
		t.Fatalf("error copied the token: %v", err)
	}
	if source != "" {
		t.Fatalf("source = %q, want empty", source)
	}
	if len(kr.saved) != 0 {
		t.Fatal("plaintext token was copied into the vault")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("hosts.yml bytes changed:\n%s", after)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !afterInfo.ModTime().Equal(info.ModTime()) {
		t.Fatal("hosts.yml was rewritten")
	}
}

func TestImportStoresEnvTokenOnlyWhenSaveSucceeds(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv("GH_TOKEN", "env-token-value")
	hostsPath(t)
	name := contracts.GHVaultSecretName("github.com")

	t.Run("save fails", func(t *testing.T) {
		kr := &memKeyring{saveErr: errors.New("vault write failed")}
		source, err := Import("github.com", kr)
		if err == nil {
			t.Fatal("expected vault write failure")
		}
		if source != "" {
			t.Fatalf("source = %q, want empty until the write succeeds", source)
		}
		if _, ok := kr.saved[name]; ok {
			t.Fatal("environment token stored despite vault write failure")
		}
	})

	t.Run("save succeeds", func(t *testing.T) {
		kr := &memKeyring{}
		source, err := Import("github.com", kr)
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if source != SourceEnvGHToken {
			t.Fatalf("source = %q, want %q", source, SourceEnvGHToken)
		}
		got, ok := kr.saved[name]
		if !ok || got != os.Getenv("GH_TOKEN") {
			t.Fatal("vault entry is not the environment token")
		}
	})
}

func TestMapHoldNamesKeyringFailure(t *testing.T) {
	cases := []struct {
		err  error
		want error
	}{
		{fmtWrap(secretservice.ErrServiceMissing), ErrKeyringMissing},
		{fmtWrap(secretservice.ErrServiceLocked), ErrKeyringLocked},
		{fmtWrap(secretservice.ErrServiceWrongWallet), ErrKeyringWrongWallet},
	}
	for _, tc := range cases {
		got := mapHold(tc.err)
		if !errors.Is(got, tc.want) || !strings.Contains(got.Error(), "keyring") {
			t.Errorf("mapHold(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func fmtWrap(err error) error {
	return errors.Join(err)
}
