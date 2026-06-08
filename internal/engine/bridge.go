// Package bridge note: this file contains the //export'd Go functions called
// from C via the trampolines in bridge_c.go. Cgo requires that any file with
// //export directives must NOT define C code in its preamble — that is why the
// C definitions live in bridge_c.go and this file's preamble only declares them.
package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

//export goSBPushline
func goSBPushline(cols C.int, cells *C.VTermScreenCell, user unsafe.Pointer) C.int {
	// Safe: the handle was created in registerScrollbackCallbacks with this
	// engine as its value, libvterm returns the same user pointer verbatim, and
	// Close deletes the handle only after no further callbacks can fire.
	e := cgo.Handle(uintptr(user)).Value().(*Engine)
	line := make([]Cell, int(cols))
	for i := 0; i < int(cols); i++ {
		// TODO: only the base codepoint (chars[0]) is captured. Combining chars
		// (chars[1..5]), cell width (CJK wide glyphs), color, and attrs are
		// dropped. Acceptable for early scrollback; fix before rendering non-ASCII.
		line[i] = Cell{Rune: sbCellsChar0(cells, i), Width: 1}
	}
	e.sb.push(line)
	return 1
}

//export goSBPopline
func goSBPopline(cols C.int, cells *C.VTermScreenCell, user unsafe.Pointer) C.int {
	// Safe: the handle was created in registerScrollbackCallbacks with this
	// engine as its value, libvterm returns the same user pointer verbatim, and
	// Close deletes the handle only after no further callbacks can fire.
	e := cgo.Handle(uintptr(user)).Value().(*Engine)
	line, ok := e.sb.pop()
	if !ok {
		return 0
	}
	for i := 0; i < int(cols) && i < len(line); i++ {
		sbCellSetChar0(cells, i, line[i].Rune)
	}
	return 1
}
