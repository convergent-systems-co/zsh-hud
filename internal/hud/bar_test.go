package hud

import (
	"testing"

	"terminal-hud/internal/engine"
)

func barText(cells []engine.Cell) string {
	r := make([]rune, len(cells))
	for i, c := range cells {
		if c.Rune == 0 {
			r[i] = ' '
		} else {
			r[i] = c.Rune
		}
	}
	return string(r)
}

func TestAssembleBarJoinsLeftWithSeparator(t *testing.T) {
	left := []Segment{{Text: "AA", Color: ColorWhite}, {Text: "BB", Color: ColorGreen}}
	cells := AssembleBar(20, left, nil)
	if len(cells) != 20 {
		t.Fatalf("len = %d, want 20", len(cells))
	}
	if got := barText(cells); got != "AA | BB             " {
		t.Fatalf("bar = %q", got)
	}
	if cells[0].FG != ColorWhite || cells[5].FG != ColorGreen {
		t.Fatalf("segment colors not applied: %+v %+v", cells[0].FG, cells[5].FG)
	}
}

func TestAssembleBarRightAlignsRightSegments(t *testing.T) {
	left := []Segment{{Text: "L", Color: ColorWhite}}
	right := []Segment{{Text: "R", Color: ColorCyan}}
	cells := AssembleBar(5, left, right)
	if got := barText(cells); got != "L   R" {
		t.Fatalf("bar = %q, want 'L   R'", got)
	}
	if cells[4].FG != ColorCyan {
		t.Fatalf("right segment color not applied")
	}
}

func TestAssembleBarClipsToWidth(t *testing.T) {
	left := []Segment{{Text: "0123456789", Color: ColorWhite}}
	cells := AssembleBar(4, left, nil)
	if got := barText(cells); got != "0123" {
		t.Fatalf("bar = %q, want '0123'", got)
	}
}

func TestAssembleBarSkipsEmptySegments(t *testing.T) {
	left := []Segment{{Text: "A", Color: ColorWhite}, {Text: "", Color: ColorGreen}, {Text: "B", Color: ColorYellow}}
	if got := barText(AssembleBar(10, left, nil)); got != "A | B     " {
		t.Fatalf("bar = %q, want 'A | B     '", got)
	}
}
