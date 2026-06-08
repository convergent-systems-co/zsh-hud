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
