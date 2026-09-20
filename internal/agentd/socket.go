package agentd

import (
	"bufio"
	"log"
	"net"
	"strings"
)

// serveSocket accepts connections on the activation socket and answers a
// trivial one-line-in, one-line-out protocol: PING -> PONG, STOP -> OK (and
// triggers shutdown). This exists so the agent has a liveness/control
// surface that works even in the brief window right after systemd starts
// it, before the D-Bus session bus RequestName call has completed — and so
// systemd has something to hold a listening socket on for lazy activation
// in the first place. It carries no policy/vault/identity semantics.
func serveSocket(l net.Listener, a *Agent) {
	for {
		conn, err := l.Accept()
		if err != nil {
			// Listener closed during shutdown — not an error worth logging.
			return
		}
		go handleSocketConn(conn, a)
	}
}

func handleSocketConn(conn net.Conn, a *Agent) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	switch strings.TrimSpace(scanner.Text()) {
	case "PING":
		_, _ = conn.Write([]byte("PONG\n"))
	case "STOP":
		_, _ = conn.Write([]byte("OK\n"))
		a.requestStop()
	default:
		_, _ = conn.Write([]byte("ERR unknown command\n"))
	}
	if err := scanner.Err(); err != nil {
		log.Printf("cmdwarden-agent: socket read error: %v", err)
	}
}
