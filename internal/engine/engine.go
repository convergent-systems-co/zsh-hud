package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>
#include <stdlib.h>
#include <string.h>

static VTermScreenCell *new_cell() {
	VTermScreenCell *c = malloc(sizeof(VTermScreenCell));
	memset(c, 0, sizeof(VTermScreenCell));
	return c;
}
static uint32_t cell_char0(VTermScreenCell *c) { return c->chars[0]; }
static int      cell_width(VTermScreenCell *c) { return c->width; }
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// scrollback is a placeholder until Task 4 provides the real ring buffer.
// TEMP: replaced by scrollback.go in the next task.
type scrollback struct{}

func newScrollback(int) *scrollback { return &scrollback{} }

// Engine wraps a libvterm Terminal + Screen and a scrollback ring buffer.
// Not safe for concurrent use; the owner must serialize calls (the event loop
// does this).
type Engine struct {
	vt     *C.VTerm
	screen *C.VTermScreen
	rows   int
	cols   int
	sb     *scrollback
	handle cgo.Handle
}

// New creates an engine with a rows x cols grid and a scrollback cap of
// sbLines lines. Returns an error if libvterm allocation fails.
func New(rows, cols, sbLines int) (*Engine, error) {
	vt := C.vterm_new(C.int(rows), C.int(cols))
	if vt == nil {
		return nil, fmt.Errorf("engine: vterm_new returned nil")
	}
	C.vterm_set_utf8(vt, 1)
	screen := C.vterm_obtain_screen(vt)
	if screen == nil {
		C.vterm_free(vt)
		return nil, fmt.Errorf("engine: vterm_obtain_screen returned nil")
	}
	C.vterm_screen_reset(screen, 1)
	C.vterm_screen_enable_altscreen(screen, 1)

	e := &Engine{vt: vt, screen: screen, rows: rows, cols: cols, sb: newScrollback(sbLines)}
	return e, nil
}

// Write feeds shell output bytes into the parser. Implements io.Writer.
func (e *Engine) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	cb := C.CBytes(p)
	defer C.free(cb)
	n := C.vterm_input_write(e.vt, (*C.char)(cb), C.size_t(len(p)))
	return int(n), nil
}

// Cell returns the visible grid cell at (row, col). Out-of-range returns a zero
// Cell.
func (e *Engine) Cell(row, col int) Cell {
	if row < 0 || row >= e.rows || col < 0 || col >= e.cols {
		return Cell{}
	}
	var pos C.VTermPos
	pos.row = C.int(row)
	pos.col = C.int(col)
	cell := C.new_cell()
	defer C.free(unsafe.Pointer(cell))
	if C.vterm_screen_get_cell(e.screen, pos, cell) == 0 {
		return Cell{}
	}
	return Cell{
		Rune:  rune(C.cell_char0(cell)),
		Width: int(C.cell_width(cell)),
		FG:    mapColor(&cell.fg, true),
		BG:    mapColor(&cell.bg, false),
		Attrs: mapAttrs(cell),
	}
}

// CursorPos returns the current cursor row, col.
func (e *Engine) CursorPos() (int, int) {
	var pos C.VTermPos
	C.vterm_state_get_cursorpos(C.vterm_obtain_state(e.vt), &pos)
	return int(pos.row), int(pos.col)
}

// Size returns the current grid dimensions.
func (e *Engine) Size() (int, int) { return e.rows, e.cols }

// Resize changes the grid dimensions (SIGWINCH). Active screen reflows;
// scrollback history does not (libvterm limitation).
func (e *Engine) Resize(rows, cols int) {
	C.vterm_set_size(e.vt, C.int(rows), C.int(cols))
	e.rows, e.cols = rows, cols
}

// Close frees the underlying libvterm Terminal and releases the cgo handle.
func (e *Engine) Close() error {
	if e.handle != 0 {
		e.handle.Delete()
		e.handle = 0
	}
	if e.vt != nil {
		C.vterm_free(e.vt)
		e.vt = nil
	}
	return nil
}
