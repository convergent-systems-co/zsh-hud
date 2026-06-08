// Package ptyhost opens a pseudo-terminal, spawns a child process (the user's
// shell in production), and exposes the child's I/O as an io.Reader/io.Writer
// plus resize, wait, and close. It does not depend on the engine: main wires
// the reader to engine.Write and forwards input through Write.
package ptyhost
