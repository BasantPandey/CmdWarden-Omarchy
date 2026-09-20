package contracts

import "testing"

func TestGHVaultSecretName(t *testing.T) {
	if got := GHVaultSecretName("github.com"); got != "gh:github.com" {
		t.Errorf("GHVaultSecretName(github.com) = %q, want gh:github.com", got)
	}
}
