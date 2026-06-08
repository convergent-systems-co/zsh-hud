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

func feedCopy(t *testing.T) *Interpreter {
	t.Helper()
	it := New()
	it.Feed([]byte("\x1b[5;2~")) // enter copy mode
	if it.Mode() != ModeCopy {
		t.Fatal("setup: expected copy mode")
	}
	return it
}

func TestCopyModeMotionKeys(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("jjkl")) // down down up right (vim keys)
	want := []Action{CopyMoveDown, CopyMoveDown, CopyMoveUp, CopyMoveRight}
	if len(r.Actions) != len(want) {
		t.Fatalf("actions = %v, want %v", r.Actions, want)
	}
	for i := range want {
		if r.Actions[i] != want[i] {
			t.Fatalf("action[%d] = %v, want %v", i, r.Actions[i], want[i])
		}
	}
	if len(r.Forward) != 0 {
		t.Fatalf("copy mode must capture, not forward; got %q", r.Forward)
	}
}

func TestCopyModeArrowKeys(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[A\x1b[B\x1b[C\x1b[D")) // up down right left
	want := []Action{CopyMoveUp, CopyMoveDown, CopyMoveRight, CopyMoveLeft}
	for i := range want {
		if r.Actions[i] != want[i] {
			t.Fatalf("arrow action[%d] = %v, want %v", i, r.Actions[i], want[i])
		}
	}
}

func TestCopyModeSelectAndYank(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("vy")) // toggle select, yank
	if len(r.Actions) != 3 || r.Actions[0] != CopyToggleSelect || r.Actions[1] != CopyYank || r.Actions[2] != ExitCopyMode {
		t.Fatalf("actions = %v, want [CopyToggleSelect CopyYank ExitCopyMode]", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("yank should return to normal mode")
	}
}

func TestCopyModeQuitExits(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("q"))
	if len(r.Actions) != 1 || r.Actions[0] != ExitCopyMode {
		t.Fatalf("actions = %v, want [ExitCopyMode]", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("q should exit copy mode")
	}
}

func TestCopyModePageScroll(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[6~")) // PageDown
	if len(r.Actions) != 1 || r.Actions[0] != ScrollPageDown {
		t.Fatalf("actions = %v, want [ScrollPageDown]", r.Actions)
	}
}

func TestNormalModeEscNonCSIForwarded(t *testing.T) {
	it := New()
	// ESC followed by a non-'[' byte: ESC forwarded standalone, then 'x'.
	r := it.Feed([]byte("\x1bx"))
	if !bytes.Equal(r.Forward, []byte("\x1bx")) {
		t.Fatalf("ESC+non-CSI forward = %q, want [ESC x]", r.Forward)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("should remain in normal mode")
	}
}

func TestCopyModeWheelScroll(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[<64;1;1M\x1b[<65;1;1M")) // wheel up then down
	if len(r.Actions) != 2 || r.Actions[0] != ScrollLineUp || r.Actions[1] != ScrollLineDown {
		t.Fatalf("actions = %v, want [ScrollLineUp ScrollLineDown]", r.Actions)
	}
	if len(r.Forward) != 0 {
		t.Fatalf("copy-mode wheel must not forward; got %q", r.Forward)
	}
}

func TestCopyModeShiftPageScroll(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[5;2~\x1b[6;2~")) // Shift+PgUp then Shift+PgDn
	if len(r.Actions) != 2 || r.Actions[0] != ScrollPageUp || r.Actions[1] != ScrollPageDown {
		t.Fatalf("actions = %v, want [ScrollPageUp ScrollPageDown]", r.Actions)
	}
}
