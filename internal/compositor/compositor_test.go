package compositor

import (
	"testing"

	"terminal-hud/internal/engine"
)

// fakeGrid is a test GridSource: a fixed screen of text rows plus an ordered
// scrollback (index 0 = OLDEST here; newest returned by ScrollbackLine(0)).
type fakeGrid struct {
	rows, cols int
	screen     []string // len rows; each padded/clipped to cols on read
	sb         []string // oldest..newest
	curRow     int
	curCol     int
}

func (g *fakeGrid) Size() (int, int)      { return g.rows, g.cols }
func (g *fakeGrid) CursorPos() (int, int) { return g.curRow, g.curCol }
func (g *fakeGrid) ScrollbackLen() int    { return len(g.sb) }

func (g *fakeGrid) Cell(row, col int) engine.Cell {
	if row < 0 || row >= g.rows || col < 0 || col >= len(g.screen[row]) {
		return engine.Cell{}
	}
	return engine.Cell{Rune: rune(g.screen[row][col]), Width: 1}
}

func (g *fakeGrid) ScrollbackLine(n int) []engine.Cell {
	if n < 0 || n >= len(g.sb) {
		return nil
	}
	return mkCells(g.sb[len(g.sb)-1-n]) // n=0 -> last element = newest
}

func midRow(f interface{ At(r, c int) engine.Cell }, row, cols int) string {
	var r []rune
	for c := 0; c < cols; c++ {
		ch := f.At(row, c).Rune
		if ch == 0 {
			ch = ' '
		}
		r = append(r, ch)
	}
	return string(r)
}

func TestComposeLiveViewPlacesBarsAndScreen(t *testing.T) {
	g := &fakeGrid{rows: 2, cols: 5, screen: []string{"AB", "CD"}, curRow: 1, curCol: 2}
	f := Compose(4, 5, g, 0, mkCells("TOP"), mkCells("BOT"))

	if f.Rows != 4 || f.Cols != 5 {
		t.Fatalf("frame dims = %dx%d, want 4x5", f.Rows, f.Cols)
	}
	if got := midRow(f, 0, 5); got != "TOP  " {
		t.Fatalf("top bar = %q, want 'TOP  '", got)
	}
	if got := midRow(f, 3, 5); got != "BOT  " {
		t.Fatalf("bottom bar = %q, want 'BOT  '", got)
	}
	if got := midRow(f, 1, 5); got != "AB   " {
		t.Fatalf("middle row 0 = %q, want 'AB   '", got)
	}
	if got := midRow(f, 2, 5); got != "CD   " {
		t.Fatalf("middle row 1 = %q, want 'CD   '", got)
	}
	if !f.CursorShown || f.CursorRow != 2 || f.CursorCol != 2 {
		t.Fatalf("cursor = (%d,%d) shown=%v, want (2,2) true", f.CursorRow, f.CursorCol, f.CursorShown)
	}
}
