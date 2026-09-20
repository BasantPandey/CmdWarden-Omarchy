package ghclassify

import (
	"strings"
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		cmd   string
		class contracts.CommandClass
	}{
		// secret-reveal: token-export commands
		{"auth token", contracts.ClassSecretReveal},
		{"auth status --show-token", contracts.ClassSecretReveal},
		{"auth git-credential get", contracts.ClassSecretReveal},
		{"api repos/owner/repo", contracts.ClassSecretReveal},

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
		{"auth status", contracts.ClassRead},
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
