package policy

import (
	"testing"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func isolateStore(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestUnenrolledDefaultsToDeny(t *testing.T) {
	isolateStore(t)

	level, enrolled, err := Resolve("mise:nobody")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if enrolled {
		t.Error("expected an unenrolled key to report enrolled=false")
	}
	if level != contracts.LevelDeny {
		t.Errorf("level = %q, want %q", level, contracts.LevelDeny)
	}
}

func TestEnrollSetsKindDefaultLevel(t *testing.T) {
	isolateStore(t)

	entry, err := Enroll("mise:claude", contracts.KindAIHarness)
	if err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if entry.Level != contracts.LevelRead {
		t.Errorf("ai-harness default level = %q, want %q", entry.Level, contracts.LevelRead)
	}

	entry, err = Enroll("pacman:foot", contracts.KindTerminal)
	if err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if entry.Level != contracts.LevelTrusted {
		t.Errorf("terminal default level = %q, want %q", entry.Level, contracts.LevelTrusted)
	}
}

func TestEnrollRejectsUnrecognizedKind(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("mise:claude", contracts.LauncherKind("bogus")); err == nil {
		t.Fatal("expected Enroll to reject an unrecognized kind")
	}
}

func TestListReturnsEnrolledLaunchersSorted(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("pacman:foot", contracts.KindTerminal); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if _, err := Enroll("mise:claude", contracts.KindAIHarness); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}

	list, err := List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d entries, want 2", len(list))
	}
	if list[0].IdentityKey != "mise:claude" || list[1].IdentityKey != "pacman:foot" {
		t.Errorf("List not sorted by identity key: %+v", list)
	}
}

func TestSetRequiresPriorEnrollment(t *testing.T) {
	isolateStore(t)

	if _, err := Set("mise:claude", contracts.LevelFull); err == nil {
		t.Fatal("expected Set to fail for an unenrolled key")
	}
}

func TestSetChangesLevel(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("mise:claude", contracts.KindAIHarness); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	entry, err := Set("mise:claude", contracts.LevelFull)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if entry.Level != contracts.LevelFull {
		t.Errorf("Level = %q, want %q", entry.Level, contracts.LevelFull)
	}
	if entry.IdentityKey != "mise:claude" {
		t.Errorf("Set's returned entry has IdentityKey = %q, want %q", entry.IdentityKey, "mise:claude")
	}

	level, enrolled, err := Resolve("mise:claude")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !enrolled || level != contracts.LevelFull {
		t.Errorf("Resolve after Set = (%q, %v), want (%q, true)", level, enrolled, contracts.LevelFull)
	}
}

func TestSetRejectsUnrecognizedLevel(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("mise:claude", contracts.KindAIHarness); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if _, err := Set("mise:claude", contracts.PolicyLevel("Superuser")); err == nil {
		t.Fatal("expected Set to reject an unrecognized level")
	}
}

func TestUnenrollRemovesEntry(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("mise:claude", contracts.KindAIHarness); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if err := Unenroll("mise:claude"); err != nil {
		t.Fatalf("Unenroll failed: %v", err)
	}

	_, enrolled, err := Resolve("mise:claude")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if enrolled {
		t.Error("expected key to be unenrolled")
	}
}

func TestUnenrollUnknownKeyIsNotAnError(t *testing.T) {
	isolateStore(t)

	if err := Unenroll("mise:never-enrolled"); err != nil {
		t.Errorf("Unenroll of an unknown key should not error, got %v", err)
	}
}

func TestReEnrollResetsToKindDefault(t *testing.T) {
	isolateStore(t)

	if _, err := Enroll("mise:claude", contracts.KindAIHarness); err != nil {
		t.Fatalf("Enroll failed: %v", err)
	}
	if _, err := Set("mise:claude", contracts.LevelFull); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	entry, err := Enroll("mise:claude", contracts.KindTerminal)
	if err != nil {
		t.Fatalf("re-Enroll failed: %v", err)
	}
	if entry.Kind != contracts.KindTerminal || entry.Level != contracts.LevelTrusted {
		t.Errorf("re-Enroll = %+v, want Kind=terminal Level=Trusted", entry)
	}
}
