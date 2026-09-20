package agentclient

import (
	"context"
	"fmt"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// RequestGate asks the agent to pop the real Approval Gate and blocks until
// a human answers (or the gate fails closed). timeout here is only for the
// D-Bus call machinery, not the human — pass a timeout comfortably longer
// than the agent's own gateTimeout.
func RequestGate(ctx context.Context, timeout time.Duration, identityKey, tool, commandLine string, class contracts.CommandClass, policyLevel string, offerAllowSession bool) (contracts.Decision, error) {
	conn, call, err := dial(ctx, timeout, "RequestGate", identityKey, tool, commandLine, string(class), policyLevel, offerAllowSession)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var decision string
	if err := call.Store(&decision); err != nil {
		return "", fmt.Errorf("agentclient: decoding RequestGate reply: %w", err)
	}
	return contracts.Decision(decision), nil
}

// SubmitGateDecision reports a human's Approval Gate answer back to the
// agent for the given pending request. Used by `cw gate respond`, which the
// gate UI itself invokes — never called directly by a person.
func SubmitGateDecision(ctx context.Context, timeout time.Duration, requestID, decision string) error {
	return callVoid(ctx, timeout, "SubmitGateDecision", requestID, decision)
}
