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

// ErrServiceMissing, ErrServiceLocked, and ErrServiceWrongWallet are the
// three ways a Secret Service cannot hold a token. Callers wrap them with
// a keyring prefix; the text is part of the caller-visible error.
var (
	ErrServiceMissing     = errors.New("Secret Service is missing")
	ErrServiceLocked      = errors.New("Secret Service is locked")
	ErrServiceWrongWallet = errors.New("Secret Service wallet cannot hold the token")
)

// classifyProbe decides whether a probed Secret Service can hold a token.
// A service that cannot be reached is missing. A default collection that
// will not unlock is locked. No default collection means this wallet is
// not one that can hold the token. There is no separate result for a
// desktop session, an SSH session, or a headless host.
func classifyProbe(serviceErr error, alias dbus.ObjectPath, aliasErr, unlockErr error) error {
	if serviceErr != nil {
		return fmt.Errorf("%w: %v", ErrServiceMissing, serviceErr)
	}
	if aliasErr != nil {
		return fmt.Errorf("%w: %v", ErrServiceWrongWallet, aliasErr)
	}
	if alias == "" || alias == nullObjectPath {
		return fmt.Errorf("%w: no default collection", ErrServiceWrongWallet)
	}
	if unlockErr != nil {
		return fmt.Errorf("%w: %v", ErrServiceLocked, unlockErr)
	}
	return nil
}

// ProbeHold reports whether the connected Secret Service can hold a token.
func (c *Client) ProbeHold() error {
	if c == nil || c.conn == nil {
		return classifyProbe(errors.New("no Secret Service connection"), "", nil, nil)
	}
	svc := c.conn.Object(serviceName, servicePath)
	var serviceErr error
	if _, err := svc.GetProperty(ifaceService + ".Collections"); err != nil {
		serviceErr = err
	}
	var (
		alias    dbus.ObjectPath
		aliasErr error
	)
	if serviceErr == nil {
		call := svc.Call(ifaceService+".ReadAlias", 0, "default")
		if call.Err != nil {
			aliasErr = call.Err
		} else if err := call.Store(&alias); err != nil {
			aliasErr = err
		}
	}
	var unlockErr error
	if serviceErr == nil && aliasErr == nil && alias != "" && alias != nullObjectPath {
		if _, err := c.Unlock([]dbus.ObjectPath{alias}); err != nil {
			unlockErr = err
		}
	}
	return classifyProbe(serviceErr, alias, aliasErr, unlockErr)
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
