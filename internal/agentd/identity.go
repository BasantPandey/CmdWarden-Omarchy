package agentd

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/identity"
)

// ResolveIdentity resolves the Identity Key of whatever process is actually
// calling this D-Bus method. It never trusts a self-reported PID from the
// caller: dbus.Sender is populated by godbus from the message's real
// sender, and callerPID asks the bus daemon itself (not the caller) which
// process that sender's connection belongs to. A lying client cannot spoof
// this the way it could if the CLI resolved its own identity and merely
// told the agent the answer.
func (a *Agent) ResolveIdentity(sender dbus.Sender) (channel string, tool string, path string, hash string, dbusErr *dbus.Error) {
	pid, err := a.callerPID(sender)
	if err != nil {
		return "", "", "", "", dbus.MakeFailedError(err)
	}
	key, err := identity.Resolve(pid)
	if err != nil {
		return "", "", "", "", dbus.MakeFailedError(err)
	}
	return string(key.Channel), key.Tool, key.Path, key.Hash, nil
}

func (a *Agent) callerPID(sender dbus.Sender) (int, error) {
	if a.conn == nil {
		return 0, fmt.Errorf("agentd: no D-Bus connection available to resolve caller PID")
	}
	var pid uint32
	call := a.conn.BusObject().Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, string(sender))
	if call.Err != nil {
		return 0, fmt.Errorf("agentd: GetConnectionUnixProcessID: %w", call.Err)
	}
	if err := call.Store(&pid); err != nil {
		return 0, fmt.Errorf("agentd: decoding GetConnectionUnixProcessID reply: %w", err)
	}
	return int(pid), nil
}
