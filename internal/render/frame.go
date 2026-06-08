package render

import "terminal-hud/internal/engine"

// Frame is a composited screen: a row-major grid of cells plus the cursor the
// terminal should show after drawing. Cell (0,0) is top-left.
type Frame struct {
	Rows, Cols  int
	cells       []engine.Cell // len == Rows*Cols, row-major
	CursorRow   int
	CursorCol   int
	CursorShown bool
}

// NewFrame returns a Frame of the given size with all-blank cells (rune 0).
func NewFrame(rows, cols int) *Frame {
	if rows < 0 {
		rows = 0
	}
	if cols < 0 {
		cols = 0
	}
	return &Frame{Rows: rows, Cols: cols, cells: make([]engine.Cell, rows*cols), CursorShown: true}
}

// At returns the cell at (row, col), or a zero Cell if out of range.
func (f *Frame) At(row, col int) engine.Cell {
	if row < 0 || row >= f.Rows || col < 0 || col >= f.Cols {
		return engine.Cell{}
	}
	return f.cells[row*f.Cols+col]
}

// Set writes a cell at (row, col). Out-of-range writes are ignored.
func (f *Frame) Set(row, col int, c engine.Cell) {
	if row < 0 || row >= f.Rows || col < 0 || col >= f.Cols {
		return
	}
	f.cells[row*f.Cols+col] = c
}

// SameSize reports whether two frames have identical dimensions.
func (f *Frame) SameSize(other *Frame) bool {
	return other != nil && f.Rows == other.Rows && f.Cols == other.Cols
}
