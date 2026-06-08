package hud

import "terminal-hud/internal/engine"

const separator = " | " // dim-colored, placed between non-empty segments

// AssembleBar lays out left segments (joined by a dim separator) from column 0
// and right segments (joined likewise) flush against the last column, over a
// row of exactly width cells. Empty segments are skipped (no stray separators).
// Content is clipped to width; right cells that would collide with the left run
// are dropped.
func AssembleBar(width int, left, right []Segment) []engine.Cell {
	if width < 0 {
		width = 0
	}
	row := make([]engine.Cell, width)

	col := 0
	writeJoined(row, &col, left)

	// Build right run into scratch, then place flush-right without overwriting
	// the left run.
	scratch := make([]engine.Cell, width)
	rcol := 0
	writeJoined(scratch, &rcol, right)
	rcells := scratch[:rcol]
	start := width - len(rcells)
	for i := 0; i < len(rcells); i++ {
		pos := start + i
		if pos >= 0 && pos < width && pos >= col {
			row[pos] = rcells[i]
		}
	}
	return row
}

// writeJoined writes each non-empty segment's runes into row from *col, with a
// dim separator between consecutive non-empty segments, clipping at len(row).
func writeJoined(row []engine.Cell, col *int, segs []Segment) {
	wroteOne := false
	for _, s := range segs {
		if s.Text == "" {
			continue
		}
		if wroteOne {
			putRunes(row, col, separator, ColorDim)
		}
		putRunes(row, col, s.Text, s.Color)
		wroteOne = true
	}
}

// putRunes writes s's runes into row from *col with color, advancing *col, up
// to len(row).
func putRunes(row []engine.Cell, col *int, s string, color engine.Color) {
	for _, r := range s {
		if *col >= len(row) {
			return
		}
		row[*col] = engine.Cell{Rune: r, Width: 1, FG: color}
		*col++
	}
}
