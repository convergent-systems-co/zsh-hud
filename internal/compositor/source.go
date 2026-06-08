package compositor

import "terminal-hud/internal/engine"

// GridSource is the read-only view of the shell's screen + scrollback that the
// compositor needs. *engine.Engine satisfies it. ScrollbackLine(0) is the
// newest line that scrolled off the top of the screen.
type GridSource interface {
	Size() (rows, cols int)
	Cell(row, col int) engine.Cell
	CursorPos() (row, col int)
	ScrollbackLen() int
	ScrollbackLine(n int) []engine.Cell
}

// padOrClip returns a slice exactly width long: cells beyond width are dropped;
// a short input is padded with blank cells (rune 0). Returns an empty slice for
// width <= 0.
func padOrClip(cells []engine.Cell, width int) []engine.Cell {
	if width <= 0 {
		return []engine.Cell{}
	}
	out := make([]engine.Cell, width)
	copy(out, cells) // copies min(len(cells), width); remainder stays blank
	return out
}

// Compile-time check that the real engine satisfies GridSource.
var _ GridSource = (*engine.Engine)(nil)
