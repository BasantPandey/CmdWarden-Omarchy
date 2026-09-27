package ghclassify

import (
	"strings"
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/policy"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		cmd   string
		class contracts.CommandClass
	}{
		// secret-reveal: token-export commands
		{"auth token", contracts.ClassSecretReveal},
		{"auth status", contracts.ClassSecretReveal},
		{"auth status --show-token", contracts.ClassSecretReveal},
		{"auth git-credential get", contracts.ClassSecretReveal},
		{"api repos/owner/repo", contracts.ClassSecretReveal},
		{"run view --log", contracts.ClassSecretReveal},
		{"run view --log-failed", contracts.ClassSecretReveal},
		{"codespace ports forward", contracts.ClassSecretReveal},

		// write: side-effecting or auth-state-changing commands
		{"pr create --title x --body y", contracts.ClassWrite},
		{"repo create foo", contracts.ClassWrite},
		{"auth login", contracts.ClassWrite},
		{"auth refresh", contracts.ClassWrite},
		{"auth logout", contracts.ClassWrite},
		{"auth switch", contracts.ClassWrite},
		{"auth git-credential store", contracts.ClassWrite},
		{"secret set DEPLOY_TOKEN", contracts.ClassWrite},

		// read: read-mostly commands
		{"repo view BasantPandey/CmdWarden --json defaultBranchRef", contracts.ClassRead},
		{"pr list", contracts.ClassRead},
		{"issue list", contracts.ClassRead},
		{"run view", contracts.ClassRead},
		{"run view 12345", contracts.ClassRead},
		{"codespace ports", contracts.ClassRead},
		{"search prs --author=x", contracts.ClassRead},

		// unknown: unmatched subcommand must not silently become read
		{"totally-made-up-subcommand", contracts.ClassUnknown},
		{"pr not-a-real-verb", contracts.ClassUnknown},
		{"", contracts.ClassUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			var args []string
			if tc.cmd != "" {
				args = strings.Fields(tc.cmd)
			}
			got := Classify(args)
			if got != tc.class {
				t.Errorf("Classify(%q) = %q, want %q", tc.cmd, got, tc.class)
			}
		})
	}
}

// TestGateCommandsClassAndPolicy classifies the gh invocations the gate
// cares about and asks the existing policy decision what Read, Trusted,
// Deny, and Full do with that class. It does not add a matrix cell.
func TestGateCommandsClassAndPolicy(t *testing.T) {
	cases := []struct {
		cmd   string
		class contracts.CommandClass
	}{
		{"auth status", contracts.ClassSecretReveal},
		{"auth status --show-token", contracts.ClassSecretReveal},
		{"run view", contracts.ClassRead},
		{"run view --log", contracts.ClassSecretReveal},
		{"run view --log-failed", contracts.ClassSecretReveal},
		{"codespace ports", contracts.ClassRead},
		{"codespace ports forward", contracts.ClassSecretReveal},
		{"auth token", contracts.ClassSecretReveal},
		{"auth git-credential get", contracts.ClassSecretReveal},
		{"api repos/owner/repo", contracts.ClassSecretReveal},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			class := Classify(strings.Fields(tc.cmd))
			if class != tc.class {
				t.Fatalf("Classify(%q) = %q, want %q", tc.cmd, class, tc.class)
			}

			var want map[contracts.PolicyLevel]contracts.PolicyOutcome
			if class == contracts.ClassRead {
				want = map[contracts.PolicyLevel]contracts.PolicyOutcome{
					contracts.LevelRead:    contracts.OutcomeAutoAllow,
					contracts.LevelTrusted: contracts.OutcomeAutoAllow,
					contracts.LevelDeny:    contracts.OutcomeDeny,
					contracts.LevelFull:    contracts.OutcomeAutoAllow,
				}
			} else {
				want = map[contracts.PolicyLevel]contracts.PolicyOutcome{
					contracts.LevelRead:    contracts.OutcomePrompt,
					contracts.LevelTrusted: contracts.OutcomePrompt,
					contracts.LevelDeny:    contracts.OutcomeDeny,
					contracts.LevelFull:    contracts.OutcomeAutoAllow,
				}
			}

			for level, outcome := range want {
				got := policy.Decide(level, class)
				if got != outcome {
					t.Errorf("Decide(%s, %s) = %s, want %s", level, class, got, outcome)
				}
			}
		})
	}
}

func TestClassifyIsPureAndIndependent(t *testing.T) {
	// Calling Classify repeatedly with the same input must be
	// deterministic and side-effect free — no agent, vault, or shim
	// involved.
	for i := 0; i < 3; i++ {
		if got := Classify([]string{"pr", "create"}); got != contracts.ClassWrite {
			t.Fatalf("Classify not deterministic on call %d: got %q", i, got)
		}
	}
}
