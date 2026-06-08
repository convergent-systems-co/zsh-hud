package compositor

import (
	"testing"

	"terminal-hud/internal/engine"
)

func cellsRunes(cs []engine.Cell) string {
	var r []rune
	for _, c := range cs {
		if c.Rune == 0 {
			r = append(r, ' ')
		} else {
			r = append(r, c.Rune)
		}
	}
	return string(r)
}

func mkCells(s string) []engine.Cell {
	out := make([]engine.Cell, 0, len(s))
	for _, r := range s {
		out = append(out, engine.Cell{Rune: r, Width: 1})
	}
	return out
}

func TestPadOrClipPads(t *testing.T) {
	got := padOrClip(mkCells("hi"), 5)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if cellsRunes(got) != "hi   " {
		t.Fatalf("padded = %q, want 'hi   '", cellsRunes(got))
	}
}

func TestPadOrClipClips(t *testing.T) {
	got := padOrClip(mkCells("toolong"), 4)
	if len(got) != 4 || cellsRunes(got) != "tool" {
		t.Fatalf("clipped = %q (len %d), want 'tool'", cellsRunes(got), len(got))
	}
}

func TestPadOrClipExact(t *testing.T) {
	got := padOrClip(mkCells("abcd"), 4)
	if cellsRunes(got) != "abcd" {
		t.Fatalf("exact = %q", cellsRunes(got))
	}
}
