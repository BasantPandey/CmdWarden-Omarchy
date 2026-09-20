package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func sampleRecord(decision contracts.Decision) contracts.AuditRecord {
	return contracts.AuditRecord{
		Decision:       decision,
		ReasonCode:     "test-reason",
		Tool:           "gh",
		CommandClass:   contracts.ClassWrite,
		PolicyLevel:    "Read",
		IdentityKey:    "mise:claude",
		LauncherKind:   "ai-harness",
		EnrollmentKind: "ai-harness",
		SecretName:     "gh-token",
	}
}

func TestLogAndReadAllRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	decisions := []contracts.Decision{
		contracts.DecisionAutoAllow, contracts.DecisionAllowOnce, contracts.DecisionDeny,
		contracts.DecisionUnavailable, contracts.DecisionSessionGrant, contracts.DecisionSessionAllow,
	}
	for _, d := range decisions {
		if err := Log(sampleRecord(d)); err != nil {
			t.Fatalf("Log(%s) failed: %v", d, err)
		}
	}

	records, err := ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(records) != len(decisions) {
		t.Fatalf("got %d records, want %d (exactly one row per decision)", len(records), len(decisions))
	}
	for i, rec := range records {
		if rec.Decision != decisions[i] {
			t.Errorf("record %d decision = %q, want %q", i, rec.Decision, decisions[i])
		}
		if rec.Timestamp.IsZero() {
			t.Errorf("record %d has zero timestamp", i)
		}
	}
}

func TestLogRejectsUnrecognizedDecision(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rec := sampleRecord(contracts.Decision("not-a-real-decision"))
	if err := Log(rec); err == nil {
		t.Fatal("expected Log to reject an unrecognized decision, got nil error")
	}
}

func TestLogRequiresReasonCodeAndTool(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rec := sampleRecord(contracts.DecisionDeny)
	rec.ReasonCode = ""
	if err := Log(rec); err == nil {
		t.Error("expected error when reason_code is empty")
	}

	rec2 := sampleRecord(contracts.DecisionDeny)
	rec2.Tool = ""
	if err := Log(rec2); err == nil {
		t.Error("expected error when tool is empty")
	}
}

func TestAuditRecordNeverCarriesSecretValueOrArgv(t *testing.T) {
	// Structural guarantee: marshal a record and confirm only the
	// documented field set appears — there is no field a caller could
	// have populated with a secret value or a full command line.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	rec := sampleRecord(contracts.DecisionSessionGrant)

	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var asMap map[string]any
	if err := json.Unmarshal(line, &asMap); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	allowed := map[string]bool{
		"ts": true, "decision": true, "reason_code": true, "tool": true,
		"command_class": true, "policy_level": true, "launcher_policy_key": true,
		"launcher_kind": true, "enrollment_kind": true, "secret_name": true,
		"purpose": true, "path": true, "pid": true,
	}
	for k := range asMap {
		if !allowed[k] {
			t.Errorf("audit record has unexpected field %q (not in the documented schema)", k)
		}
	}
	if strings.Contains(string(line), "secret_value") || strings.Contains(string(line), "argv") {
		t.Error("audit record JSON must never contain a secret_value or argv field")
	}
}

func TestFailClosedWhenLogUnwritable(t *testing.T) {
	// Point XDG_STATE_HOME at a location that can't be a directory (a
	// regular file in its place), so Path()'s MkdirAll fails and Log
	// must fail closed rather than silently succeeding.
	tmp := t.TempDir()
	blocked := tmp + "/blocked"
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", blocked+"/cannot-create-under-a-file")

	if err := Log(sampleRecord(contracts.DecisionSessionGrant)); err == nil {
		t.Fatal("expected Log to fail closed when the state directory can't be created, got nil error")
	}
}

func TestPruneDropsOldRecords(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	old := sampleRecord(contracts.DecisionDeny)
	old.Timestamp = time.Now().Add(-60 * 24 * time.Hour)
	recent := sampleRecord(contracts.DecisionAutoAllow)
	recent.Timestamp = time.Now()

	writeRawRecord(t, old)
	writeRawRecord(t, recent)

	kept, dropped, err := Prune(RetentionWindow)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if kept != 1 || dropped != 1 {
		t.Errorf("Prune kept=%d dropped=%d, want kept=1 dropped=1", kept, dropped)
	}

	records, err := ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after prune failed: %v", err)
	}
	if len(records) != 1 || records[0].Decision != contracts.DecisionAutoAllow {
		t.Errorf("unexpected records after prune: %+v", records)
	}
}

func TestFollowStopsCleanly(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := Log(sampleRecord(contracts.DecisionAutoAllow)); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	stop := make(chan struct{})
	done := make(chan error, 1)
	buf := &bytes.Buffer{}
	go func() { done <- Follow(buf, stop) }()

	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Follow returned error after stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Follow did not stop within 3s of the stop channel closing")
	}
}

// writeRawRecord is a small test helper for seeding fixtures via Log.
func writeRawRecord(t *testing.T, rec contracts.AuditRecord) {
	t.Helper()
	if err := Log(rec); err != nil {
		t.Fatalf("seeding record failed: %v", err)
	}
}
