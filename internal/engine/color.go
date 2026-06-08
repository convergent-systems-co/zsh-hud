package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>

static int color_is_default_fg(VTermColor *c) { return VTERM_COLOR_IS_DEFAULT_FG(c); }
static int color_is_default_bg(VTermColor *c) { return VTERM_COLOR_IS_DEFAULT_BG(c); }
static int color_is_indexed(VTermColor *c)    { return VTERM_COLOR_IS_INDEXED(c); }
static unsigned char color_index(VTermColor *c){ return c->indexed.idx; }
static unsigned char color_r(VTermColor *c)    { return c->rgb.red; }
static unsigned char color_g(VTermColor *c)    { return c->rgb.green; }
static unsigned char color_b(VTermColor *c)    { return c->rgb.blue; }

static int attr_bold(VTermScreenCell *c)      { return c->attrs.bold; }
static int attr_underline(VTermScreenCell *c) { return c->attrs.underline; }
static int attr_italic(VTermScreenCell *c)    { return c->attrs.italic; }
static int attr_reverse(VTermScreenCell *c)   { return c->attrs.reverse; }
static int attr_strike(VTermScreenCell *c)    { return c->attrs.strike; }
static int attr_blink(VTermScreenCell *c)     { return c->attrs.blink; }
*/
import "C"

// mapColor converts a libvterm VTermColor to our Color. isFG selects which
// "default" flag to honor (fg vs bg use different default bits).
func mapColor(c *C.VTermColor, isFG bool) Color {
	var def C.int
	if isFG {
		def = C.color_is_default_fg(c)
	} else {
		def = C.color_is_default_bg(c)
	}
	if def != 0 {
		return Color{IsDefault: true}
	}
	if C.color_is_indexed(c) != 0 {
		return Color{IsIndexed: true, Index: uint8(C.color_index(c))}
	}
	return Color{R: uint8(C.color_r(c)), G: uint8(C.color_g(c)), B: uint8(C.color_b(c))}
}

// mapAttrs extracts the character-attribute bitset from a libvterm cell.
// Note: VTermScreenCellAttrs.underline is a 2-bit enum (VTERM_UNDERLINE_*),
// so the shim's `!= 0` test correctly means "any underline style present".
func mapAttrs(c *C.VTermScreenCell) Attr {
	var a Attr
	if C.attr_bold(c) != 0 {
		a |= AttrBold
	}
	if C.attr_underline(c) != 0 {
		a |= AttrUnderline
	}
	if C.attr_italic(c) != 0 {
		a |= AttrItalic
	}
	if C.attr_reverse(c) != 0 {
		a |= AttrReverse
	}
	if C.attr_strike(c) != 0 {
		a |= AttrStrike
	}
	if C.attr_blink(c) != 0 {
		a |= AttrBlink
	}
	return a
}
