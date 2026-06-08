package compositor

import (
	"terminal-hud/internal/engine"
	"terminal-hud/internal/render"
)

// Compose builds a Frame of rows x cols. Row 0 is the top bar, row rows-1 the
// bottom bar, and rows 1..rows-2 the shell view. scrollOffset scrolls the middle
// back into scrollback: 0 shows the live screen at the bottom; k shows the
// content k lines higher. scrollOffset is clamped to [0, ScrollbackLen].
//
// Bars are clipped/padded to cols. The cursor is mapped from the engine's screen
// position into the middle region and shown only in the live view (scrollOffset
// == 0); when scrolled back it is hidden.
func Compose(rows, cols int, src GridSource, scrollOffset int, topBar, bottomBar []engine.Cell) *render.Frame {
	f := render.NewFrame(rows, cols)
	if rows >= 1 {
		writeRow(f, 0, padOrClip(topBar, cols))
	}
	if rows >= 2 {
		writeRow(f, rows-1, padOrClip(bottomBar, cols))
	}

	midHeight := rows - 2
	if midHeight <= 0 {
		f.CursorShown = false
		return f
	}

	sb := src.ScrollbackLen()
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > sb {
		scrollOffset = sb
	}
	screenRows, _ := src.Size()

	for i := 0; i < midHeight; i++ {
		frameRow := 1 + i
		virtual := (sb - scrollOffset) + i
		switch {
		case virtual < 0:
			// above oldest history — leave blank
		case virtual < sb:
			writeRow(f, frameRow, padOrClip(src.ScrollbackLine(sb-1-virtual), cols))
		default:
			screenRow := virtual - sb
			if screenRow < screenRows {
				for c := 0; c < cols; c++ {
					f.Set(frameRow, c, src.Cell(screenRow, c))
				}
			}
		}
	}

	if scrollOffset == 0 {
		cr, cc := src.CursorPos()
		f.CursorRow = 1 + cr
		f.CursorCol = cc
		f.CursorShown = true
	} else {
		f.CursorShown = false
	}
	return f
}

// writeRow places a row of cells (already sized to cols) at frame row r.
func writeRow(f *render.Frame, r int, cells []engine.Cell) {
	for c := 0; c < len(cells); c++ {
		f.Set(r, c, cells[c])
	}
}
