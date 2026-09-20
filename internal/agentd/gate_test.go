package agentd

import (
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func TestRunGateFailsClosedWithNoWaylandDisplay(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	// Deliberately leave PATH alone — this test is specifically about the
	// no-display check running (and short-circuiting) before qs is even
	// looked up.

	agent := &Agent{stopCh: make(chan struct{})}
	got := agent.runGate("mise:claude", "gh", "gh pr create", "write", "Read", true)
	if got != contracts.DecisionUnavailable {
		t.Errorf("runGate with no WAYLAND_DISPLAY = %q, want %q", got, contracts.DecisionUnavailable)
	}
}

func TestRunGateFailsClosedWithNoQSBinary(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0") // pass the display check
	t.Setenv("PATH", t.TempDir())            // guarantee qs isn't found

	agent := &Agent{stopCh: make(chan struct{})}
	got := agent.runGate("mise:claude", "gh", "gh pr create", "write", "Read", true)
	if got != contracts.DecisionUnavailable {
		t.Errorf("runGate with no qs on PATH = %q, want %q", got, contracts.DecisionUnavailable)
	}
}

func TestSubmitGateDecisionRejectsUnknownRequest(t *testing.T) {
	agent := &Agent{stopCh: make(chan struct{})}
	if err := agent.SubmitGateDecision("no-such-request", "deny"); err == nil {
		t.Error("expected an error for an unknown gate request id")
	}
}

func TestSubmitGateDecisionRejectsUnrecognizedDecision(t *testing.T) {
	agent := &Agent{stopCh: make(chan struct{})}
	pending := &pendingGate{resultCh: make(chan contracts.Decision, 1)}
	agent.gatesMu.Lock()
	agent.pendingGates()["req-1"] = pending
	agent.gatesMu.Unlock()

	if err := agent.SubmitGateDecision("req-1", "maybe"); err == nil {
		t.Error("expected an error for an unrecognized decision string")
	}
}

func TestSubmitGateDecisionDeliversToWaiter(t *testing.T) {
	agent := &Agent{stopCh: make(chan struct{})}
	pending := &pendingGate{resultCh: make(chan contracts.Decision, 1)}
	agent.gatesMu.Lock()
	agent.pendingGates()["req-1"] = pending
	agent.gatesMu.Unlock()

	if err := agent.SubmitGateDecision("req-1", "allow-once"); err != nil {
		t.Fatalf("SubmitGateDecision failed: %v", err)
	}

	select {
	case got := <-pending.resultCh:
		if got != contracts.DecisionAllowOnce {
			t.Errorf("delivered decision = %q, want %q", got, contracts.DecisionAllowOnce)
		}
	default:
		t.Fatal("expected a decision to be waiting on resultCh")
	}
}
