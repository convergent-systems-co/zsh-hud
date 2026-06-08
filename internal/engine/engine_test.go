package engine

import "testing"

func TestEngineReadsBackPlainText(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if _, err := e.Write([]byte("Hello, \x1b[1;32mworld\x1b[0m!")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got []rune
	for col := 0; col < 14; col++ {
		c := e.Cell(0, col)
		if c.Rune != 0 {
			got = append(got, c.Rune)
		}
	}
	if string(got) != "Hello, world!" {
		t.Fatalf("row 0 = %q, want %q", string(got), "Hello, world!")
	}
}

func TestEngineCursorAdvances(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if _, err := e.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	row, col := e.CursorPos()
	if row != 0 || col != 3 {
		t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
	}
}

func TestEngineParsesColor(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	// SGR 32 = indexed green (palette index 2).
	if _, err := e.Write([]byte("\x1b[32mX")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	c := e.Cell(0, 0)
	if c.Rune != 'X' {
		t.Fatalf("rune = %q, want X", c.Rune)
	}
	if !c.FG.IsIndexed || c.FG.Index != 2 {
		t.Fatalf("fg = %+v, want indexed 2", c.FG)
	}
}

func TestEngineResizeChangesGrid(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	e.Resize(10, 40)
	r, c := e.Size()
	if r != 10 || c != 40 {
		t.Fatalf("size = (%d,%d), want (10,40)", r, c)
	}
}

func TestEngineScrollbackFillsOnOverflow(t *testing.T) {
	// 3-row grid; writing 5 newline-separated lines pushes 2 off the top.
	e, err := New(3, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if _, err := e.Write([]byte("L0\r\nL1\r\nL2\r\nL3\r\nL4\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if e.ScrollbackLen() < 2 {
		t.Fatalf("scrollback len = %d, want >= 2", e.ScrollbackLen())
	}
	top := lineText(e.ScrollbackLine(0))
	if top != "L1" && top != "L2" {
		t.Fatalf("scrollback line(0) = %q, want a scrolled-off line", top)
	}
}

func TestNewRejectsNonPositiveSize(t *testing.T) {
	if _, err := New(0, 80, 100); err == nil {
		t.Fatal("New(0,80,...) should error on zero rows")
	}
	if _, err := New(24, 0, 100); err == nil {
		t.Fatal("New(24,0,...) should error on zero cols")
	}
}

func lineText(cells []Cell) string {
	var r []rune
	for _, c := range cells {
		if c.Rune != 0 {
			r = append(r, c.Rune)
		}
	}
	return string(r)
}
