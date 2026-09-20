package agentd

import (
	"fmt"
	"net"
	"os"

	"github.com/coreos/go-systemd/v22/activation"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/xdgpaths"
)

// socketListener returns the activation-socket listener the agent serves
// its trivial PING/STOP protocol on.
//
// When started by systemd (LISTEN_FDS set by cmdwarden-agent.socket), this
// is the pre-bound fd systemd already holds open — the whole point of
// socket activation being that systemd, not the agent, owns the socket's
// lifetime across restarts. When run directly (dev/manual invocation, not
// under systemd), it falls back to binding the same path itself so the
// agent is still usable without the unit installed.
func socketListener() (net.Listener, error) {
	listeners, err := activation.Listeners()
	if err == nil && len(listeners) > 0 {
		return listeners[0], nil
	}

	sockPath, err := xdgpaths.RuntimeSocketPath(contracts.SocketFileName)
	if err != nil {
		return nil, fmt.Errorf("agentd: resolving socket path for manual bind: %w", err)
	}
	// Remove a stale socket left by an unclean previous exit; harmless if
	// it doesn't exist.
	_ = os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("agentd: binding %s: %w", sockPath, err)
	}
	return l, nil
}
