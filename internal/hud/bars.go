package hud

import "terminal-hud/internal/engine"

// Deps holds the already-computed segment strings for one frame's bars. main's
// refresh goroutine fills these from the module functions; the bars are
// assembled synchronously on the render path (cheap string->cell work).
type Deps struct {
	// top bar
	Time    string
	LocalIP string
	ExtIP   string
	Weather string
	// bottom bar (path and exit are deferred until OSC support lands)
	Git   string
	Azure string
	K8s   string
}

// TopBar lays out time | localip | extip on the left and weather on the right.
func TopBar(width int, d Deps) []engine.Cell {
	left := []Segment{
		{Text: d.Time, Color: ColorWhite},
		{Text: d.LocalIP, Color: ColorGreen},
		{Text: d.ExtIP, Color: ColorYellow},
	}
	right := []Segment{{Text: d.Weather, Color: ColorCyan}}
	return AssembleBar(width, left, right)
}

// BottomBar lays out git | azure | k8s on the left.
func BottomBar(width int, d Deps) []engine.Cell {
	left := []Segment{
		{Text: d.Git, Color: ColorWhite},
		{Text: d.Azure, Color: ColorBlue},
		{Text: d.K8s, Color: ColorCyan},
	}
	return AssembleBar(width, left, nil)
}
