// Package engine wraps libvterm (a C99 VT state engine) behind a small Go
// interface: feed shell output bytes in, read the parsed grid out, and keep a
// bounded scrollback ring buffer of lines that scroll off the top.
//
// libvterm is linked statically/dynamically via cgo + pkg-config (vterm).
package engine
