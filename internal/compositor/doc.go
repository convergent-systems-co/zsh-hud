// Package compositor assembles a render.Frame for the whole screen: a frozen
// top-bar row, the shell view (live or scrolled into scrollback) in the middle,
// and a frozen bottom-bar row. It is pure: the shell content arrives through the
// GridSource interface (satisfied by *engine.Engine), so no cgo is pulled in.
package compositor
