package agentd

import (
	"os"
	"testing"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/identity"
)

// aliveLauncher returns a Launcher pointing at the test process itself —
// guaranteed alive with a real start time, unlike a made-up PID.
func aliveLauncher(t *testing.T) identity.Launcher {
	t.Helper()
	st, err := identity.StartTime(os.Getpid())
	if err != nil {
		t.Skipf("no /proc on this system: %v", err)
	}
	return identity.Launcher{PID: os.Getpid(), StartTime: st}
}

func TestClassCoveredBy(t *testing.T) {
	cases := []struct {
		requested, granted contracts.CommandClass
		want               bool
	}{
		{contracts.ClassRead, contracts.ClassRead, true},
		{contracts.ClassRead, contracts.ClassWrite, true},
		{contracts.ClassRead, contracts.ClassSecretReveal, true},
		{contracts.ClassWrite, contracts.ClassRead, false},
		{contracts.ClassWrite, contracts.ClassWrite, true},
		{contracts.ClassWrite, contracts.ClassSecretReveal, true},
		{contracts.ClassSecretReveal, contracts.ClassWrite, false},
		{contracts.ClassSecretReveal, contracts.ClassSecretReveal, true},
		{contracts.ClassUnknown, contracts.ClassSecretReveal, false},
		{contracts.ClassUnknown, contracts.ClassUnknown, true},
		{contracts.ClassRead, contracts.ClassUnknown, false},
	}
	for _, tc := range cases {
		if got := classCoveredBy(tc.requested, tc.granted); got != tc.want {
			t.Errorf("classCoveredBy(%s, %s) = %v, want %v", tc.requested, tc.granted, got, tc.want)
		}
	}
}

func TestSessionGrantCoversSubsequentCallWithoutRePrompting(t *testing.T) {
	a := &Agent{stopCh: make(chan struct{})}
	launcher := aliveLauncher(t)

	if a.checkSessionGrant(launcher, "gh", contracts.ClassWrite) {
		t.Fatal("expected no grant before one is recorded")
	}

	a.recordSessionGrant(launcher, "gh", contracts.ClassWrite)

	if !a.checkSessionGrant(launcher, "gh", contracts.ClassWrite) {
		t.Error("expected the just-recorded grant to cover a matching subsequent call")
	}
	if !a.checkSessionGrant(launcher, "gh", contracts.ClassRead) {
		t.Error("expected a write grant to also cover a lower-privilege read call")
	}
	if a.checkSessionGrant(launcher, "gh", contracts.ClassSecretReveal) {
		t.Error("expected a write grant to NOT cover secret-reveal")
	}
	if a.checkSessionGrant(launcher, "git", contracts.ClassRead) {
		t.Error("expected the grant to be scoped to its own tool")
	}
}

func TestSessionGrantDoesNotCoverADifferentLauncherProcess(t *testing.T) {
	a := &Agent{stopCh: make(chan struct{})}
	launcher := aliveLauncher(t)
	a.recordSessionGrant(launcher, "gh", contracts.ClassWrite)

	differentPID := identity.Launcher{PID: launcher.PID + 1, StartTime: launcher.StartTime}
	if a.checkSessionGrant(differentPID, "gh", contracts.ClassWrite) {
		t.Error("expected a grant for one PID not to cover a different PID")
	}
	reusedPID := identity.Launcher{PID: launcher.PID, StartTime: launcher.StartTime + 1}
	if a.checkSessionGrant(reusedPID, "gh", contracts.ClassWrite) {
		t.Error("expected a grant to not cover the same PID reused by a different process (different start time)")
	}
}

func TestSessionGrantExpiresWhenIdle(t *testing.T) {
	t.Setenv("CW_SESSION_IDLE_SECONDS", "1")

	a := &Agent{stopCh: make(chan struct{})}
	launcher := aliveLauncher(t)
	a.recordSessionGrant(launcher, "gh", contracts.ClassWrite)

	a.sessionMu.Lock()
	a.sessionGrants()[sessionKey(launcher.PID, launcher.StartTime)].lastUsed = time.Now().Add(-2 * time.Second)
	a.sessionMu.Unlock()

	if a.checkSessionGrant(launcher, "gh", contracts.ClassWrite) {
		t.Error("expected an idle-expired grant to no longer cover a call")
	}
}

func TestSessionGrantDoesNotCoverAfterLauncherExits(t *testing.T) {
	// PID 0 never corresponds to a real, alive process — identity.IsAlive
	// should report false for it, which checkSessionGrant must treat as
	// "the launcher exited," not "still covered."
	a := &Agent{stopCh: make(chan struct{})}
	launcher := identity.Launcher{PID: 0, StartTime: 1}
	a.recordSessionGrant(launcher, "gh", contracts.ClassWrite)

	if a.checkSessionGrant(launcher, "gh", contracts.ClassWrite) {
		t.Error("expected a grant for a no-longer-alive launcher to not cover a call")
	}
}
