package input

import (
	"strconv"
	"strings"
)

// Mouse-tracking enable/disable escapes for main to write on startup/shutdown:
// SGR extended mouse mode (1006) + any-event tracking (1003). Exposed so the
// event loop owns terminal setup.
const (
	EnableMouse  = "\x1b[?1003h\x1b[?1006h"
	DisableMouse = "\x1b[?1006l\x1b[?1003l"
)

// mouseButton extracts the button code from SGR mouse params "<button;col;row".
// Returns -1 if the params are not an SGR mouse report.
func mouseButton(params string) int {
	if len(params) == 0 || params[0] != '<' {
		return -1
	}
	rest := params[1:]
	semi := strings.IndexByte(rest, ';')
	if semi < 0 {
		return -1
	}
	n, err := strconv.Atoi(rest[:semi])
	if err != nil {
		return -1
	}
	return n
}

func isWheelUp(params string) bool   { return mouseButton(params) == 64 }
func isWheelDown(params string) bool { return mouseButton(params) == 65 }
