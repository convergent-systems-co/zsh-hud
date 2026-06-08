package render

import (
	"strconv"
	"strings"

	"terminal-hud/internal/engine"
)

// ANSI control prefixes.
const (
	csi   = "\x1b["
	clear = "\x1b[2J" // erase entire screen
)

// cursorTo returns the escape to move the cursor to (row, col), 0-based input,
// emitted as 1-based CSI H.
func cursorTo(row, col int) string {
	return csi + strconv.Itoa(row+1) + ";" + strconv.Itoa(col+1) + "H"
}

// encodeStyle returns a full SGR sequence that resets then sets attrs, fg, bg.
// Always self-contained (leads with reset 0) so it can be emitted standalone.
func encodeStyle(fg, bg engine.Color, attrs engine.Attr) string {
	params := []string{"0"}
	if attrs.Has(engine.AttrBold) {
		params = append(params, "1")
	}
	if attrs.Has(engine.AttrItalic) {
		params = append(params, "3")
	}
	if attrs.Has(engine.AttrUnderline) {
		params = append(params, "4")
	}
	if attrs.Has(engine.AttrBlink) {
		params = append(params, "5")
	}
	if attrs.Has(engine.AttrReverse) {
		params = append(params, "7")
	}
	if attrs.Has(engine.AttrStrike) {
		params = append(params, "9")
	}
	params = append(params, colorParams(fg, true)...)
	params = append(params, colorParams(bg, false)...)
	return csi + strings.Join(params, ";") + "m"
}

// colorParams returns the SGR parameters for one color. fg selects the
// foreground code family (30s/90s/38) vs background (40s/100s/48).
func colorParams(c engine.Color, fg bool) []string {
	base := 30
	if !fg {
		base = 40
	}
	switch {
	case c.IsDefault:
		return []string{strconv.Itoa(base + 9)}
	case c.IsIndexed && c.Index < 8:
		return []string{strconv.Itoa(base + int(c.Index))}
	case c.IsIndexed && c.Index < 16:
		brightBase := 90
		if !fg {
			brightBase = 100
		}
		return []string{strconv.Itoa(brightBase + int(c.Index) - 8)}
	case c.IsIndexed:
		return []string{strconv.Itoa(base + 8), "5", strconv.Itoa(int(c.Index))}
	default:
		return []string{strconv.Itoa(base + 8), "2",
			strconv.Itoa(int(c.R)), strconv.Itoa(int(c.G)), strconv.Itoa(int(c.B))}
	}
}
