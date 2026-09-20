package cliapp

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommandPrintsSomethingReal(t *testing.T) {
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("cw version returned error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "cmdwarden") {
		t.Errorf("expected version output to mention cmdwarden, got %q", got)
	}
}

func TestHelpCommandRuns(t *testing.T) {
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("cw help returned error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected cw help to print usage text")
	}
}
