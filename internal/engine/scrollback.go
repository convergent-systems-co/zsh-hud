package engine

// scrollback is a bounded FIFO of lines that have scrolled off the top of the
// screen. push appends the newest line; when full, the oldest is evicted.
// line(0) is the newest pushed-off line (the one just above the screen);
// higher indices go further back in history. pop removes and returns the newest
// (used when the screen scrolls back down and libvterm asks for a line).
//
// Pure Go, no cgo: cells are copied in by the caller before crossing here.
type scrollback struct {
	lines [][]Cell
	cap   int
}

func newScrollback(capLines int) *scrollback {
	if capLines < 1 {
		capLines = 1
	}
	return &scrollback{cap: capLines}
}

func (s *scrollback) push(cells []Cell) {
	s.lines = append(s.lines, cells)
	if len(s.lines) > s.cap {
		// evict oldest; copy down to avoid unbounded slice growth.
		copy(s.lines, s.lines[len(s.lines)-s.cap:])
		s.lines = s.lines[:s.cap]
	}
}

func (s *scrollback) pop() ([]Cell, bool) {
	if len(s.lines) == 0 {
		return nil, false
	}
	last := s.lines[len(s.lines)-1]
	s.lines = s.lines[:len(s.lines)-1]
	return last, true
}

func (s *scrollback) len() int { return len(s.lines) }

// line returns the n-th line back from the screen (0 = newest pushed-off).
// Out-of-range returns nil.
func (s *scrollback) line(n int) []Cell {
	if n < 0 || n >= len(s.lines) {
		return nil
	}
	return s.lines[len(s.lines)-1-n]
}
