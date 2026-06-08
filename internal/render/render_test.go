package render

import (
	"bytes"
	"strings"
	"testing"

	"terminal-hud/internal/engine"
)

func putText(f *Frame, row, col int, s string) {
	for i, r := range s {
		f.Set(row, col+i, engine.Cell{Rune: r, Width: 1})
	}
}

func TestRendererFirstFrameFullRedraw(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	f := NewFrame(1, 5)
	putText(f, 0, 0, "hi")
	f.CursorRow, f.CursorCol = 0, 2

	if err := r.Render(f); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, clear) {
		t.Fatalf("first frame should clear screen; got %q", out)
	}
	if !strings.Contains(out, "h") || !strings.Contains(out, "i") {
		t.Fatalf("output missing content; got %q", out)
	}
	if !strings.HasSuffix(out, cursorTo(0, 2)) {
		t.Fatalf("output should end positioning cursor at (0,2); got %q", out)
	}
}

func TestRendererDiffEmitsOnlyChangedCells(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	f1 := NewFrame(1, 5)
	putText(f1, 0, 0, "abcde")
	if err := r.Render(f1); err != nil {
		t.Fatalf("Render f1: %v", err)
	}

	buf.Reset()
	f2 := NewFrame(1, 5)
	putText(f2, 0, 0, "abXde")
	if err := r.Render(f2); err != nil {
		t.Fatalf("Render f2: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, clear) {
		t.Fatalf("diff must not clear screen; got %q", out)
	}
	if !strings.Contains(out, cursorTo(0, 2)) || !strings.Contains(out, "X") {
		t.Fatalf("diff should reposition to (0,2) and emit X; got %q", out)
	}
	// Only the changed cell's glyph should be emitted; unchanged glyphs must not.
	for _, g := range []string{"a", "b", "d", "e"} {
		if strings.Contains(out, g) {
			t.Fatalf("diff re-emitted unchanged glyph %q; got %q", g, out)
		}
	}
}

func TestRendererUnchangedFrameEmitsNoCellWrites(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	f := NewFrame(1, 3)
	putText(f, 0, 0, "xyz")
	_ = r.Render(f)

	buf.Reset()
	same := NewFrame(1, 3)
	putText(same, 0, 0, "xyz")
	same.CursorRow, same.CursorCol = f.CursorRow, f.CursorCol
	_ = r.Render(same)
	out := buf.String()
	if strings.ContainsAny(out, "xyz") {
		t.Fatalf("unchanged frame should emit no glyphs; got %q", out)
	}
}

func TestRendererSizeChangeForcesFullRedraw(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	_ = r.Render(NewFrame(1, 3))

	buf.Reset()
	_ = r.Render(NewFrame(2, 4))
	if !strings.Contains(buf.String(), clear) {
		t.Fatalf("size change should full-redraw (clear); got %q", buf.String())
	}
}
