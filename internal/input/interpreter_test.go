package input

import (
	"bytes"
	"testing"
)

func TestNormalModeForwardsPlainBytes(t *testing.T) {
	it := New()
	r := it.Feed([]byte("ls -la\n"))
	if !bytes.Equal(r.Forward, []byte("ls -la\n")) {
		t.Fatalf("forward = %q, want 'ls -la\\n'", r.Forward)
	}
	if len(r.Actions) != 0 {
		t.Fatalf("actions = %v, want none", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("should stay in normal mode")
	}
}

func TestShiftPageUpEntersCopyModeAndScrolls(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[5;2~")) // Shift+PageUp
	if len(r.Forward) != 0 {
		t.Fatalf("Shift+PgUp must not be forwarded; got %q", r.Forward)
	}
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode || r.Actions[1] != ScrollPageUp {
		t.Fatalf("actions = %v, want [EnterCopyMode ScrollPageUp]", r.Actions)
	}
	if it.Mode() != ModeCopy {
		t.Fatal("should be in copy mode")
	}
}

func TestSplitEscapeSequenceBuffered(t *testing.T) {
	it := New()
	if r := it.Feed([]byte("\x1b[5")); len(r.Forward) != 0 || len(r.Actions) != 0 {
		t.Fatalf("partial CSI should buffer, not emit; got %+v", r)
	}
	r := it.Feed([]byte(";2~")) // completes Shift+PageUp
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode {
		t.Fatalf("completed sequence actions = %v", r.Actions)
	}
}

func TestUnrecognizedCSIForwardedInNormalMode(t *testing.T) {
	it := New()
	// F5 = ESC[15~ ; not a hotkey, must be forwarded verbatim to the shell.
	r := it.Feed([]byte("\x1b[15~"))
	if !bytes.Equal(r.Forward, []byte("\x1b[15~")) {
		t.Fatalf("unrecognized CSI forward = %q, want verbatim", r.Forward)
	}
}
