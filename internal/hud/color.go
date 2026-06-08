package hud

import "terminal-hud/internal/engine"

// Named-ANSI role colors for bar chrome (indices 0-7 + bright). The host
// terminal theme maps these to actual RGB, per the SPEC color policy.
var (
	ColorRed    = engine.Color{IsIndexed: true, Index: 1}
	ColorGreen  = engine.Color{IsIndexed: true, Index: 2}
	ColorYellow = engine.Color{IsIndexed: true, Index: 3}
	ColorBlue   = engine.Color{IsIndexed: true, Index: 4}
	ColorCyan   = engine.Color{IsIndexed: true, Index: 6}
	ColorWhite  = engine.Color{IsIndexed: true, Index: 7}
	ColorDim    = engine.Color{IsIndexed: true, Index: 8} // bright black (separators)
)

// Segment is a piece of bar text with a single color. Empty Text is omitted.
type Segment struct {
	Text  string
	Color engine.Color
}
