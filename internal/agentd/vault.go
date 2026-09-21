package agentd

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/secretservice"
)

// vaultCollectionLabel names CmdWarden-Omarchy's own Secret Service
// collection. See secretservice.EnsureCollection for the headless fallback
// when the backend can't prompt to create a new one.
const vaultCollectionLabel = "CmdWarden"

// vaultItemSchema tags every item this vault creates, distinguishing them
// from gh's own (or anyone else's) items sharing the same collection.
const vaultItemSchema = "org.cmdwarden.Secret"

// SaveSecret stores value under name in CmdWarden-Omarchy's vault,
// overwriting any existing secret with that name.
//
// It deletes every existing match for name first, service-wide, rather than
// relying solely on CreateItem's "replace" flag: EnsureCollection's choice
// of collection isn't guaranteed stable across calls (its prompt-based
// CreateCollection path can succeed on one call and fall back to the
// backend's default collection on another — see EnsureCollection's own
// doc), and "replace" only ever matches within the single collection
// CreateItem targets. Without this, a name whose collection choice changed
// since it was last saved would end up with two live items — one in each
// collection — and ReleaseSecret/DeleteSecret would then act on whichever
// one Search happened to list first, silently returning or deleting the
// wrong one. Enforcing "at most one item per name" here, on every write, is
// what keeps that invariant true regardless of EnsureCollection's history.
func (a *Agent) SaveSecret(name string, value string) *dbus.Error {
	svc, err := a.vault()
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := deleteAllVaultMatches(svc, name); err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: clearing existing copies of secret %q before save: %w", name, err))
	}
	collection, err := svc.EnsureCollection(vaultCollectionLabel)
	if err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: resolving vault collection: %w", err))
	}
	attrs := map[string]string{"xdg:schema": vaultItemSchema, "name": name}
	if _, err := svc.CreateOrReplaceItem(collection, "CmdWarden secret: "+name, attrs, []byte(value)); err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: saving secret %q: %w", name, err))
	}
	return nil
}

// ReleaseSecret returns the value of a previously saved secret. Per ticket
// #5, the agent's job ends at handing the value back once — it is the
// CLI's job (see internal/cliapp's `cw vault exec`) to inject it into a
// single child process's environment and never persist or print it.
func (a *Agent) ReleaseSecret(name string) (string, *dbus.Error) {
	svc, err := a.vault()
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	matches, err := findVaultMatches(svc, name)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	if len(matches) == 0 {
		return "", dbus.MakeFailedError(fmt.Errorf("agentd: no vault secret named %q", name))
	}
	value, err := svc.GetSecretValue(matches[0])
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("agentd: releasing secret %q: %w", name, err))
	}
	return string(value), nil
}

// DeleteSecret removes a previously saved secret — every matching item,
// service-wide (see SaveSecret's doc for why more than one can exist).
// Deleting a name that doesn't exist is not an error — the end state (no
// such secret) is already true.
func (a *Agent) DeleteSecret(name string) *dbus.Error {
	svc, err := a.vault()
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	if err := deleteAllVaultMatches(svc, name); err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: deleting secret %q: %w", name, err))
	}
	return nil
}

// ImportGHToken finds gh's currently active OAuth token for hostname (env
// var, then hosts.yml plaintext, then gh's own Secret Service keyring entry
// — gh's own real resolution order) and imports it into a CmdWarden vault
// entry named "gh:<hostname>". It returns a human-readable, secret-free
// description of where the token came from.
func (a *Agent) ImportGHToken(hostname string) (string, *dbus.Error) {
	token, source, err := ghauth.TokenFromEnvOrConfig(hostname)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("agentd: reading gh's own config: %w", err))
	}

	if token == "" {
		svc, svcErr := a.vault()
		if svcErr != nil {
			return "", dbus.MakeFailedError(svcErr)
		}
		item, activeUser, findErr := findGHKeyringItem(svc, hostname)
		if findErr != nil {
			return "", dbus.MakeFailedError(fmt.Errorf("agentd: searching gh's keyring entry: %w", findErr))
		}
		if item == "" {
			return "", dbus.MakeFailedError(fmt.Errorf(
				"agentd: no active gh token found for %q via env vars, hosts.yml, or the %q keyring entry — is gh logged in?",
				hostname, ghauth.KeyringServiceName(hostname)))
		}
		value, getErr := svc.GetSecretValue(item)
		if getErr != nil {
			return "", dbus.MakeFailedError(fmt.Errorf("agentd: reading gh's keyring entry: %w", getErr))
		}
		token = string(value)
		source = "keyring (" + ghauth.KeyringServiceName(hostname) + ", user " + activeUser + ")"
	}

	name := contracts.GHVaultSecretName(hostname)
	if err := a.SaveSecret(name, token); err != nil {
		return "", err
	}
	return source, nil
}

// findGHKeyringItem searches for gh's own Secret Service entry for
// hostname, preferring one whose "username" attribute matches hosts.yml's
// recorded active user when there's more than one candidate (see the
// research doc: a stale entry with an empty/different username can coexist
// with the real one).
func findGHKeyringItem(svc *secretservice.Client, hostname string) (item dbus.ObjectPath, username string, err error) {
	matches, err := svc.Search(map[string]string{"service": ghauth.KeyringServiceName(hostname)})
	if err != nil {
		return "", "", err
	}
	if len(matches) == 0 {
		return "", "", nil
	}

	activeUser, _ := ghauth.ActiveUser(hostname) // best-effort; "" is fine

	var fallback dbus.ObjectPath
	var fallbackUser string
	for _, m := range matches {
		attrs, attrErr := svc.ItemAttributes(m)
		if attrErr != nil {
			continue
		}
		user := attrs["username"]
		if activeUser != "" && user == activeUser {
			return m, user, nil
		}
		if fallback == "" || (fallbackUser == "" && user != "") {
			fallback = m
			fallbackUser = user
		}
	}
	return fallback, fallbackUser, nil
}

// findVaultMatches looks up every one of CmdWarden's own vault entries
// currently matching name, service-wide (across every collection).
func findVaultMatches(svc *secretservice.Client, name string) ([]dbus.ObjectPath, error) {
	matches, err := svc.Search(map[string]string{"xdg:schema": vaultItemSchema, "name": name})
	if err != nil {
		return nil, fmt.Errorf("agentd: searching vault: %w", err)
	}
	return matches, nil
}

// deleteAllVaultMatches deletes every existing item matching name. No
// matches is not an error.
func deleteAllVaultMatches(svc *secretservice.Client, name string) error {
	matches, err := findVaultMatches(svc, name)
	if err != nil {
		return err
	}
	for _, item := range matches {
		if err := svc.DeleteItem(item); err != nil {
			return err
		}
	}
	return nil
}
