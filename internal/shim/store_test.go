package shim

import (
	"testing"
	"time"

	"github.com/BasantPandey/CmdWarden-Omarchy/internal/contracts"
)

func isolateStore(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestSaveGetListDeletePin(t *testing.T) {
	isolateStore(t)

	pin := Pin{
		Mode: ModeOccupied, Channel: contracts.ChannelMise, ChannelTool: "testtool",
		OriginalPath: "/fake/path", RealBinaryPath: "/fake/path.cmdwarden-real",
		ShimPath: "/fake/path", InstalledAt: time.Now().UTC(),
	}
	if err := savePin("testtool", pin); err != nil {
		t.Fatalf("savePin failed: %v", err)
	}

	got, ok, err := GetPin("testtool")
	if err != nil {
		t.Fatalf("GetPin failed: %v", err)
	}
	if !ok || got.Tool != "testtool" || got.Mode != ModeOccupied {
		t.Errorf("GetPin = %+v, ok=%v", got, ok)
	}

	list, err := ListPins()
	if err != nil {
		t.Fatalf("ListPins failed: %v", err)
	}
	if len(list) != 1 || list[0].Tool != "testtool" {
		t.Errorf("ListPins = %+v", list)
	}

	if err := deletePin("testtool"); err != nil {
		t.Fatalf("deletePin failed: %v", err)
	}
	_, ok, err = GetPin("testtool")
	if err != nil {
		t.Fatalf("GetPin after delete failed: %v", err)
	}
	if ok {
		t.Error("expected pin to be gone after delete")
	}
}
