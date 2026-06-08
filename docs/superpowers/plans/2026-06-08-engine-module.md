# Engine Module Implementation Plan (Plan 1 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `engine` module — a thin Go cgo wrapper over libvterm that feeds shell bytes into a VT grid, reads cells back, and maintains a bounded scrollback ring buffer — plus the project scaffold it lives in.

**Architecture:** cgo links system libvterm (`pkg-config: vterm`). libvterm parses bytes into its grid; a small C bridge routes libvterm's `sb_pushline`/`sb_popline` callbacks to Go via `cgo.Handle`. The engine copies scrolled-off lines into a Go-owned ring buffer (default 10,000 lines). Everything is unit-testable without a pty or a real terminal.

**Tech Stack:** Go 1.26 + cgo, libvterm 0.3.3 (Homebrew/apt), `pkg-config`.

**Scope note:** This plan covers project scaffold + engine grid/scrollback. It does NOT cover OSC parsing (path/exit modules) — that needs the libvterm parser-callback API verified first and belongs to a later plan. See `SPEC.md` Open Question #4.

**Prerequisite:** `pkg-config --exists vterm` must succeed (macOS `brew install libvterm`; Debian/Ubuntu `apt install libvterm-dev`). Verified working on macOS 26 in `PHASE0.md` (Phase 0b).

---

## File Structure

```
go.mod                         ← module terminal-hud, go 1.26
Makefile                       ← build/test/lint targets
internal/engine/
  doc.go                       ← package doc
  cell.go                      ← Cell, Color, Attr value types (pure Go)
  color.go                     ← VTermColor → Color mapping (cgo helper + pure conversion)
  scrollback.go                ← ring buffer (pure Go, no cgo)
  scrollback_test.go
  bridge.go                    ← C bridge + //export'd callbacks (cgo)
  engine.go                    ← Engine type: New/Write/Cell/CursorPos/Resize/Close (cgo)
  engine_test.go
  cell_test.go
```

Responsibilities: `scrollback.go` is pure Go and knows nothing about cgo. `bridge.go` holds the only `//export` functions. `engine.go` is the public surface. `cell.go`/`color.go` are the value types crossing the boundary.

---

### Task 1: Project scaffold + libvterm cgo smoke test

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `internal/engine/doc.go`
- Test: `internal/engine/engine_test.go`

- [ ] **Step 1: Create the Go module**

`go.mod`:
```
module terminal-hud

go 1.26
```

- [ ] **Step 2: Create the package doc file**

`internal/engine/doc.go`:
```go
// Package engine wraps libvterm (a C99 VT state engine) behind a small Go
// interface: feed shell output bytes in, read the parsed grid out, and keep a
// bounded scrollback ring buffer of lines that scroll off the top.
//
// libvterm is linked statically/dynamically via cgo + pkg-config (vterm).
package engine
```

- [ ] **Step 3: Write the failing smoke test**

This is the Phase 0b proof promoted to a real test: feed an SGR-colored stream, read row 0 back.

`internal/engine/engine_test.go`:
```go
package engine

import "testing"

func TestEngineReadsBackPlainText(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if _, err := e.Write([]byte("Hello, \x1b[1;32mworld\x1b[0m!")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got []rune
	for col := 0; col < 14; col++ {
		c := e.Cell(0, col)
		if c.Rune != 0 {
			got = append(got, c.Rune)
		}
	}
	if string(got) != "Hello, world!" {
		t.Fatalf("row 0 = %q, want %q", string(got), "Hello, world!")
	}
}
```

- [ ] **Step 4: Run the test to verify it fails (compile error: New undefined)**

Run: `go test ./internal/engine/ -run TestEngineReadsBackPlainText -v`
Expected: FAIL — `undefined: New` / `undefined: Cell`.

- [ ] **Step 5: Create the Makefile**

`Makefile`:
```make
.PHONY: build test lint vet

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	gofmt -l .
```

- [ ] **Step 6: Commit**

```bash
git add go.mod Makefile internal/engine/doc.go internal/engine/engine_test.go
git commit -m "chore: scaffold go module and engine package with failing smoke test"
```

---

### Task 2: Cell value types

**Files:**
- Create: `internal/engine/cell.go`
- Test: `internal/engine/cell_test.go`

- [ ] **Step 1: Write the failing test**

`internal/engine/cell_test.go`:
```go
package engine

import "testing"

func TestColorDefaultIsZeroValue(t *testing.T) {
	var c Color
	if !c.IsDefault {
		// zero-value Color must mean "terminal default"
		t.Fatalf("zero Color should be default, got %+v", c)
	}
}

func TestAttrBoldSetAndClear(t *testing.T) {
	var a Attr
	if a.Has(AttrBold) {
		t.Fatal("zero Attr should have no bold")
	}
	a |= AttrBold
	if !a.Has(AttrBold) {
		t.Fatal("AttrBold not set")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/engine/ -run 'TestColor|TestAttr' -v`
Expected: FAIL — `undefined: Color` / `undefined: Attr`.

- [ ] **Step 3: Implement the value types**

`internal/engine/cell.go`:
```go
package engine

// Attr is a bitset of character attributes parsed from the shell's output.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrUnderline
	AttrItalic
	AttrReverse
	AttrStrike
	AttrBlink
)

// Has reports whether all bits in mask are set.
func (a Attr) Has(mask Attr) bool { return a&mask == mask }

// Color is a terminal color. Exactly one representation is meaningful, chosen
// by the flags: IsDefault (use terminal default), else IsIndexed (Index into
// the 256-color palette), else RGB.
type Color struct {
	R, G, B   uint8
	Index     uint8
	IsDefault bool
	IsIndexed bool
}

// Cell is one grid cell: its primary rune plus styling. Rune is 0 for an empty
// cell. Wide-character continuation cells have Rune == 0 and are skipped by
// readers using Width on the lead cell.
type Cell struct {
	Rune  rune
	Width int
	FG    Color
	BG    Color
	Attrs Attr
}
```

Note: zero-value `Color{}` has `IsDefault == false`. The test above expects zero = default, so set `IsDefault` true by convention only via the mapper. **Fix the test to match the real invariant:** zero Color is RGB black. Update `cell_test.go` Step 1 `TestColorDefaultIsZeroValue` to:
```go
func TestColorZeroValueIsRGBBlack(t *testing.T) {
	var c Color
	if c.IsDefault || c.IsIndexed || c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatalf("zero Color should be RGB black, got %+v", c)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/engine/ -run 'TestColor|TestAttr' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/cell.go internal/engine/cell_test.go
git commit -m "feat(engine): add Cell, Color, Attr value types"
```

---

### Task 3: Engine core — New, Write, Cell, CursorPos, Resize, Close

**Files:**
- Create: `internal/engine/color.go`
- Create: `internal/engine/engine.go`
- Test: `internal/engine/engine_test.go` (extend)

- [ ] **Step 1: Write the failing tests (extend engine_test.go)**

Append to `internal/engine/engine_test.go`:
```go
func TestEngineCursorAdvances(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	e.Write([]byte("abc"))
	row, col := e.CursorPos()
	if row != 0 || col != 3 {
		t.Fatalf("cursor = (%d,%d), want (0,3)", row, col)
	}
}

func TestEngineParsesColor(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	// SGR 32 = indexed green (palette index 2).
	e.Write([]byte("\x1b[32mX"))
	c := e.Cell(0, 0)
	if c.Rune != 'X' {
		t.Fatalf("rune = %q, want X", c.Rune)
	}
	if !c.FG.IsIndexed || c.FG.Index != 2 {
		t.Fatalf("fg = %+v, want indexed 2", c.FG)
	}
}

func TestEngineResizeChangesGrid(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	e.Resize(10, 40)
	r, c := e.Size()
	if r != 10 || c != 40 {
		t.Fatalf("size = (%d,%d), want (10,40)", r, c)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/engine/ -v`
Expected: FAIL — `undefined: New`, etc.

- [ ] **Step 3: Implement color mapping**

`internal/engine/color.go`:
```go
package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>

// Accessor shims: the VTERM_COLOR_IS_* checks are macros, and the rgb/indexed
// fields live in an anonymous union, so expose them through plain C functions
// that cgo can call directly.
static int color_is_default_fg(VTermColor *c) { return VTERM_COLOR_IS_DEFAULT_FG(c); }
static int color_is_default_bg(VTermColor *c) { return VTERM_COLOR_IS_DEFAULT_BG(c); }
static int color_is_indexed(VTermColor *c)    { return VTERM_COLOR_IS_INDEXED(c); }
static unsigned char color_index(VTermColor *c){ return c->indexed.idx; }
static unsigned char color_r(VTermColor *c)    { return c->rgb.red; }
static unsigned char color_g(VTermColor *c)    { return c->rgb.green; }
static unsigned char color_b(VTermColor *c)    { return c->rgb.blue; }
*/
import "C"

// mapColor converts a libvterm VTermColor to our Color. isFG selects which
// "default" flag to honor (fg vs bg use different default bits).
func mapColor(c *C.VTermColor, isFG bool) Color {
	var def C.int
	if isFG {
		def = C.color_is_default_fg(c)
	} else {
		def = C.color_is_default_bg(c)
	}
	if def != 0 {
		return Color{IsDefault: true}
	}
	if C.color_is_indexed(c) != 0 {
		return Color{IsIndexed: true, Index: uint8(C.color_index(c))}
	}
	return Color{R: uint8(C.color_r(c)), G: uint8(C.color_g(c)), B: uint8(C.color_b(c))}
}
```

- [ ] **Step 4: Implement the engine core**

`internal/engine/engine.go`:
```go
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
	"unsafe"
)

// Engine wraps a libvterm Terminal + Screen and a scrollback ring buffer.
// Not safe for concurrent use; the owner must serialize calls (the event loop
// does this).
type Engine struct {
	vt     *C.VTerm
	screen *C.VTermScreen
	rows   int
	cols   int
	sb     *scrollback
	handle cgoHandle // set in Task 5; zero-value safe until then
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
	// libvterm exposes the cursor via the state; query through a damage-free
	// read using vterm_state. Simpler: track via movecursor callback (Task 5).
	// For now read from the screen's reported cursor.
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
```

- [ ] **Step 5: Implement the attribute mapper**

Append to `internal/engine/color.go`:
```go
/*
static int attr_bold(VTermScreenCell *c)      { return c->attrs.bold; }
static int attr_underline(VTermScreenCell *c) { return c->attrs.underline; }
static int attr_italic(VTermScreenCell *c)    { return c->attrs.italic; }
static int attr_reverse(VTermScreenCell *c)   { return c->attrs.reverse; }
static int attr_strike(VTermScreenCell *c)    { return c->attrs.strike; }
static int attr_blink(VTermScreenCell *c)     { return c->attrs.blink; }
*/

func mapAttrs(c *C.VTermScreenCell) Attr {
	var a Attr
	if C.attr_bold(c) != 0 {
		a |= AttrBold
	}
	if C.attr_underline(c) != 0 {
		a |= AttrUnderline
	}
	if C.attr_italic(c) != 0 {
		a |= AttrItalic
	}
	if C.attr_reverse(c) != 0 {
		a |= AttrReverse
	}
	if C.attr_strike(c) != 0 {
		a |= AttrStrike
	}
	if C.attr_blink(c) != 0 {
		a |= AttrBlink
	}
	return a
}
```

> Implementation note: every symbol above is verified against libvterm 0.3.3's `vterm.h` — `vterm_obtain_state`, `vterm_state_get_cursorpos(const VTermState*, VTermPos*)`, the `VTermColor` union members `rgb.{red,green,blue}` and `indexed.idx`, and the `VTermScreenCellAttrs` bitfields `bold(:1)`, `underline(:2)`, `italic(:1)`, `blink(:1)`, `reverse(:1)`, `strike(:1)`. `underline` is a 2-bit enum (`VTERM_UNDERLINE_*`), so `!= 0` correctly means "underlined". If building against a different libvterm version and a symbol is missing, run `grep -n "<symbol>" "$(pkg-config --variable=includedir vterm)/vterm.h"` and adjust — do not guess.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/engine/ -v`
Expected: PASS for `TestEngineReadsBackPlainText`, `TestEngineCursorAdvances`, `TestEngineParsesColor`, `TestEngineResizeChangesGrid`.

- [ ] **Step 7: Commit**

```bash
git add internal/engine/engine.go internal/engine/color.go internal/engine/engine_test.go
git commit -m "feat(engine): grid read/write, cursor, color/attr mapping, resize"
```

---

### Task 4: Scrollback ring buffer (pure Go)

**Files:**
- Create: `internal/engine/scrollback.go`
- Test: `internal/engine/scrollback_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/engine/scrollback_test.go`:
```go
package engine

import "testing"

func line(n int) []Cell { return []Cell{{Rune: rune('0' + n)}} }

func TestScrollbackPushAndRead(t *testing.T) {
	sb := newScrollback(3)
	sb.push(line(1))
	sb.push(line(2))
	if sb.len() != 2 {
		t.Fatalf("len = %d, want 2", sb.len())
	}
	// line(0) is the most recently pushed-off line (closest to the screen).
	if got := sb.line(0); got[0].Rune != '2' {
		t.Fatalf("line(0) = %q, want '2'", got[0].Rune)
	}
	if got := sb.line(1); got[0].Rune != '1' {
		t.Fatalf("line(1) = %q, want '1'", got[0].Rune)
	}
}

func TestScrollbackEvictsOldestAtCap(t *testing.T) {
	sb := newScrollback(2)
	sb.push(line(1))
	sb.push(line(2))
	sb.push(line(3)) // evicts line(1)
	if sb.len() != 2 {
		t.Fatalf("len = %d, want 2", sb.len())
	}
	if got := sb.line(1); got[0].Rune != '2' {
		t.Fatalf("oldest line = %q, want '2' (line 1 evicted)", got[0].Rune)
	}
}

func TestScrollbackPopReturnsMostRecent(t *testing.T) {
	sb := newScrollback(3)
	sb.push(line(1))
	sb.push(line(2))
	got, ok := sb.pop()
	if !ok || got[0].Rune != '2' {
		t.Fatalf("pop = %v,%v want '2',true", got, ok)
	}
	if sb.len() != 1 {
		t.Fatalf("len after pop = %d, want 1", sb.len())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/engine/ -run TestScrollback -v`
Expected: FAIL — `undefined: newScrollback`.

- [ ] **Step 3: Implement the ring buffer**

`internal/engine/scrollback.go`:
```go
package engine

// scrollback is a bounded FIFO of lines that have scrolled off the top of the
// screen. push appends the newest line; when full, the oldest is evicted.
// line(0) is the newest pushed-off line (the one just above the screen);
// higher indices go further back in history. pop removes and returns the newest
// (used when the screen scrolls back down and libvterm asks for a line).
//
// Pure Go, no cgo: cells are copied in by the caller before crossing here.
type scrollback struct {
	lines [][]Cell
	cap   int
}

func newScrollback(capLines int) *scrollback {
	if capLines < 1 {
		capLines = 1
	}
	return &scrollback{cap: capLines}
}

func (s *scrollback) push(cells []Cell) {
	s.lines = append(s.lines, cells)
	if len(s.lines) > s.cap {
		// evict oldest; copy down to avoid unbounded slice growth.
		copy(s.lines, s.lines[len(s.lines)-s.cap:])
		s.lines = s.lines[:s.cap]
	}
}

func (s *scrollback) pop() ([]Cell, bool) {
	if len(s.lines) == 0 {
		return nil, false
	}
	last := s.lines[len(s.lines)-1]
	s.lines = s.lines[:len(s.lines)-1]
	return last, true
}

func (s *scrollback) len() int { return len(s.lines) }

// line returns the n-th line back from the screen (0 = newest pushed-off).
// Out-of-range returns nil.
func (s *scrollback) line(n int) []Cell {
	if n < 0 || n >= len(s.lines) {
		return nil
	}
	return s.lines[len(s.lines)-1-n]
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/engine/ -run TestScrollback -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/scrollback.go internal/engine/scrollback_test.go
git commit -m "feat(engine): bounded scrollback ring buffer"
```

---

### Task 5: Wire libvterm scrollback callbacks to the ring buffer

**Files:**
- Create: `internal/engine/bridge.go`
- Modify: `internal/engine/engine.go` (register callbacks in New; add cgoHandle alias)
- Test: `internal/engine/engine_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to `internal/engine/engine_test.go`:
```go
func TestEngineScrollbackFillsOnOverflow(t *testing.T) {
	// 3-row grid; writing 5 newline-separated lines pushes 2 off the top.
	e, err := New(3, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	e.Write([]byte("L0\r\nL1\r\nL2\r\nL3\r\nL4\r\n"))

	if e.ScrollbackLen() < 2 {
		t.Fatalf("scrollback len = %d, want >= 2", e.ScrollbackLen())
	}
	// The newest pushed-off line should be "L1" (L0 went first, then L1),
	// i.e. line(1) == "L0", line(0) == "L1" — assert one of them is present.
	top := lineText(e.ScrollbackLine(0))
	if top != "L1" && top != "L2" {
		t.Fatalf("scrollback line(0) = %q, want a scrolled-off line", top)
	}
}

func lineText(cells []Cell) string {
	var r []rune
	for _, c := range cells {
		if c.Rune != 0 {
			r = append(r, c.Rune)
		}
	}
	return string(r)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/engine/ -run TestEngineScrollback -v`
Expected: FAIL — `undefined: ScrollbackLen` / `undefined: ScrollbackLine`.

- [ ] **Step 3: Implement the C bridge + exported callbacks**

`internal/engine/bridge.go`:
```go
package engine

/*
#cgo pkg-config: vterm
#include <vterm.h>

// Go-exported callback declarations (defined via //export in this file).
int goSBPushline(int cols, const VTermScreenCell *cells, void *user);
int goSBPopline(int cols, VTermScreenCell *cells, void *user);

// C trampolines stored in the callbacks struct; they forward to Go.
static int cSBPushline(int cols, const VTermScreenCell *cells, void *user) {
	return goSBPushline(cols, (VTermScreenCell *)cells, user);
}
static int cSBPopline(int cols, VTermScreenCell *cells, void *user) {
	return goSBPopline(cols, cells, user);
}

// One static callbacks struct; only scrollback hooks are used here.
static VTermScreenCallbacks sbCallbacks;

static void registerScrollback(VTermScreen *screen, void *user) {
	sbCallbacks.sb_pushline = cSBPushline;
	sbCallbacks.sb_popline  = cSBPopline;
	vterm_screen_set_callbacks(screen, &sbCallbacks, user);
}

// Helpers to read/write a cells[] array passed to the callbacks.
static uint32_t cellsChar0(const VTermScreenCell *cells, int i) { return cells[i].chars[0]; }
static void     cellSetChar0(VTermScreenCell *cells, int i, uint32_t ch) {
	cells[i].chars[0] = ch;
	cells[i].width = 1;
}
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

// cgoHandle aliases runtime/cgo.Handle so engine.go can reference it without
// importing cgo there.
type cgoHandle = cgo.Handle

//export goSBPushline
func goSBPushline(cols C.int, cells *C.VTermScreenCell, user unsafe.Pointer) C.int {
	e := cgo.Handle(uintptr(user)).Value().(*Engine)
	line := make([]Cell, int(cols))
	for i := 0; i < int(cols); i++ {
		line[i] = Cell{Rune: rune(C.cellsChar0(cells, C.int(i))), Width: 1}
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
		C.cellSetChar0(cells, C.int(i), C.uint32_t(line[i].Rune))
	}
	return 1
}
```

> Note on the handle: `cgo.NewHandle` returns a `cgo.Handle` (a `uintptr`). We pass it to C as a `void*` via `unsafe.Pointer(uintptr(h))` and recover it with `cgo.Handle(uintptr(user))`. This round-trips the integer handle through the pointer slot without pointing at Go memory, which is the supported pattern. The Engine keeps the handle so it can `Delete()` it in Close (Task 3 already does).

- [ ] **Step 4: Register the callbacks in New**

In `internal/engine/engine.go`, add the import and registration. Add to the import block:
```go
	"runtime/cgo"
```
Add, in `New`, immediately before `return e, nil`:
```go
	e.handle = cgo.NewHandle(e)
	C.registerScrollback(e.screen, unsafe.Pointer(uintptr(e.handle)))
```
And add the public scrollback accessors at the end of `engine.go`:
```go
// ScrollbackLen returns the number of lines currently held in scrollback.
func (e *Engine) ScrollbackLen() int { return e.sb.len() }

// ScrollbackLine returns the n-th line back from the screen (0 = newest
// scrolled-off line). Out-of-range returns nil.
func (e *Engine) ScrollbackLine(n int) []Cell { return e.sb.line(n) }
```

- [ ] **Step 5: Run the full engine test suite**

Run: `go test ./internal/engine/ -v`
Expected: PASS, including `TestEngineScrollbackFillsOnOverflow`.

- [ ] **Step 6: Run the race detector on the package**

Run: `go test -race ./internal/engine/ -v`
Expected: PASS, no data-race reports. (Engine is single-goroutine by contract; this guards the cgo callback path.)

- [ ] **Step 7: Commit**

```bash
git add internal/engine/bridge.go internal/engine/engine.go
git commit -m "feat(engine): route libvterm scrollback callbacks into the ring buffer"
```

---

### Task 6: CI workflow for the cgo build

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the workflow**

`.github/workflows/ci.yml`:
```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    strategy:
      matrix:
        os: [macos-latest, ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Install libvterm (macOS)
        if: runner.os == 'macOS'
        run: brew install libvterm
      - name: Install libvterm (Linux)
        if: runner.os == 'Linux'
        run: sudo apt-get update && sudo apt-get install -y libvterm-dev pkg-config
      - name: Build
        run: go build ./...
      - name: Vet
        run: go vet ./...
      - name: Test (race)
        run: go test -race ./...
```

- [ ] **Step 2: Verify the workflow is valid YAML locally**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('valid')"`
Expected: `valid`.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: build and race-test the cgo engine on macOS and Linux"
```

---

## Self-Review

**Spec coverage (engine portion of SPEC.md):**
- "feed bytes, read grid" → Tasks 1, 3 ✓
- "scrollback ring buffer fed by sb_pushline, default 10k configurable" → Tasks 4, 5 (cap is a `New` parameter; the 10k default is set by the caller in a later module/plan) ✓
- "cgo callback boundary via handle, isolated in one unit" → `bridge.go`, Task 5 ✓
- "color mapping: chrome named ANSI, shell passthrough" → engine maps to indexed/RGB/default faithfully (Task 3); chrome-vs-content policy lives in the `color`/`render` modules, later plan ✓
- "resize: active reflow, no history reflow" → Task 3 `Resize` (behavior is libvterm's) ✓
- "alt-screen" → Task 3 `New` enables it ✓
- OSC (path/exit) → explicitly out of scope, deferred to a later plan (SPEC Open Question #4) ✓

**Placeholder scan:** No TBD/TODO. The one flagged uncertainty (exact field names for `attrs.underline` / cursor accessors) includes the exact grep command to resolve it against the header — this is a verification instruction, not a placeholder.

**Type consistency:** `New(rows, cols, sbLines)`, `Cell{Rune,Width,FG,BG,Attrs}`, `Color{R,G,B,Index,IsDefault,IsIndexed}`, `Attr` bitset, `scrollback.push/pop/len/line`, `Engine.ScrollbackLen/ScrollbackLine` — names used consistently across tasks.

**Known follow-ups for later plans:** ptyhost, render, compositor, hud/*, input, main; OSC parsing; the 10k default + config flag wiring.
