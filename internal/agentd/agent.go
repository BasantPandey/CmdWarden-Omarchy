// Package agentd is the cmdwarden-agent daemon implementation: transport and
// lifecycle only (systemd socket activation + D-Bus session-bus
// registration). Policy, vault, identity, and gate logic are exported here
// by later tickets as additional D-Bus methods on *Agent; this file (ticket
// #2) only proves the process starts, is reachable, and stops cleanly.
package agentd

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/secretservice"
)

// Agent is the D-Bus-exported object at contracts.DBusObjectPath. Its
// methods must match the signature godbus requires for exported methods:
// ordinary Go types in, ending in a trailing *dbus.Error.
type Agent struct {
	stopOnce sync.Once
	stopCh   chan struct{}

	conn      *dbus.Conn
	secretSvc *secretservice.Client

	gatesMu sync.Mutex
	gates   map[string]*pendingGate
}

// vault returns the agent's Secret Service session, or an error if it
// hasn't been established (it's opened once in Run, right after the D-Bus
// session bus connection itself — see there).
func (a *Agent) vault() (*secretservice.Client, error) {
	if a.secretSvc == nil {
		return nil, fmt.Errorf("agentd: Secret Service session not available")
	}
	return a.secretSvc, nil
}

// Ping answers the Session Agent's health check. No side effects.
func (a *Agent) Ping() (string, *dbus.Error) {
	return "pong", nil
}

// Stop asks the agent to shut down. It replies success immediately and
// tears the process down shortly after, giving godbus time to flush the
// method reply before the connection closes.
func (a *Agent) Stop() *dbus.Error {
	go func() {
		time.Sleep(150 * time.Millisecond)
		a.requestStop()
	}()
	return nil
}

func (a *Agent) requestStop() {
	a.stopOnce.Do(func() { close(a.stopCh) })
}

// Run starts the agent and blocks until ctx is cancelled or a Stop request
// is served, then shuts down cleanly (releases the D-Bus name, closes the
// activation socket listener).
func Run(ctx context.Context) error {
	sockListener, err := socketListener()
	if err != nil {
		return err
	}
	defer sockListener.Close()

	agent := &Agent{stopCh: make(chan struct{})}
	go serveSocket(sockListener, agent)

	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("agentd: connecting to session bus: %w", err)
	}
	defer conn.Close()

	reply, err := conn.RequestName(contracts.DBusServiceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("agentd: requesting D-Bus name %s: %w", contracts.DBusServiceName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("agentd: another process already owns %s (is cmdwarden-agent already running?)", contracts.DBusServiceName)
	}
	defer conn.ReleaseName(contracts.DBusServiceName)
	agent.conn = conn

	secretSvc, err := secretservice.Open(conn)
	if err != nil {
		return fmt.Errorf("agentd: opening Secret Service session: %w", err)
	}
	agent.secretSvc = secretSvc

	if err := conn.Export(agent, contracts.DBusObjectPath, contracts.DBusInterface); err != nil {
		return fmt.Errorf("agentd: exporting object: %w", err)
	}

	if ok, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		log.Printf("cmdwarden-agent: sd_notify(READY) failed: %v", err)
	} else if !ok {
		log.Printf("cmdwarden-agent: sd_notify not available (not run under systemd?) — continuing")
	}
	log.Printf("cmdwarden-agent: healthy, owns %s at %s", contracts.DBusServiceName, contracts.DBusObjectPath)

	select {
	case <-ctx.Done():
		log.Printf("cmdwarden-agent: shutting down (%v)", ctx.Err())
	case <-agent.stopCh:
		log.Printf("cmdwarden-agent: shutting down (stop requested)")
	}
	return nil
}
