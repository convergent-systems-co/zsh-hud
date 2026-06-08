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
	e := cgo.Handle(uintptr(user)).Value().(*Engine)
	line := make([]Cell, int(cols))
	for i := 0; i < int(cols); i++ {
		line[i] = Cell{Rune: sbCellsChar0(cells, i), Width: 1}
	}
	e.sb.push(line)
	return 1
}

//export goSBPopline
func goSBPopline(cols C.int, cells *C.VTermScreenCell, user unsafe.Pointer) C.int {
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
