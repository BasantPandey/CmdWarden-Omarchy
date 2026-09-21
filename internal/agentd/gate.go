// Approval Gate wiring (ticket #6): the agent owns pending gate requests,
// spawns a real Quickshell instance (embedded QML, see qml/shell.qml) to
// show one, and blocks the caller until a human answers or the gate fails
// closed. No policy/audit/session-tracking integration yet — that's #11.
package agentd

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/selfpath"
)

//go:embed qml/shell.qml
var gateQML embed.FS

// gateTimeout bounds how long a single Approval Gate request waits for a
// human before failing closed — "cannot show / timeout -> Unavailable ->
// fail closed," per the Windows CmdWarden Approval Gate spec.
const gateTimeout = 5 * time.Minute

type pendingGate struct {
	resultCh chan contracts.Decision
}

func (a *Agent) pendingGates() map[string]*pendingGate {
	if a.gates == nil {
		a.gates = make(map[string]*pendingGate)
	}
	return a.gates
}

// RequestGate pops the real Approval Gate UI populated with the given
// (already-resolved) data and blocks until a human answers, the process
// exits without answering, or gateTimeout elapses — every one of those
// paths returns a decision, never an error, because "can't show the gate"
// is itself a decision (Unavailable) under fail-closed semantics.
//
// Before popping anything, it checks whether the real caller (resolved from
// sender, never trusted from identityKey) already holds a live "Allow for
// Session" grant covering this tool/class — if so, it returns
// DecisionSessionAllow immediately, with no UI at all. A fresh
// DecisionSessionGrant answer from the UI is recorded as a new grant for
// that same caller before returning.
func (a *Agent) RequestGate(identityKey, tool, commandLine, commandClassStr, policyLevel string, offerAllowSession bool, sender dbus.Sender) (string, *dbus.Error) {
	class := contracts.CommandClass(commandClassStr)

	launcher, launcherErr := a.resolveLauncher(sender)
	if launcherErr != nil {
		log.Printf("cmdwarden-agent: could not resolve caller for session-grant tracking, skipping it: %v", launcherErr)
	} else if a.checkSessionGrant(launcher, tool, class) {
		return string(contracts.DecisionSessionAllow), nil
	}

	decision := a.runGate(identityKey, tool, commandLine, commandClassStr, policyLevel, offerAllowSession)
	if decision == contracts.DecisionSessionGrant && launcherErr == nil {
		a.recordSessionGrant(launcher, tool, class)
	}
	return string(decision), nil
}

func (a *Agent) runGate(identityKey, tool, commandLine, commandClassStr, policyLevel string, offerAllowSession bool) contracts.Decision {
	qsPath, err := exec.LookPath("qs")
	if err != nil {
		log.Printf("cmdwarden-agent: gate unavailable: qs not found on PATH: %v", err)
		return contracts.DecisionUnavailable
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		log.Printf("cmdwarden-agent: gate unavailable: no WAYLAND_DISPLAY in agent environment")
		return contracts.DecisionUnavailable
	}
	cwPath := resolveCWBinary()

	qmlDir, cleanup, err := extractGateQML()
	if err != nil {
		log.Printf("cmdwarden-agent: gate unavailable: %v", err)
		return contracts.DecisionUnavailable
	}
	defer cleanup()

	requestID, err := randomRequestID()
	if err != nil {
		log.Printf("cmdwarden-agent: gate unavailable: generating request id: %v", err)
		return contracts.DecisionUnavailable
	}

	pending := &pendingGate{resultCh: make(chan contracts.Decision, 1)}
	a.gatesMu.Lock()
	a.pendingGates()[requestID] = pending
	a.gatesMu.Unlock()
	defer func() {
		a.gatesMu.Lock()
		delete(a.pendingGates(), requestID)
		a.gatesMu.Unlock()
	}()

	sessionFlag := "0"
	if offerAllowSession {
		sessionFlag = "1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), gateTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, qsPath, "-p", qmlDir)
	cmd.Env = append(os.Environ(),
		"CMDWARDEN_GATE_REQUEST_ID="+requestID,
		"CMDWARDEN_GATE_IDENTITY="+identityKey,
		"CMDWARDEN_GATE_TOOL="+tool,
		"CMDWARDEN_GATE_COMMAND="+commandLine,
		"CMDWARDEN_GATE_CLASS="+commandClassStr,
		"CMDWARDEN_GATE_POLICY_LEVEL="+policyLevel,
		"CMDWARDEN_GATE_ALLOW_SESSION="+sessionFlag,
		"CMDWARDEN_CW_PATH="+cwPath,
	)

	if err := cmd.Start(); err != nil {
		log.Printf("cmdwarden-agent: gate unavailable: starting qs: %v", err)
		return contracts.DecisionUnavailable
	}

	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	select {
	case decision := <-pending.resultCh:
		return decision
	case <-exited:
		// The window closed (or qs crashed) without ever calling `cw gate
		// respond`. Drain resultCh once more in case respond() raced the
		// process exit (respond -> Process exits synchronously -> Qt.quit
		// all happen in close succession).
		select {
		case decision := <-pending.resultCh:
			return decision
		default:
			log.Printf("cmdwarden-agent: gate %s: qs exited without a decision — failing closed", requestID)
			return contracts.DecisionUnavailable
		}
	case <-ctx.Done():
		log.Printf("cmdwarden-agent: gate %s: timed out after %s — failing closed", requestID, gateTimeout)
		_ = cmd.Process.Kill()
		return contracts.DecisionUnavailable
	}
}

// SubmitGateDecision is called by `cw gate respond`, itself invoked by the
// QML's button handlers — never directly by an end user.
func (a *Agent) SubmitGateDecision(requestID string, decisionStr string) *dbus.Error {
	var decision contracts.Decision
	switch decisionStr {
	case "deny":
		decision = contracts.DecisionDeny
	case "allow-once":
		decision = contracts.DecisionAllowOnce
	case "allow-session":
		decision = contracts.DecisionSessionGrant
	default:
		return dbus.MakeFailedError(fmt.Errorf("agentd: unrecognized gate decision %q", decisionStr))
	}

	a.gatesMu.Lock()
	pending, ok := a.pendingGates()[requestID]
	a.gatesMu.Unlock()
	if !ok {
		return dbus.MakeFailedError(fmt.Errorf("agentd: unknown or expired gate request %q", requestID))
	}

	select {
	case pending.resultCh <- decision:
	default:
		// Already answered (e.g. a double-click); the first answer wins.
	}
	return nil
}

func randomRequestID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// extractGateQML writes the embedded shell.qml into a fresh temp directory
// so `qs -p <dir>` has a real filesystem path to run, regardless of where
// cmdwarden-agent itself is installed.
func extractGateQML() (dir string, cleanup func(), err error) {
	content, err := gateQML.ReadFile("qml/shell.qml")
	if err != nil {
		return "", nil, fmt.Errorf("agentd: reading embedded gate QML: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "cmdwarden-gate-*")
	if err != nil {
		return "", nil, fmt.Errorf("agentd: creating temp dir for gate QML: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "shell.qml"), content, 0o644); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("agentd: writing gate QML: %w", err)
	}
	return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
}

// resolveCWBinary finds the cw binary to hand the QML for its `cw gate
// respond` callback.
func resolveCWBinary() string {
	if path, err := selfpath.Sibling("cw"); err == nil {
		return path
	}
	return "cw" // best-effort fallback; PATH resolution inside qs's own env
}
