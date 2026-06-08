package render

import (
	"bytes"
	"io"

	"terminal-hud/internal/engine"
)

// Renderer draws Frames to an io.Writer, emitting only what changed since the
// last Render. Not safe for concurrent use; the event loop calls it serially.
type Renderer struct {
	w    io.Writer
	last *Frame
}

// NewRenderer returns a Renderer writing to w. The first Render is a full redraw.
func NewRenderer(w io.Writer) *Renderer { return &Renderer{w: w} }

// Render draws f. On the first call or when the size changes it does a full
// redraw; otherwise it emits only cells that differ from the last frame.
func (r *Renderer) Render(f *Frame) error {
	var b bytes.Buffer
	if r.last == nil || !r.last.SameSize(f) {
		r.fullRedraw(&b, f)
	} else {
		r.diff(&b, f)
	}
	b.WriteString(cursorTo(f.CursorRow, f.CursorCol))
	if _, err := r.w.Write(b.Bytes()); err != nil {
		return err
	}
	r.last = cloneFrame(f)
	return nil
}

// fullRedraw clears the screen and writes every cell, tracking the active style
// to avoid redundant SGR escapes.
func (r *Renderer) fullRedraw(b *bytes.Buffer, f *Frame) {
	b.WriteString(clear)
	cur := ""
	for row := 0; row < f.Rows; row++ {
		b.WriteString(cursorTo(row, 0))
		for col := 0; col < f.Cols; col++ {
			cur = writeCell(b, f.At(row, col), cur)
		}
	}
}

// writeCell emits a cell's style (only if it differs from cur) and its glyph,
// returning the new active style. A zero rune renders as a space.
func writeCell(b *bytes.Buffer, c engine.Cell, cur string) string {
	st := encodeStyle(c.FG, c.BG, c.Attrs)
	if st != cur {
		b.WriteString(st)
		cur = st
	}
	if c.Rune == 0 {
		b.WriteByte(' ')
	} else {
		b.WriteRune(c.Rune)
	}
	return cur
}

func cloneFrame(f *Frame) *Frame {
	cp := &Frame{Rows: f.Rows, Cols: f.Cols, CursorRow: f.CursorRow, CursorCol: f.CursorCol, CursorShown: f.CursorShown}
	cp.cells = make([]engine.Cell, len(f.cells))
	copy(cp.cells, f.cells)
	return cp
}

// TEMP: replaced by the real diff in Task 4.
func (r *Renderer) diff(b *bytes.Buffer, f *Frame) { r.fullRedraw(b, f) }
