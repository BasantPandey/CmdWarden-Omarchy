package agentclient

import (
	"context"
	"fmt"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// ResolveIdentity asks the agent to resolve the calling cw process's own
// Identity Key. The agent determines "calling process" from the D-Bus
// connection itself (see agentd.ResolveIdentity) — cw cannot influence the
// answer beyond however it happens to have been launched.
func ResolveIdentity(ctx context.Context, timeout time.Duration) (contracts.IdentityKey, error) {
	conn, call, err := dial(ctx, timeout, "ResolveIdentity")
	if err != nil {
		return contracts.IdentityKey{}, err
	}
	defer conn.Close()

	var channel, tool, path, hash string
	if err := call.Store(&channel, &tool, &path, &hash); err != nil {
		return contracts.IdentityKey{}, fmt.Errorf("agentclient: decoding ResolveIdentity reply: %w", err)
	}
	return contracts.IdentityKey{
		Channel: contracts.Channel(channel),
		Tool:    tool,
		Path:    path,
		Hash:    hash,
	}, nil
}
