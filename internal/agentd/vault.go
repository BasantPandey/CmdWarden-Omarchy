package agentd

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/ghauth"
)

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
	if err := svc.SaveNamed(name, value); err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: %w", err))
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
	value, ok, err := svc.LookupNamed(name)
	if err != nil {
		return "", dbus.MakeFailedError(fmt.Errorf("agentd: %w", err))
	}
	if !ok {
		return "", dbus.MakeFailedError(fmt.Errorf("agentd: no vault secret named %q", name))
	}
	return value, nil
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
	if err := svc.DeleteNamed(name); err != nil {
		return dbus.MakeFailedError(fmt.Errorf("agentd: deleting secret %q: %w", name, err))
	}
	return nil
}

// ImportGHToken finds gh's currently active OAuth token for hostname and
// imports it into a CmdWarden vault entry named "gh:<hostname>". It returns
// a human-readable, secret-free description of where the token came from.
//
// A missing, locked, or wrong-wallet Secret Service, and a token whose only
// active source is plaintext hosts.yml, fail here — before the shim
// installer runs. hosts.yml is not written. The source label is returned
// only after the vault write succeeds.
func (a *Agent) ImportGHToken(hostname string) (string, *dbus.Error) {
	kr := a.tokenKeyring
	if kr == nil && a.secretSvc != nil {
		kr = ghauth.NewLiveKeyring(a.secretSvc)
	}
	source, err := ghauth.Import(hostname, kr)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return source, nil
}
