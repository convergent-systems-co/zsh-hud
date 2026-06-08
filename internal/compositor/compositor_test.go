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

func TestComposeScrollbackOffset(t *testing.T) {
	g := &fakeGrid{
		rows: 3, cols: 4,
		screen: []string{"L0", "L1", "L2"},
		sb:     []string{"s0", "s1", "s2", "s3", "s4"},
		curRow: 2, curCol: 0,
	}
	// 5-row screen => midHeight 3, scrollOffset 2: virtual i=0,1,2 = 3,4,5 => s3,s4,L0
	f := Compose(5, 4, g, 2, mkCells("T"), mkCells("B"))
	if got := midRow(f, 1, 4); got != "s3  " {
		t.Fatalf("mid row 0 = %q, want 's3  '", got)
	}
	if got := midRow(f, 2, 4); got != "s4  " {
		t.Fatalf("mid row 1 = %q, want 's4  '", got)
	}
	if got := midRow(f, 3, 4); got != "L0  " {
		t.Fatalf("mid row 2 = %q, want 'L0  '", got)
	}
	if f.CursorShown {
		t.Fatal("cursor should be hidden when scrolled back")
	}
}

func TestComposeScrollOffsetClampedToScrollbackLen(t *testing.T) {
	g := &fakeGrid{rows: 2, cols: 3, screen: []string{"L0", "L1"}, sb: []string{"a", "b"}, curRow: 0, curCol: 0}
	// 4-row screen => midHeight 2; scrollOffset 99 clamps to 2: virtual 0,1 => a,b
	f := Compose(4, 3, g, 99, mkCells("T"), mkCells("B"))
	if got := midRow(f, 1, 3); got != "a  " {
		t.Fatalf("mid row 0 = %q, want 'a  '", got)
	}
	if got := midRow(f, 2, 3); got != "b  " {
		t.Fatalf("mid row 1 = %q, want 'b  '", got)
	}
}

func TestComposeNegativeOffsetTreatedAsLive(t *testing.T) {
	g := &fakeGrid{rows: 2, cols: 2, screen: []string{"L0", "L1"}, curRow: 0, curCol: 1}
	f := Compose(4, 2, g, -5, mkCells("T"), mkCells("B"))
	if got := midRow(f, 1, 2); got != "L0" {
		t.Fatalf("mid row 0 = %q, want 'L0'", got)
	}
	if !f.CursorShown || f.CursorRow != 1 || f.CursorCol != 1 {
		t.Fatalf("cursor = (%d,%d) shown=%v, want (1,1) true", f.CursorRow, f.CursorCol, f.CursorShown)
	}
}

func TestComposeTinyTerminalNoMiddle(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 5, screen: nil}
	f := Compose(2, 5, g, 0, mkCells("TOP"), mkCells("BOT")) // must not panic
	if f.Rows != 2 {
		t.Fatalf("rows = %d, want 2", f.Rows)
	}
	if got := midRow(f, 0, 5); got != "TOP  " {
		t.Fatalf("top = %q", got)
	}
	if got := midRow(f, 1, 5); got != "BOT  " {
		t.Fatalf("bottom = %q", got)
	}
	if f.CursorShown {
		t.Fatal("no middle => cursor hidden")
	}
}

func TestComposeSingleRowOnlyTopBar(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 3}
	f := Compose(1, 3, g, 0, mkCells("HI"), mkCells("XX")) // must not panic
	if f.Rows != 1 {
		t.Fatalf("rows = %d, want 1", f.Rows)
	}
	if got := midRow(f, 0, 3); got != "HI " {
		t.Fatalf("row0 = %q, want 'HI '", got)
	}
}

func TestComposeZeroRows(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 0}
	f := Compose(0, 0, g, 0, nil, nil) // must not panic
	if f.Rows != 0 {
		t.Fatalf("rows = %d, want 0", f.Rows)
	}
}
