package engine

// Attr is a bitset of character attributes parsed from the shell's output.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrUnderline
	AttrItalic
	AttrReverse
	AttrStrike
	AttrBlink
)

// Has reports whether all bits in mask are set.
func (a Attr) Has(mask Attr) bool { return a&mask == mask }

// Color is a terminal color. Exactly one representation is meaningful, chosen
// by the flags: IsDefault (use terminal default), else IsIndexed (Index into
// the 256-color palette), else RGB.
type Color struct {
	R, G, B   uint8
	Index     uint8
	IsDefault bool
	IsIndexed bool
}

// Cell is one grid cell: its primary rune plus styling. Rune is 0 for an empty
// cell. Wide-character continuation cells have Rune == 0 and are skipped by
// readers using Width on the lead cell.
type Cell struct {
	Rune  rune
	Width int
	FG    Color
	BG    Color
	Attrs Attr
}
