package hud

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestNamedColorsAreIndexed(t *testing.T) {
	cases := map[engine.Color]uint8{
		ColorWhite: 7, ColorGreen: 2, ColorYellow: 3, ColorCyan: 6,
		ColorBlue: 4, ColorRed: 1, ColorDim: 8,
	}
	for c, idx := range cases {
		if !c.IsIndexed || c.Index != idx {
			t.Fatalf("color %+v: want indexed %d", c, idx)
		}
	}
}
