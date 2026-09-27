package secretservice

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	// VaultCollectionLabel is the Secret Service collection CmdWarden stores
	// its own items in. See EnsureCollection for the fallback when the
	// backend cannot create one.
	VaultCollectionLabel = "CmdWarden"
	// VaultItemSchema tags every item this vault creates so a search never
	// matches gh's own items in the same collection.
	VaultItemSchema = "org.cmdwarden.Secret"
)

// loginCollectionPath is the collection gh's keyring (zalando/go-keyring
// v0.2.8 GetLoginCollection) uses when that path is in Collections. The
// default alias is the wallet only when login is absent.
const loginCollectionPath dbus.ObjectPath = "/org/freedesktop/secrets/collection/login"

// ErrServiceMissing, ErrServiceLocked, and ErrServiceWrongWallet are the
// three ways a Secret Service cannot hold a token. Callers wrap them with
// a keyring prefix; the text is part of the caller-visible error.
var (
	ErrServiceMissing     = errors.New("Secret Service is missing")
	ErrServiceLocked      = errors.New("Secret Service is locked")
	ErrServiceWrongWallet = errors.New("Secret Service wallet cannot hold the token")
)

// selectGHWallet reports the collection gh itself would unlock. The login
// collection wins whenever it is in the service's collection list, even if
// the default alias is unset or points somewhere else. The default alias is
// the wallet only when login is absent.
func selectGHWallet(collections []dbus.ObjectPath) (dbus.ObjectPath, bool) {
	for _, path := range collections {
		if path == loginCollectionPath {
			return loginCollectionPath, true
		}
	}
	return "", false
}

// classifyWalletUnlock checks the unlock result for the wallet
// selectGHWallet chose. login is true when that wallet is the login
// collection. A prompt or unlock error is locked. Unlocking some other
// collection instead of login is the wrong wallet.
func classifyWalletUnlock(login bool, target dbus.ObjectPath, unlocked []dbus.ObjectPath, unlockErr error) error {
	if unlockErr != nil {
		return fmt.Errorf("%w: %v", ErrServiceLocked, unlockErr)
	}
	if login {
		if len(unlocked) != 1 || unlocked[0] != target {
			return fmt.Errorf("%w: unlocked %v, not the login collection %s", ErrServiceWrongWallet, unlocked, target)
		}
		return nil
	}
	if len(unlocked) != 1 {
		return fmt.Errorf("%w: default collection did not unlock", ErrServiceWrongWallet)
	}
	return nil
}

// ProbeHold reports whether the connected Secret Service can hold a token
// in the collection gh would use. There is no separate result for a desktop
// session, an SSH session, or a headless host.
func (c *Client) ProbeHold() error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("%w: no Secret Service connection", ErrServiceMissing)
	}
	collections, err := c.collections()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrServiceMissing, err)
	}
	target, err := c.ghWallet(collections)
	if err != nil {
		return err
	}
	_, login := selectGHWallet(collections)
	unlocked, unlockErr := c.Unlock([]dbus.ObjectPath{target})
	return classifyWalletUnlock(login, target, unlocked, unlockErr)
}

// ghWallet is the collection a token write may use: login when gh would
// use login, otherwise the default alias. It does not substitute a
// different alias while login exists.
func (c *Client) ghWallet(collections []dbus.ObjectPath) (dbus.ObjectPath, error) {
	if path, ok := selectGHWallet(collections); ok {
		return path, nil
	}
	alias, err := c.readDefaultAlias()
	return walletWithoutLogin(alias, err)
}

// walletWithoutLogin is the fallback when collection/login is not present.
// An unset default alias cannot hold the token.
func walletWithoutLogin(alias dbus.ObjectPath, aliasErr error) (dbus.ObjectPath, error) {
	if aliasErr != nil {
		return "", fmt.Errorf("%w: %v", ErrServiceWrongWallet, aliasErr)
	}
	if alias == "" || alias == nullObjectPath {
		return "", fmt.Errorf("%w: no login collection and no default collection", ErrServiceWrongWallet)
	}
	return alias, nil
}

func (c *Client) collections() ([]dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)
	variant, err := svc.GetProperty(ifaceService + ".Collections")
	if err != nil {
		return nil, fmt.Errorf("reading Collections property: %w", err)
	}
	var collections []dbus.ObjectPath
	if err := variant.Store(&collections); err != nil {
		return nil, fmt.Errorf("decoding Collections property: %w", err)
	}
	return collections, nil
}

func (c *Client) readDefaultAlias() (dbus.ObjectPath, error) {
	svc := c.conn.Object(serviceName, servicePath)
	call := svc.Call(ifaceService+".ReadAlias", 0, "default")
	if call.Err != nil {
		return "", call.Err
	}
	var alias dbus.ObjectPath
	if err := call.Store(&alias); err != nil {
		return "", err
	}
	return alias, nil
}

// SaveNamed stores value under name, replacing every existing match.
// Ensuring the collection happens before any delete, so a wallet that
// cannot hold the token does not drop a previously stored copy.
func (c *Client) SaveNamed(name, value string) error {
	collection, err := c.EnsureCollection(VaultCollectionLabel)
	if err != nil {
		return fmt.Errorf("resolving vault collection: %w", err)
	}
	if err := c.DeleteNamed(name); err != nil {
		return fmt.Errorf("clearing existing copies of secret %q before save: %w", name, err)
	}
	attrs := map[string]string{"xdg:schema": VaultItemSchema, "name": name}
	if _, err := c.CreateOrReplaceItem(collection, "CmdWarden secret: "+name, attrs, []byte(value)); err != nil {
		return fmt.Errorf("saving secret %q: %w", name, err)
	}
	return nil
}

// LookupNamed returns the stored value for name. ok is false when no item
// matches; that is not an error.
func (c *Client) LookupNamed(name string) (string, bool, error) {
	matches, err := c.searchNamed(name)
	if err != nil {
		return "", false, err
	}
	if len(matches) == 0 {
		return "", false, nil
	}
	value, err := c.GetSecretValue(matches[0])
	if err != nil {
		return "", false, fmt.Errorf("releasing secret %q: %w", name, err)
	}
	return string(value), true, nil
}

// DeleteNamed removes every item stored under name. No match is not an error.
func (c *Client) DeleteNamed(name string) error {
	matches, err := c.searchNamed(name)
	if err != nil {
		return err
	}
	for _, item := range matches {
		if err := c.DeleteItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) searchNamed(name string) ([]dbus.ObjectPath, error) {
	matches, err := c.Search(map[string]string{"xdg:schema": VaultItemSchema, "name": name})
	if err != nil {
		return nil, fmt.Errorf("searching vault: %w", err)
	}
	return matches, nil
}
