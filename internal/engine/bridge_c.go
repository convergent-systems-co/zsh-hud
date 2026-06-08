package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>
#include <stdint.h>

// Forward declarations for the Go-exported callbacks so the trampolines below
// can call them. These match the signatures cgo generates for //export functions.
extern int goSBPushline(int cols, VTermScreenCell *cells, void *user);
extern int goSBPopline(int cols, VTermScreenCell *cells, void *user);

// C trampolines: libvterm calls these; they forward to the Go side.
static int cSBPushline(int cols, const VTermScreenCell *cells, void *user) {
	// Cast is safe: goSBPushline only reads cells; it never writes through this pointer.
	return goSBPushline(cols, (VTermScreenCell *)cells, user);
}
static int cSBPopline(int cols, VTermScreenCell *cells, void *user) {
	return goSBPopline(cols, cells, user);
}

// One static callbacks struct; only scrollback hooks are populated.
static VTermScreenCallbacks sbCallbacks;

// registerScrollback accepts the handle as a uintptr_t and reinterprets it as
// void* entirely within C, bypassing Go's checkptr validation which rejects
// unsafe.Pointer(uintptr(handle)) when the uintptr is not a real Go pointer.
static void registerScrollback(VTermScreen *screen, uintptr_t handle) {
	sbCallbacks.sb_pushline = cSBPushline;
	sbCallbacks.sb_popline  = cSBPopline;
	vterm_screen_set_callbacks(screen, &sbCallbacks, (void *)handle);
}

// Helpers to read/write a cells[] array element without pointer arithmetic in Go.
static uint32_t cellsChar0(const VTermScreenCell *cells, int i) { return cells[i].chars[0]; }
static void     cellSetChar0(VTermScreenCell *cells, int i, uint32_t ch) {
	cells[i].chars[0] = ch;
	cells[i].width    = 1;
}
*/
import "C"

import (
	"runtime/cgo"
)

// registerScrollbackCallbacks wires the libvterm sb_pushline/sb_popline
// callbacks to the engine. Called from New after the Engine is allocated so
// the cgo handle is valid before any screen output is processed.
//
// The handle is passed to C as a uintptr_t (not void*) to avoid Go's checkptr
// validator rejecting the uintptr→unsafe.Pointer cast on the Go side. The C
// helper performs the reinterpret cast internally, which is well-defined in C
// when sizeof(uintptr_t) == sizeof(void*).
func registerScrollbackCallbacks(e *Engine) {
	e.handle = cgo.NewHandle(e)
	C.registerScrollback(e.screen, C.uintptr_t(e.handle))
}

// sbCellsChar0 and sbCellSetChar0 are thin Go wrappers around the C helpers,
// called from the //export functions in bridge.go which cannot define C code.
func sbCellsChar0(cells *C.VTermScreenCell, i int) rune {
	return rune(C.cellsChar0(cells, C.int(i)))
}

func sbCellSetChar0(cells *C.VTermScreenCell, i int, ch rune) {
	C.cellSetChar0(cells, C.int(i), C.uint32_t(ch))
}
