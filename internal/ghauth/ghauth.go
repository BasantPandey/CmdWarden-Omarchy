// Package ghauth locates gh's own currently-active OAuth token, mirroring
// gh's own resolution order exactly (see
// docs/research/gh-linux-keyring.md): (1) GH_TOKEN/GITHUB_TOKEN (or their
// *_ENTERPRISE_TOKEN siblings for non-github.com hosts), (2) the plaintext
// oauth_token in hosts.yml, (3) the Secret Service keyring entry gh itself
// wrote under service "gh:<hostname>".
//
// This package only implements (1) and (2) — pure env/file I/O, no D-Bus.
// (3) needs a live Secret Service session, which only the agent holds; see
// internal/secretservice and KeyringServiceName below, which the agent uses
// as the fallback when this package finds nothing.
package ghauth

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Source names returned alongside a token, describing where it came from —
// safe to log/print, since they never contain the token itself.
const (
	SourceEnvGHToken           = "env:GH_TOKEN"
	SourceEnvGitHubToken       = "env:GITHUB_TOKEN"
	SourceEnvGHEnterpriseToken = "env:GH_ENTERPRISE_TOKEN"
	SourceEnvGitHubEnterprise  = "env:GITHUB_ENTERPRISE_TOKEN"
	SourceHostsYAML            = "hosts.yml (plaintext)"
)

// DefaultHost is the host cw targets when none is specified — matches gh's
// own default.
const DefaultHost = "github.com"

// KeyringServiceName returns the Secret Service "service" attribute value
// gh itself uses for a given host, e.g. "gh:github.com" — see
// docs/research/gh-linux-keyring.md, keyringServiceName() in cli/cli.
func KeyringServiceName(host string) string {
	return "gh:" + host
}

// TokenFromEnvOrConfig replicates gh's own ActiveToken/TokenFromEnvOrConfig
// order for everything short of the keyring: env vars, then hosts.yml's
// plaintext oauth_token. It returns ("", "", nil) — not an error — when
// neither path has a token; that just means the caller should fall back to
// the keyring.
func TokenFromEnvOrConfig(host string) (token, source string, err error) {
	if host == "github.com" || host == "localhost" {
		if t := os.Getenv("GH_TOKEN"); t != "" {
			return t, SourceEnvGHToken, nil
		}
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t, SourceEnvGitHubToken, nil
		}
	} else {
		if t := os.Getenv("GH_ENTERPRISE_TOKEN"); t != "" {
			return t, SourceEnvGHEnterpriseToken, nil
		}
		if t := os.Getenv("GITHUB_ENTERPRISE_TOKEN"); t != "" {
			return t, SourceEnvGitHubEnterprise, nil
		}
	}

	hosts, err := readHostsYAML()
	if err != nil {
		return "", "", err
	}
	entry, ok := hosts[host]
	if !ok {
		return "", "", nil
	}
	if entry.OAuthToken != "" {
		return entry.OAuthToken, SourceHostsYAML, nil
	}
	if entry.User != "" {
		if user, ok := entry.Users[entry.User]; ok && user.OAuthToken != "" {
			return user.OAuthToken, SourceHostsYAML, nil
		}
	}
	return "", "", nil
}

// ActiveUser returns the hosts.yml "user:" field for host, if any — used to
// disambiguate multiple keyring entries for the same host by username.
func ActiveUser(host string) (string, error) {
	hosts, err := readHostsYAML()
	if err != nil {
		return "", err
	}
	return hosts[host].User, nil
}

type hostsYAMLEntry struct {
	OAuthToken string                        `yaml:"oauth_token"`
	User       string                        `yaml:"user"`
	Users      map[string]hostsYAMLUserEntry `yaml:"users"`
}

type hostsYAMLUserEntry struct {
	OAuthToken string `yaml:"oauth_token"`
}

// readHostsYAML reads gh's hosts.yml, mirroring cli/go-gh's ConfigDir()
// resolution: $GH_CONFIG_DIR, else $XDG_CONFIG_HOME/gh, else ~/.config/gh.
// A missing file is treated as "no hosts configured," not an error.
func readHostsYAML() (map[string]hostsYAMLEntry, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "hosts.yml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]hostsYAMLEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ghauth: reading %s: %w", path, err)
	}

	var hosts map[string]hostsYAMLEntry
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("ghauth: parsing %s: %w", path, err)
	}
	if hosts == nil {
		hosts = map[string]hostsYAMLEntry{}
	}
	return hosts, nil
}

func configDir() (string, error) {
	if dir := os.Getenv("GH_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "gh"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ghauth: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gh"), nil
}
