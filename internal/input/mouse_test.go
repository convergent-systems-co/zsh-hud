package input

import "testing"

func TestIsWheelUpDown(t *testing.T) {
	// SGR mouse params are "<button;col;row". Wheel up = 64, wheel down = 65.
	if !isWheelUp("<64;10;5") {
		t.Fatal("64 should be wheel up")
	}
	if isWheelUp("<65;10;5") {
		t.Fatal("65 is wheel down, not up")
	}
	if !isWheelDown("<65;10;5") {
		t.Fatal("65 should be wheel down")
	}
	if isWheelUp("<0;10;5") {
		t.Fatal("0 is left-click, not wheel")
	}
}

func TestWheelUpInNormalModeEntersCopy(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[<64;10;5M"))
	if it.Mode() != ModeCopy {
		t.Fatal("wheel up should enter copy mode")
	}
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode || r.Actions[1] != ScrollLineUp {
		t.Fatalf("actions = %v, want [EnterCopyMode ScrollLineUp]", r.Actions)
	}
	if len(r.Forward) != 0 {
		t.Fatalf("mouse event must not be forwarded; got %q", r.Forward)
	}
}

func TestLeftClickSwallowedNotForwarded(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[<0;3;3M"))
	if len(r.Forward) != 0 || len(r.Actions) != 0 {
		t.Fatalf("unhandled mouse event should be swallowed; got %+v", r)
	}
}
