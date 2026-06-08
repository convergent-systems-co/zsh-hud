package render

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestNewFrameDimensionsAndBlankCells(t *testing.T) {
	f := NewFrame(3, 5)
	if f.Rows != 3 || f.Cols != 5 {
		t.Fatalf("dims = %dx%d, want 3x5", f.Rows, f.Cols)
	}
	if got := f.At(0, 0); got.Rune != 0 {
		t.Fatalf("blank cell rune = %q, want 0", got.Rune)
	}
}

func TestFrameSetAndAt(t *testing.T) {
	f := NewFrame(2, 2)
	f.Set(1, 1, engine.Cell{Rune: 'X', Width: 1})
	if got := f.At(1, 1); got.Rune != 'X' {
		t.Fatalf("At(1,1).Rune = %q, want X", got.Rune)
	}
	if got := f.At(9, 9); got.Rune != 0 {
		t.Fatalf("At(9,9) should be zero Cell")
	}
	f.Set(9, 9, engine.Cell{Rune: 'Z'}) // must not panic
}

func TestFrameEqualCellAndDims(t *testing.T) {
	a := NewFrame(1, 2)
	b := NewFrame(1, 2)
	if !a.SameSize(b) {
		t.Fatal("same dims should report SameSize")
	}
	c := NewFrame(2, 2)
	if a.SameSize(c) {
		t.Fatal("different dims should not be SameSize")
	}
}
