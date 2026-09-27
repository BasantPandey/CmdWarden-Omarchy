package agentd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
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

func TestImportGHTokenFailsClosed(t *testing.T) {
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		t.Setenv(key, "")
	}
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)

	t.Run("missing keyring", func(t *testing.T) {
		a := &Agent{}
		source, err := a.ImportGHToken("github.com")
		if source != "" || err == nil {
			t.Fatalf("source=%q err=%v, want missing keyring", source, err)
		}
		if !strings.Contains(err.Error(), ghauth.ErrKeyringMissing.Error()) {
			t.Fatalf("err = %v, want the missing-keyring failure", err)
		}
		if strings.Contains(err.Error(), "not running") {
			t.Fatalf("err = %v, import decision was not reached", err)
		}
	})

	t.Run("locked", func(t *testing.T) {
		a := &Agent{tokenKeyring: &memKeyring{holdErr: ghauth.ErrKeyringLocked}}
		_, err := a.ImportGHToken("github.com")
		if err == nil || !strings.Contains(err.Error(), ghauth.ErrKeyringLocked.Error()) {
			t.Fatalf("err = %v, want locked keyring", err)
		}
	})

	t.Run("wrong wallet", func(t *testing.T) {
		a := &Agent{tokenKeyring: &memKeyring{holdErr: ghauth.ErrKeyringWrongWallet}}
		_, err := a.ImportGHToken("github.com")
		if err == nil || !strings.Contains(err.Error(), ghauth.ErrKeyringWrongWallet.Error()) {
			t.Fatalf("err = %v, want wrong-wallet keyring", err)
		}
	})

	t.Run("plaintext hosts.yml", func(t *testing.T) {
		body := []byte("github.com:\n  oauth_token: plaintext-oauth-token-value\n")
		path := filepath.Join(dir, "hosts.yml")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		kr := &memKeyring{}
		a := &Agent{tokenKeyring: kr}
		source, err := a.ImportGHToken("github.com")
		if source != "" || err == nil || !strings.Contains(err.Error(), ghauth.ErrPlaintextHosts.Error()) {
			t.Fatalf("source=%q err=%v, want plaintext refusal", source, err)
		}
		if len(kr.saved) != 0 {
			t.Fatal("plaintext token was copied into the vault")
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || string(after) != string(body) {
			t.Fatalf("hosts.yml changed: %q err=%v", after, readErr)
		}
		os.Remove(path)
	})

	t.Run("env token save fails", func(t *testing.T) {
		t.Setenv("GH_TOKEN", "env-token-value")
		kr := &memKeyring{saveErr: errors.New("vault write failed")}
		a := &Agent{tokenKeyring: kr}
		source, err := a.ImportGHToken("github.com")
		if source != "" || err == nil {
			t.Fatalf("source=%q err=%v, want write failure and no source label", source, err)
		}
		if _, ok := kr.saved[contracts.GHVaultSecretName("github.com")]; ok {
			t.Fatal("environment token stored despite vault write failure")
		}
	})

	t.Run("env token save succeeds", func(t *testing.T) {
		t.Setenv("GH_TOKEN", "env-token-value")
		kr := &memKeyring{}
		a := &Agent{tokenKeyring: kr}
		source, err := a.ImportGHToken("github.com")
		if err != nil {
			t.Fatalf("ImportGHToken: %v", err)
		}
		if source != ghauth.SourceEnvGHToken {
			t.Fatalf("source = %q, want %q", source, ghauth.SourceEnvGHToken)
		}
		if kr.saved[contracts.GHVaultSecretName("github.com")] != os.Getenv("GH_TOKEN") {
			t.Fatal("vault entry is not the environment token")
		}
	})
}
