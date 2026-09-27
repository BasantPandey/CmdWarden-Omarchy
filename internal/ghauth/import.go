package ghauth

import (
	"errors"
	"fmt"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// ErrKeyringMissing, ErrKeyringLocked, and ErrKeyringWrongWallet name the
// three Secret Service failures that stop a token import. The error text
// is the caller-visible result.
var (
	ErrKeyringMissing     = errors.New("keyring: Secret Service is missing")
	ErrKeyringLocked      = errors.New("keyring: Secret Service is locked")
	ErrKeyringWrongWallet = errors.New("keyring: Secret Service wallet cannot hold the token")
	// ErrPlaintextHosts refuses a token whose active source is plaintext
	// hosts.yml. The token is not copied and hosts.yml is not written.
	ErrPlaintextHosts = errors.New("ghauth: refusing to copy a plaintext hosts.yml token")
)

// Keyring is the Secret Service session an import stores into and, when
// the active token is not in the environment, reads gh's own entry from.
type Keyring interface {
	// Hold reports whether this Secret Service can hold a token.
	Hold() error
	// Save stores value under name. A non-nil error means value was not stored.
	Save(name, value string) error
	// GHToken returns gh's own keyring token for host, or ("", "", nil)
	// when gh has no keyring entry.
	GHToken(host string) (token, user string, err error)
	Close() error
}

// Import copies gh's active token for host into kr.
//
// It returns a secret-free source label only after the vault write
// succeeds. A missing, locked, or wrong-wallet Secret Service fails before
// that write. A token whose active source is plaintext hosts.yml fails
// before that write: the token is not copied and hosts.yml is not created
// or changed. An environment token is stored only when Save succeeds.
// Those refusals do not depend on a desktop session, an SSH session, or a
// headless host; none of those is a path that copies the token into plaintext.
func Import(host string, kr Keyring) (string, error) {
	if kr == nil {
		return "", fmt.Errorf("%w", ErrKeyringMissing)
	}
	if err := kr.Hold(); err != nil {
		return "", err
	}

	token, source, err := TokenFromEnvOrConfig(host)
	if err != nil {
		return "", fmt.Errorf("ghauth: reading gh config: %w", err)
	}
	if source == SourceHostsYAML {
		return "", fmt.Errorf("%w for %q", ErrPlaintextHosts, host)
	}
	if token == "" {
		kt, user, kerr := kr.GHToken(host)
		if kerr != nil {
			return "", fmt.Errorf("ghauth: searching gh's keyring entry: %w", kerr)
		}
		if kt == "" {
			return "", fmt.Errorf(
				"ghauth: no active gh token found for %q via env vars, hosts.yml, or the %q keyring entry — is gh logged in?",
				host, KeyringServiceName(host))
		}
		token = kt
		source = "keyring (" + KeyringServiceName(host) + ", user " + user + ")"
	}

	name := contracts.GHVaultSecretName(host)
	if err := kr.Save(name, token); err != nil {
		return "", err
	}
	return source, nil
}
