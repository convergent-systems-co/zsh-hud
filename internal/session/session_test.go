package session

import (
	"testing"

	"terminal-hud/internal/engine"
	"terminal-hud/internal/hud"
	"terminal-hud/internal/input"
)

// fakeGrid satisfies compositor.GridSource for tests.
type fakeGrid struct {
	rows, cols int
	sb         int // scrollback length
}

func (g *fakeGrid) Size() (int, int)          { return g.rows, g.cols }
func (g *fakeGrid) Cell(r, c int) engine.Cell { return engine.Cell{Rune: 'x', Width: 1} }
func (g *fakeGrid) CursorPos() (int, int)     { return 0, 0 }
func (g *fakeGrid) ScrollbackLen() int        { return g.sb }
func (g *fakeGrid) ScrollbackLine(n int) []engine.Cell {
	return []engine.Cell{{Rune: 's', Width: 1}}
}

func TestNewSessionLiveScroll(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 100}, 10, 20)
	if s.ScrollOffset() != 0 {
		t.Fatalf("initial scrollOffset = %d, want 0", s.ScrollOffset())
	}
}

func TestApplyScrollClampsToScrollbackLen(t *testing.T) {
	g := &fakeGrid{rows: 8, cols: 20, sb: 3}
	s := New(g, 10, 20)
	for i := 0; i < 10; i++ {
		s.Apply(actionScrollLineUp())
	}
	if s.ScrollOffset() != 3 {
		t.Fatalf("scrollOffset = %d, want clamped to 3", s.ScrollOffset())
	}
}

func TestApplyScrollDownClampsToZero(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 50}, 10, 20)
	s.Apply(actionScrollLineDown()) // already at 0
	if s.ScrollOffset() != 0 {
		t.Fatalf("scrollOffset = %d, want 0", s.ScrollOffset())
	}
}

func TestExitCopyModeResetsToLive(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 50}, 10, 20)
	s.Apply(actionScrollPageUp())
	if s.ScrollOffset() == 0 {
		t.Fatal("page up should scroll")
	}
	s.Apply(actionExitCopyMode())
	if s.ScrollOffset() != 0 {
		t.Fatalf("exit copy should reset to live, got %d", s.ScrollOffset())
	}
}

func TestFrameHasBarsAndDimensions(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 0}, 10, 20)
	s.SetDeps(hud.Deps{Time: "12:00:00"})
	f := s.Frame()
	if f.Rows != 10 || f.Cols != 20 {
		t.Fatalf("frame dims = %dx%d, want 10x20", f.Rows, f.Cols)
	}
	// top bar row 0 should contain the time text's first rune
	if f.At(0, 0).Rune != '1' {
		t.Fatalf("top bar not rendered; (0,0)=%q", f.At(0, 0).Rune)
	}
}

func actionScrollLineUp() input.Action   { return input.ScrollLineUp }
func actionScrollLineDown() input.Action { return input.ScrollLineDown }
func actionScrollPageUp() input.Action   { return input.ScrollPageUp }
func actionExitCopyMode() input.Action   { return input.ExitCopyMode }
