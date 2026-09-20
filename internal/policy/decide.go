// Package policy implements the auto-allow matrix (level x class) and the
// Launcher enrollment store it's evaluated against, per the Windows
// CmdWarden policy spec
// (https://github.com/BasantPandey/CmdWarden/blob/main/docs/spec/cmdwarden.md#6-policy-and-enrollment).
//
// This ticket (#4) proves the decision logic only: Decide and the
// enrollment Store are pure/local, with no wiring yet to a real gated tool
// invocation (that's #6/#7/#8/#11) or to the Approval Gate (#6).
package policy

import "github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"

// matrix is the auto-allow table for non-Deny levels: matrix[level][class]
// is true when that combination is auto-allowed. Deny is handled specially
// in Decide (it never auto-allows and never prompts — see the package doc).
var matrix = map[contracts.PolicyLevel]map[contracts.CommandClass]bool{
	contracts.LevelRead: {
		contracts.ClassRead: true,
	},
	contracts.LevelTrusted: {
		contracts.ClassRead:  true,
		contracts.ClassWrite: true,
	},
	contracts.LevelFull: {
		contracts.ClassRead:         true,
		contracts.ClassWrite:        true,
		contracts.ClassSecretReveal: true,
		contracts.ClassUnknown:      true,
	},
}

// Decide returns the policy pre-check outcome for a Launcher at level
// attempting a command of the given class.
//
//   - level == LevelDeny (including an unenrolled Launcher, which callers
//     should treat as LevelDeny — see Store.Resolve): always OutcomeDeny,
//     regardless of class. A Deny policy level means "never even ask."
//   - Otherwise, an auto-allowed cell in the matrix -> OutcomeAutoAllow.
//   - Otherwise (not auto-allowed, but not Deny either) -> OutcomePrompt:
//     not auto-allowed, but worth asking a human via the Approval Gate.
func Decide(level contracts.PolicyLevel, class contracts.CommandClass) contracts.PolicyOutcome {
	if level == contracts.LevelDeny {
		return contracts.OutcomeDeny
	}
	if matrix[level][class] {
		return contracts.OutcomeAutoAllow
	}
	return contracts.OutcomePrompt
}
