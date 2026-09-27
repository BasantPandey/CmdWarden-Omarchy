package ghauth

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenKeyringAbsentBusIsMissing(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(t.TempDir(), "no-bus"))
	kr, err := OpenKeyring()
	if kr != nil {
		kr.Close()
		t.Fatal("OpenKeyring returned a keyring for an absent bus")
	}
	if !errors.Is(err, ErrKeyringMissing) {
		t.Fatalf("OpenKeyring err = %v, want missing keyring", err)
	}
}
