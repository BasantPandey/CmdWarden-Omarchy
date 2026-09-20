// Package agentclient is the cw CLI's view of the Session Agent: talking to
// it over the D-Bus session bus, and waking it via its systemd activation
// socket when nothing currently owns its D-Bus name. It has no dependency
// on internal/agentd — the CLI never imports the agent's own implementation.
package agentclient

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

// ErrNotRunning indicates the agent's D-Bus name currently has no owner.
var ErrNotRunning = fmt.Errorf("cmdwarden-agent is not running")

// Ping calls the agent's Ping method over the D-Bus session bus with the
// given timeout. It returns ErrNotRunning (not a generic error) when the
// name simply has no owner yet, so callers can distinguish "not started"
// from "actually broken."
func Ping(ctx context.Context, timeout time.Duration) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("agentclient: connecting to session bus: %w", err)
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	obj := conn.Object(contracts.DBusServiceName, dbus.ObjectPath(contracts.DBusObjectPath))
	var pong string
	call := obj.CallWithContext(callCtx, contracts.DBusInterface+".Ping", 0)
	if call.Err != nil {
		if isNoOwnerError(call.Err) {
			return ErrNotRunning
		}
		return fmt.Errorf("agentclient: Ping: %w", call.Err)
	}
	if err := call.Store(&pong); err != nil {
		return fmt.Errorf("agentclient: decoding Ping reply: %w", err)
	}
	if pong != "pong" {
		return fmt.Errorf("agentclient: unexpected Ping reply %q", pong)
	}
	return nil
}

// Stop calls the agent's Stop method over the D-Bus session bus.
func Stop(ctx context.Context, timeout time.Duration) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("agentclient: connecting to session bus: %w", err)
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	obj := conn.Object(contracts.DBusServiceName, dbus.ObjectPath(contracts.DBusObjectPath))
	call := obj.CallWithContext(callCtx, contracts.DBusInterface+".Stop", 0)
	if call.Err != nil {
		if isNoOwnerError(call.Err) {
			return ErrNotRunning
		}
		return fmt.Errorf("agentclient: Stop: %w", call.Err)
	}
	return nil
}

func isNoOwnerError(err error) bool {
	dbusErr, ok := err.(dbus.Error)
	if !ok {
		return false
	}
	switch dbusErr.Name {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
		return true
	}
	return false
}

// WakeSocket triggers systemd socket activation by connecting to the
// agent's activation socket and asking it to PING. Unlike Ping, this does
// not go over the D-Bus session bus at all — a bare connect() to the
// listening socket is enough for systemd to start cmdwarden-agent.service
// if it isn't running yet; this then waits for the freshly started process
// to answer over that same socket, which is available before the process
// has necessarily finished registering its D-Bus name.
//
// It returns an error naming the socket path if nothing is listening there
// at all (most likely: the unit was never installed — see cw agent
// install).
func WakeSocket(timeout time.Duration) error {
	sockPath, err := xdgpaths.RuntimeSocketPath(contracts.SocketFileName)
	if err != nil {
		return fmt.Errorf("agentclient: %w", err)
	}

	conn, err := net.DialTimeout("unix", sockPath, timeout)
	if err != nil {
		return fmt.Errorf("agentclient: connecting to activation socket %s (is %s installed and enabled? try `cw agent install`): %w", sockPath, contracts.SocketUnitName, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte("PING\n")); err != nil {
		return fmt.Errorf("agentclient: writing to activation socket: %w", err)
	}

	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("agentclient: reading activation socket reply: %w", err)
	}
	if strings.TrimSpace(reply) != "PONG" {
		return fmt.Errorf("agentclient: unexpected activation socket reply %q", reply)
	}
	return nil
}

// WaitHealthy triggers lazy-start via the activation socket, then polls the
// D-Bus name until Ping succeeds or the overall deadline elapses. This is
// what `cw doctor` uses: from a shell where the agent isn't running yet,
// this call alone brings it up and confirms it's healthy.
func WaitHealthy(ctx context.Context, overall time.Duration) error {
	deadline := time.Now().Add(overall)

	if err := Ping(ctx, 500*time.Millisecond); err == nil {
		return nil
	} else if err != ErrNotRunning {
		return err
	}

	if err := WakeSocket(overall); err != nil {
		return err
	}

	// Once the activation socket has answered, the agent process exists
	// but may still be mid-startup: it can briefly own its D-Bus name
	// before Export has registered the interface, which surfaces as a
	// transient (non-ErrNotRunning) error from Ping. Retry any error here
	// until the deadline rather than just ErrNotRunning, and report the
	// last-seen error if it never recovers.
	backoff := 100 * time.Millisecond
	var lastErr error
	for {
		err := Ping(ctx, 1*time.Second)
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().Add(backoff).After(deadline) {
			return fmt.Errorf("agentclient: agent did not become healthy within %s (last error: %v)", overall, lastErr)
		}
		time.Sleep(backoff)
		if backoff < time.Second {
			backoff *= 2
		}
	}
}
