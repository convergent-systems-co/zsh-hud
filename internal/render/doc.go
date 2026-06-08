// Package render draws a composited Frame (a grid of cells plus a cursor) to an
// io.Writer, diffing against the previously drawn frame so only changed cells
// are emitted. It is pure: no terminal setup, no cgo. main supplies the writer
// (os.Stdout) and owns raw-mode/alt-screen; the compositor supplies the frames.
package render
