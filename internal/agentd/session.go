package agentd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
	"github.com/BasantPandey/CmdWarden-Omarchy/internal/identity"
)

// sessionGrant is one "Allow for Session" grant: the launcher process (by
// PID + start time, so a reused PID never inherits someone else's grant)
// gets tool at class-and-lower auto-allowed until it exits or goes idle —
// see the Windows CmdWarden Approval Gate spec's "Allow for session."
type sessionGrant struct {
	tool     string
	class    contracts.CommandClass
	lastUsed time.Time
}

// sessionIdleTimeout is how long a grant survives with no covered calls,
// overridable via CW_SESSION_IDLE_SECONDS (matching the Windows spec's env
// var of the same name) — the default mirrors its 60-minute default.
func sessionIdleTimeout() time.Duration {
	if s := os.Getenv("CW_SESSION_IDLE_SECONDS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Minute
}

func sessionKey(pid int, startTime uint64) string {
	return fmt.Sprintf("%d:%d", pid, startTime)
}

func (a *Agent) sessionGrants() map[string]*sessionGrant {
	if a.sessions == nil {
		a.sessions = make(map[string]*sessionGrant)
	}
	return a.sessions
}

// checkSessionGrant reports whether launcher already holds a live,
// non-idle grant covering a command of class on tool, bumping its
// last-used time if so (a covered call itself keeps the grant alive).
func (a *Agent) checkSessionGrant(launcher identity.Launcher, tool string, class contracts.CommandClass) bool {
	key := sessionKey(launcher.PID, launcher.StartTime)

	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()

	grant, ok := a.sessionGrants()[key]
	if !ok {
		return false
	}
	if grant.tool != tool || !classCoveredBy(class, grant.class) {
		return false
	}
	if time.Since(grant.lastUsed) > sessionIdleTimeout() {
		delete(a.sessionGrants(), key)
		return false
	}
	if !identity.IsAlive(launcher.PID, launcher.StartTime) {
		delete(a.sessionGrants(), key)
		return false
	}
	grant.lastUsed = time.Now()
	return true
}

// recordSessionGrant stores a fresh "Allow for Session" grant for launcher.
func (a *Agent) recordSessionGrant(launcher identity.Launcher, tool string, class contracts.CommandClass) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()

	a.sessionGrants()[sessionKey(launcher.PID, launcher.StartTime)] = &sessionGrant{
		tool: tool, class: class, lastUsed: time.Now(),
	}
}

// classRank orders classes from least to most privileged for "class and
// lower" grant coverage; ClassUnknown has no place in that order (see
// classCoveredBy) since the policy matrix treats it as its own tier, not a
// point on the read/write/secret-reveal scale.
func classRank(class contracts.CommandClass) int {
	switch class {
	case contracts.ClassRead:
		return 0
	case contracts.ClassWrite:
		return 1
	case contracts.ClassSecretReveal:
		return 2
	default:
		return -1
	}
}

// classCoveredBy reports whether a grant at grantedClass covers a request
// for requestedClass: unknown only covers unknown exactly (it has no
// meaningful "lower"); otherwise the grant covers its class and anything
// less privileged.
func classCoveredBy(requestedClass, grantedClass contracts.CommandClass) bool {
	if requestedClass == contracts.ClassUnknown || grantedClass == contracts.ClassUnknown {
		return requestedClass == grantedClass
	}
	rq, gr := classRank(requestedClass), classRank(grantedClass)
	if rq < 0 || gr < 0 {
		return false
	}
	return rq <= gr
}
