package ghauth

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHostsYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing hosts.yml: %v", err)
	}
}

func TestEnvVarTakesPrecedenceOverHostsYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	writeHostsYAML(t, dir, "github.com:\n  oauth_token: from-file\n")
	t.Setenv("GH_TOKEN", "from-env")

	token, source, err := TokenFromEnvOrConfig("github.com")
	if err != nil {
		t.Fatalf("TokenFromEnvOrConfig failed: %v", err)
	}
	if token != "from-env" || source != SourceEnvGHToken {
		t.Errorf("got (%q, %q), want (\"from-env\", %q)", token, source, SourceEnvGHToken)
	}
}

func TestGITHUB_TOKENFallsBackWhenGH_TOKENUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	t.Setenv("GITHUB_TOKEN", "from-github-token")

	token, source, err := TokenFromEnvOrConfig("github.com")
	if err != nil {
		t.Fatalf("TokenFromEnvOrConfig failed: %v", err)
	}
	if token != "from-github-token" || source != SourceEnvGitHubToken {
		t.Errorf("got (%q, %q), want (\"from-github-token\", %q)", token, source, SourceEnvGitHubToken)
	}
}

func TestEnterpriseHostUsesEnterpriseEnvVars(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	t.Setenv("GH_TOKEN", "should-not-be-used") // only applies to github.com/localhost
	t.Setenv("GH_ENTERPRISE_TOKEN", "enterprise-token")

	token, source, err := TokenFromEnvOrConfig("ghe.example.com")
	if err != nil {
		t.Fatalf("TokenFromEnvOrConfig failed: %v", err)
	}
	if token != "enterprise-token" || source != SourceEnvGHEnterpriseToken {
		t.Errorf("got (%q, %q), want (\"enterprise-token\", %q)", token, source, SourceEnvGHEnterpriseToken)
	}
}

func TestHostsYAMLTopLevelOAuthToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	writeHostsYAML(t, dir, `github.com:
    users:
        octocat:
            oauth_token: nested-token
    user: octocat
    oauth_token: top-level-token
`)

	token, source, err := TokenFromEnvOrConfig("github.com")
	if err != nil {
		t.Fatalf("TokenFromEnvOrConfig failed: %v", err)
	}
	if token != "top-level-token" || source != SourceHostsYAML {
		t.Errorf("got (%q, %q), want (\"top-level-token\", %q)", token, source, SourceHostsYAML)
	}
}

func TestHostsYAMLFallsBackToPerUserToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	writeHostsYAML(t, dir, `github.com:
    users:
        octocat:
            oauth_token: nested-token
    user: octocat
`)

	token, source, err := TokenFromEnvOrConfig("github.com")
	if err != nil {
		t.Fatalf("TokenFromEnvOrConfig failed: %v", err)
	}
	if token != "nested-token" || source != SourceHostsYAML {
		t.Errorf("got (%q, %q), want (\"nested-token\", %q)", token, source, SourceHostsYAML)
	}
}

func TestNoTokenFoundReturnsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir) // no hosts.yml at all

	token, source, err := TokenFromEnvOrConfig("github.com")
	if err != nil {
		t.Fatalf("expected nil error for a missing hosts.yml, got %v", err)
	}
	if token != "" || source != "" {
		t.Errorf("got (%q, %q), want (\"\", \"\")", token, source)
	}
}

func TestKeyringServiceName(t *testing.T) {
	if got := KeyringServiceName("github.com"); got != "gh:github.com" {
		t.Errorf("KeyringServiceName(github.com) = %q, want gh:github.com", got)
	}
}
