package render

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestEncodeStyleDefault(t *testing.T) {
	got := encodeStyle(engine.Color{IsDefault: true}, engine.Color{IsDefault: true}, 0)
	if got != "\x1b[0;39;49m" {
		t.Fatalf("default style = %q, want reset+default fg/bg", got)
	}
}

func TestEncodeStyleIndexedBasicAndBright(t *testing.T) {
	got := encodeStyle(engine.Color{IsIndexed: true, Index: 2}, engine.Color{IsIndexed: true, Index: 9}, 0)
	if got != "\x1b[0;32;101m" {
		t.Fatalf("indexed style = %q, want 0;32;101", got)
	}
}

func TestEncodeStyle256AndRGB(t *testing.T) {
	fg := engine.Color{IsIndexed: true, Index: 200}
	bg := engine.Color{R: 10, G: 20, B: 30}
	got := encodeStyle(fg, bg, 0)
	if got != "\x1b[0;38;5;200;48;2;10;20;30m" {
		t.Fatalf("256/rgb style = %q", got)
	}
}

func TestEncodeStyleAttrs(t *testing.T) {
	got := encodeStyle(engine.Color{IsDefault: true}, engine.Color{IsDefault: true}, engine.AttrBold|engine.AttrUnderline)
	if got != "\x1b[0;1;4;39;49m" {
		t.Fatalf("attr style = %q, want bold+underline", got)
	}
}

func TestCursorTo(t *testing.T) {
	if got := cursorTo(0, 0); got != "\x1b[1;1H" {
		t.Fatalf("cursorTo(0,0) = %q, want 1;1H", got)
	}
	if got := cursorTo(4, 9); got != "\x1b[5;10H" {
		t.Fatalf("cursorTo(4,9) = %q, want 5;10H", got)
	}
}
