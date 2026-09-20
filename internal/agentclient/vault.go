package agentclient

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// SaveSecret asks the agent to store value under name in its vault.
func SaveSecret(ctx context.Context, timeout time.Duration, name, value string) error {
	return callVoid(ctx, timeout, "SaveSecret", name, value)
}

// DeleteSecret asks the agent to remove a previously saved secret.
func DeleteSecret(ctx context.Context, timeout time.Duration, name string) error {
	return callVoid(ctx, timeout, "DeleteSecret", name)
}

// ReleaseSecret asks the agent for a previously saved secret's value. The
// caller (see cliapp's `cw vault exec`) must inject it into exactly one
// child process's environment and never print, log, or persist it.
func ReleaseSecret(ctx context.Context, timeout time.Duration, name string) (string, error) {
	conn, call, err := dial(ctx, timeout, "ReleaseSecret", name)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var value string
	if err := call.Store(&value); err != nil {
		return "", fmt.Errorf("agentclient: decoding ReleaseSecret reply: %w", err)
	}
	return value, nil
}

// ImportGHToken asks the agent to import gh's currently active OAuth token
// for hostname into the vault. It returns a human-readable, secret-free
// description of where the token came from (e.g. "hosts.yml (plaintext)").
func ImportGHToken(ctx context.Context, timeout time.Duration, hostname string) (string, error) {
	conn, call, err := dial(ctx, timeout, "ImportGHToken", hostname)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var source string
	if err := call.Store(&source); err != nil {
		return "", fmt.Errorf("agentclient: decoding ImportGHToken reply: %w", err)
	}
	return source, nil
}

func callVoid(ctx context.Context, timeout time.Duration, method string, args ...any) error {
	conn, _, err := dial(ctx, timeout, method, args...)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

// dial connects to the agent over the session bus and makes a single method
// call, translating a no-owner error into ErrNotRunning. The returned
// *dbus.Conn must be closed by the caller once it's done with call's reply.
func dial(ctx context.Context, timeout time.Duration, method string, args ...any) (*dbus.Conn, *dbus.Call, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, nil, fmt.Errorf("agentclient: connecting to session bus: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	obj := conn.Object(contracts.DBusServiceName, dbus.ObjectPath(contracts.DBusObjectPath))
	call := obj.CallWithContext(callCtx, contracts.DBusInterface+"."+method, 0, args...)
	if call.Err != nil {
		conn.Close()
		if isNoOwnerError(call.Err) {
			return nil, nil, ErrNotRunning
		}
		return nil, nil, fmt.Errorf("agentclient: %s: %w", method, call.Err)
	}
	return conn, call, nil
}
