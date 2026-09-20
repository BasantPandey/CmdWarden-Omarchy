package agentd

import (
	"bufio"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func newTestAgent(t *testing.T) (*Agent, net.Listener) {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listening on test socket: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	agent := &Agent{stopCh: make(chan struct{})}
	go serveSocket(l, agent)
	return agent, l
}

func dialAndRoundTrip(t *testing.T, l net.Listener, line string) string {
	t.Helper()
	conn, err := net.Dial("unix", l.Addr().String())
	if err != nil {
		t.Fatalf("dialing test socket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("writing %q: %v", line, err)
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("reading reply to %q: %v", line, err)
	}
	return reply
}

func TestSocketPing(t *testing.T) {
	_, l := newTestAgent(t)

	if got := dialAndRoundTrip(t, l, "PING"); got != "PONG\n" {
		t.Errorf("PING reply = %q, want %q", got, "PONG\n")
	}
}

func TestSocketStopTriggersShutdown(t *testing.T) {
	agent, l := newTestAgent(t)

	if got := dialAndRoundTrip(t, l, "STOP"); got != "OK\n" {
		t.Errorf("STOP reply = %q, want %q", got, "OK\n")
	}

	select {
	case <-agent.stopCh:
	case <-time.After(2 * time.Second):
		t.Fatal("STOP did not close stopCh")
	}
}

func TestSocketUnknownCommand(t *testing.T) {
	_, l := newTestAgent(t)

	got := dialAndRoundTrip(t, l, "WHATEVER")
	if got == "PONG\n" || got == "OK\n" {
		t.Errorf("unknown command got a recognized reply: %q", got)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	agent := &Agent{stopCh: make(chan struct{})}
	agent.requestStop()
	agent.requestStop() // must not panic on double-close
	<-agent.stopCh
}
