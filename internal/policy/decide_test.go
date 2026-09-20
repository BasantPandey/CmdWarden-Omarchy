package policy

import (
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

// TestDecideMatrix covers every level x class combination in the Windows
// CmdWarden policy matrix, per ticket #4's acceptance criterion.
func TestDecideMatrix(t *testing.T) {
	classes := []contracts.CommandClass{
		contracts.ClassRead, contracts.ClassWrite, contracts.ClassSecretReveal, contracts.ClassUnknown,
	}

	cases := []struct {
		level contracts.PolicyLevel
		want  map[contracts.CommandClass]contracts.PolicyOutcome
	}{
		{
			level: contracts.LevelDeny,
			want: map[contracts.CommandClass]contracts.PolicyOutcome{
				contracts.ClassRead: contracts.OutcomeDeny, contracts.ClassWrite: contracts.OutcomeDeny,
				contracts.ClassSecretReveal: contracts.OutcomeDeny, contracts.ClassUnknown: contracts.OutcomeDeny,
			},
		},
		{
			level: contracts.LevelRead,
			want: map[contracts.CommandClass]contracts.PolicyOutcome{
				contracts.ClassRead: contracts.OutcomeAutoAllow, contracts.ClassWrite: contracts.OutcomePrompt,
				contracts.ClassSecretReveal: contracts.OutcomePrompt, contracts.ClassUnknown: contracts.OutcomePrompt,
			},
		},
		{
			level: contracts.LevelTrusted,
			want: map[contracts.CommandClass]contracts.PolicyOutcome{
				contracts.ClassRead: contracts.OutcomeAutoAllow, contracts.ClassWrite: contracts.OutcomeAutoAllow,
				contracts.ClassSecretReveal: contracts.OutcomePrompt, contracts.ClassUnknown: contracts.OutcomePrompt,
			},
		},
		{
			level: contracts.LevelFull,
			want: map[contracts.CommandClass]contracts.PolicyOutcome{
				contracts.ClassRead: contracts.OutcomeAutoAllow, contracts.ClassWrite: contracts.OutcomeAutoAllow,
				contracts.ClassSecretReveal: contracts.OutcomeAutoAllow, contracts.ClassUnknown: contracts.OutcomeAutoAllow,
			},
		},
	}

	for _, tc := range cases {
		for _, class := range classes {
			t.Run(string(tc.level)+"/"+string(class), func(t *testing.T) {
				got := Decide(tc.level, class)
				if got != tc.want[class] {
					t.Errorf("Decide(%s, %s) = %s, want %s", tc.level, class, got, tc.want[class])
				}
			})
		}
	}
}
