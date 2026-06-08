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
// The cursor position is repositioned only when it has changed (or on a full
// redraw), consistent with the diff-only-what-changed contract.
func (r *Renderer) Render(f *Frame) error {
	var b bytes.Buffer
	fullRedraw := r.last == nil || !r.last.SameSize(f)
	if fullRedraw {
		r.fullRedraw(&b, f)
	} else {
		r.diff(&b, f)
	}
	// Emit cursor position if this is a full redraw or the cursor moved.
	if fullRedraw || r.last.CursorRow != f.CursorRow || r.last.CursorCol != f.CursorCol {
		b.WriteString(cursorTo(f.CursorRow, f.CursorCol))
	}
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

// diff emits only cells that changed from the last frame. Each maximal run of
// adjacent changed cells in a row is repositioned once; the terminal advances
// the cursor as glyphs are written. The active style is reset at each run.
func (r *Renderer) diff(b *bytes.Buffer, f *Frame) {
	for row := 0; row < f.Rows; row++ {
		col := 0
		for col < f.Cols {
			if cellsEqual(f.At(row, col), r.last.At(row, col)) {
				col++
				continue
			}
			b.WriteString(cursorTo(row, col))
			cur := ""
			for col < f.Cols && !cellsEqual(f.At(row, col), r.last.At(row, col)) {
				cur = writeCell(b, f.At(row, col), cur)
				col++
			}
		}
	}
}

// cellsEqual reports whether two cells are visually identical.
func cellsEqual(a, b engine.Cell) bool {
	return a.Rune == b.Rune && a.Width == b.Width && a.Attrs == b.Attrs && a.FG == b.FG && a.BG == b.BG
}
