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
