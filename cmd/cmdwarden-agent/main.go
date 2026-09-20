// Command cmdwarden-agent is the CmdWarden-Omarchy Session Agent daemon:
// socket-activated by systemd --user, reachable over the D-Bus session bus.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/agentd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agentd.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "cmdwarden-agent:", err)
		os.Exit(1)
	}
}
