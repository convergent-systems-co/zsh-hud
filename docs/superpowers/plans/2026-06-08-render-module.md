# render Module Implementation Plan (Plan 3 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `render` module — given a composited `Frame` (a 2D grid of cells + cursor), draw it to an `io.Writer` (the real terminal) efficiently by diffing against the previously drawn frame and emitting only what changed.

**Architecture:** Pure logic, no OS/cgo. `Frame` holds `engine.Cell`s (render imports `engine` only for the `Cell`/`Color`/`Attr` value types — no behavior). A `Renderer` keeps the last drawn frame; `Render(f)` emits ANSI: a full redraw on the first frame or a size change, otherwise a minimal per-cell diff. Style (SGR) emission is centralized and state-tracked to avoid redundant escapes. Fully testable against a `bytes.Buffer`.

**Tech Stack:** Go 1.26, stdlib only (`bytes`, `strconv`, `strings`, `io`). Imports `terminal-hud/internal/engine` for cell types.

**Prerequisite:** `engine` module present on the branch (it is — render is branched off `main`, which has `engine`). render does NOT depend on `ptyhost`.

**Scope notes:**
- Single-width cells only in v1. Wide (CJK, `Width==2`) and combining characters are a documented follow-up — emitting them correctly requires column-accounting that the baseline diff does not do. The HUD chrome and typical shell output are single-width. (Consistent with the engine's existing scrollback cell limitation.)
- Raw-mode / alt-screen setup and where the `io.Writer` comes from (os.Stdout) belong to `main`. render only writes bytes.
- The "bar chrome uses named ANSI, shell content passes through" color policy is the **compositor's** concern (which colors populate the cells). render faithfully emits whatever color each cell carries.

---

## File Structure

```
internal/render/
  doc.go            ← package doc
  frame.go          ← Frame: 2D engine.Cell grid + cursor; NewFrame/At/Set/Equal helpers
  frame_test.go
  sgr.go            ← encodeStyle (engine.Color/Attr → SGR), cursorTo, named escapes
  sgr_test.go
  render.go         ← Renderer: Render(frame) → diff + emit to io.Writer
  render_test.go
```

Responsibilities: `frame.go` is the data structure (no I/O). `sgr.go` is pure escape-sequence encoding. `render.go` is the diff + emit engine.

---

### Task 1: Frame type

**Files:** Create `internal/render/doc.go`, `internal/render/frame.go`; test `internal/render/frame_test.go`.

- [ ] **Step 1: Package doc** — `internal/render/doc.go`:
```go
// Package render draws a composited Frame (a grid of cells plus a cursor) to an
// io.Writer, diffing against the previously drawn frame so only changed cells
// are emitted. It is pure: no terminal setup, no cgo. main supplies the writer
// (os.Stdout) and owns raw-mode/alt-screen; the compositor supplies the frames.
package render
```

- [ ] **Step 2: Failing test** — `internal/render/frame_test.go`:
```go
package render

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestNewFrameDimensionsAndBlankCells(t *testing.T) {
	f := NewFrame(3, 5)
	if f.Rows != 3 || f.Cols != 5 {
		t.Fatalf("dims = %dx%d, want 3x5", f.Rows, f.Cols)
	}
	if got := f.At(0, 0); got.Rune != 0 {
		t.Fatalf("blank cell rune = %q, want 0", got.Rune)
	}
}

func TestFrameSetAndAt(t *testing.T) {
	f := NewFrame(2, 2)
	f.Set(1, 1, engine.Cell{Rune: 'X', Width: 1})
	if got := f.At(1, 1); got.Rune != 'X' {
		t.Fatalf("At(1,1).Rune = %q, want X", got.Rune)
	}
	// out-of-range At returns zero Cell, Set is a no-op (no panic)
	if got := f.At(9, 9); got.Rune != 0 {
		t.Fatalf("At(9,9) should be zero Cell")
	}
	f.Set(9, 9, engine.Cell{Rune: 'Z'}) // must not panic
}

func TestFrameEqualCellAndDims(t *testing.T) {
	a := NewFrame(1, 2)
	b := NewFrame(1, 2)
	if !a.SameSize(b) {
		t.Fatal("same dims should report SameSize")
	}
	c := NewFrame(2, 2)
	if a.SameSize(c) {
		t.Fatal("different dims should not be SameSize")
	}
}
```

- [ ] **Step 3: Run — expect FAIL** (`undefined: NewFrame`).
Run: `go test ./internal/render/ -run TestFrame -v` and `-run TestNewFrame`.

- [ ] **Step 4: Implement** — `internal/render/frame.go`:
```go
package render

import "terminal-hud/internal/engine"

// Frame is a composited screen: a row-major grid of cells plus the cursor the
// terminal should show after drawing. Cell (0,0) is top-left.
type Frame struct {
	Rows, Cols  int
	cells       []engine.Cell // len == Rows*Cols, row-major
	CursorRow   int
	CursorCol   int
	CursorShown bool
}

// NewFrame returns a Frame of the given size with all-blank cells (rune 0).
func NewFrame(rows, cols int) *Frame {
	if rows < 0 {
		rows = 0
	}
	if cols < 0 {
		cols = 0
	}
	return &Frame{Rows: rows, Cols: cols, cells: make([]engine.Cell, rows*cols), CursorShown: true}
}

// At returns the cell at (row, col), or a zero Cell if out of range.
func (f *Frame) At(row, col int) engine.Cell {
	if row < 0 || row >= f.Rows || col < 0 || col >= f.Cols {
		return engine.Cell{}
	}
	return f.cells[row*f.Cols+col]
}

// Set writes a cell at (row, col). Out-of-range writes are ignored.
func (f *Frame) Set(row, col int, c engine.Cell) {
	if row < 0 || row >= f.Rows || col < 0 || col >= f.Cols {
		return
	}
	f.cells[row*f.Cols+col] = c
}

// SameSize reports whether two frames have identical dimensions.
func (f *Frame) SameSize(other *Frame) bool {
	return other != nil && f.Rows == other.Rows && f.Cols == other.Cols
}
```

- [ ] **Step 5: Run — expect PASS.**
- [ ] **Step 6: Commit**
```bash
git add internal/render/doc.go internal/render/frame.go internal/render/frame_test.go
git commit -m "feat(render): Frame grid type"
```

---

### Task 2: SGR + cursor escape encoding

**Files:** Create `internal/render/sgr.go`; test `internal/render/sgr_test.go`.

- [ ] **Step 1: Failing test** — `internal/render/sgr_test.go`:
```go
package render

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestEncodeStyleDefault(t *testing.T) {
	got := encodeStyle(engine.Color{IsDefault: true}, engine.Color{IsDefault: true}, 0)
	if got != "\x1b[0;39;49m" {
		t.Fatalf("default style = %q, want reset+default fg/bg", got)
	}
}

func TestEncodeStyleIndexedBasicAndBright(t *testing.T) {
	// fg indexed 2 (green) → 32; bg indexed 9 (bright red) → 101
	got := encodeStyle(engine.Color{IsIndexed: true, Index: 2}, engine.Color{IsIndexed: true, Index: 9}, 0)
	if got != "\x1b[0;32;101m" {
		t.Fatalf("indexed style = %q, want 0;32;101", got)
	}
}

func TestEncodeStyle256AndRGB(t *testing.T) {
	fg := engine.Color{IsIndexed: true, Index: 200}      // 256-color → 38;5;200
	bg := engine.Color{R: 10, G: 20, B: 30}              // RGB → 48;2;10;20;30
	got := encodeStyle(fg, bg, 0)
	if got != "\x1b[0;38;5;200;48;2;10;20;30m" {
		t.Fatalf("256/rgb style = %q", got)
	}
}

func TestEncodeStyleAttrs(t *testing.T) {
	got := encodeStyle(engine.Color{IsDefault: true}, engine.Color{IsDefault: true}, engine.AttrBold|engine.AttrUnderline)
	if got != "\x1b[0;1;4;39;49m" {
		t.Fatalf("attr style = %q, want bold+underline", got)
	}
}

func TestCursorTo(t *testing.T) {
	if got := cursorTo(0, 0); got != "\x1b[1;1H" {
		t.Fatalf("cursorTo(0,0) = %q, want 1;1H", got)
	}
	if got := cursorTo(4, 9); got != "\x1b[5;10H" {
		t.Fatalf("cursorTo(4,9) = %q, want 5;10H", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: encodeStyle`).

- [ ] **Step 3: Implement** — `internal/render/sgr.go`:
```go
package render

import (
	"strconv"
	"strings"

	"terminal-hud/internal/engine"
)

// ANSI control prefixes.
const (
	csi   = "\x1b["
	clear = "\x1b[2J" // erase entire screen
)

// cursorTo returns the escape to move the cursor to (row, col), 0-based input,
// emitted as 1-based CSI H.
func cursorTo(row, col int) string {
	return csi + strconv.Itoa(row+1) + ";" + strconv.Itoa(col+1) + "H"
}

// encodeStyle returns a full SGR sequence that resets then sets attrs, fg, bg.
// Always self-contained (leads with reset 0) so it can be emitted standalone.
func encodeStyle(fg, bg engine.Color, attrs engine.Attr) string {
	params := []string{"0"} // reset first
	if attrs.Has(engine.AttrBold) {
		params = append(params, "1")
	}
	if attrs.Has(engine.AttrItalic) {
		params = append(params, "3")
	}
	if attrs.Has(engine.AttrUnderline) {
		params = append(params, "4")
	}
	if attrs.Has(engine.AttrBlink) {
		params = append(params, "5")
	}
	if attrs.Has(engine.AttrReverse) {
		params = append(params, "7")
	}
	if attrs.Has(engine.AttrStrike) {
		params = append(params, "9")
	}
	params = append(params, colorParams(fg, true)...)
	params = append(params, colorParams(bg, false)...)
	return csi + strings.Join(params, ";") + "m"
}

// colorParams returns the SGR parameters for one color. fg selects the
// foreground code family (30s/90s/38) vs background (40s/100s/48).
func colorParams(c engine.Color, fg bool) []string {
	base := 30
	if !fg {
		base = 40
	}
	switch {
	case c.IsDefault:
		return []string{strconv.Itoa(base + 9)} // 39 fg / 49 bg
	case c.IsIndexed && c.Index < 8:
		return []string{strconv.Itoa(base + int(c.Index))} // 30-37 / 40-47
	case c.IsIndexed && c.Index < 16:
		// bright: 90-97 fg / 100-107 bg
		brightBase := 90
		if !fg {
			brightBase = 100
		}
		return []string{strconv.Itoa(brightBase + int(c.Index) - 8)}
	case c.IsIndexed:
		// 256-color: 38;5;n / 48;5;n
		return []string{strconv.Itoa(base + 8), "5", strconv.Itoa(int(c.Index))}
	default:
		// RGB: 38;2;r;g;b / 48;2;r;g;b
		return []string{strconv.Itoa(base + 8), "2",
			strconv.Itoa(int(c.R)), strconv.Itoa(int(c.G)), strconv.Itoa(int(c.B))}
	}
}
```

- [ ] **Step 4: Run — expect PASS** (all 5 SGR tests).
- [ ] **Step 5: Commit**
```bash
git add internal/render/sgr.go internal/render/sgr_test.go
git commit -m "feat(render): SGR style and cursor escape encoding"
```

---

### Task 3: Renderer — full redraw

**Files:** Create `internal/render/render.go`; test `internal/render/render_test.go`.

- [ ] **Step 1: Failing test** — `internal/render/render_test.go`:
```go
package render

import (
	"bytes"
	"strings"
	"testing"

	"terminal-hud/internal/engine"
)

func putText(f *Frame, row, col, s string) {
	for i, r := range s {
		f.Set(row, col+i, engine.Cell{Rune: r, Width: 1})
	}
}

func TestRendererFirstFrameFullRedraw(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	f := NewFrame(1, 5)
	putText(f, 0, 0, "hi")
	f.CursorRow, f.CursorCol = 0, 2

	if err := r.Render(f); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, clear) {
		t.Fatalf("first frame should clear screen; got %q", out)
	}
	if !strings.Contains(out, "h") || !strings.Contains(out, "i") {
		t.Fatalf("output missing content; got %q", out)
	}
	// cursor should end at row 1 col 3 (1-based)
	if !strings.HasSuffix(out, cursorTo(0, 2)) {
		t.Fatalf("output should end positioning cursor at (0,2); got %q", out)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: NewRenderer`).

- [ ] **Step 3: Implement** — `internal/render/render.go`:
```go
package render

import (
	"bytes"
	"io"

	"terminal-hud/internal/engine"
)

// Renderer draws Frames to an io.Writer, emitting only what changed since the
// last Render. Not safe for concurrent use; the event loop calls it serially.
type Renderer struct {
	w    io.Writer
	last *Frame
}

// NewRenderer returns a Renderer writing to w. The first Render is a full redraw.
func NewRenderer(w io.Writer) *Renderer { return &Renderer{w: w} }

// Render draws f. On the first call or when the size changes it does a full
// redraw; otherwise it emits only cells that differ from the last frame.
func (r *Renderer) Render(f *Frame) error {
	var b bytes.Buffer
	if r.last == nil || !r.last.SameSize(f) {
		r.fullRedraw(&b, f)
	} else {
		r.diff(&b, f)
	}
	// Position the visible cursor last.
	b.WriteString(cursorTo(f.CursorRow, f.CursorCol))
	if _, err := r.w.Write(b.Bytes()); err != nil {
		return err
	}
	r.last = cloneFrame(f)
	return nil
}

// fullRedraw clears the screen and writes every cell, tracking the active style
// to avoid redundant SGR escapes.
func (r *Renderer) fullRedraw(b *bytes.Buffer, f *Frame) {
	b.WriteString(clear)
	cur := ""
	for row := 0; row < f.Rows; row++ {
		b.WriteString(cursorTo(row, 0))
		for col := 0; col < f.Cols; col++ {
			cur = writeCell(b, f.At(row, col), cur)
		}
	}
}

// writeCell emits a cell's style (only if it differs from cur) and its glyph,
// returning the new active style. A zero rune renders as a space.
func writeCell(b *bytes.Buffer, c engine.Cell, cur string) string {
	st := encodeStyle(c.FG, c.BG, c.Attrs)
	if st != cur {
		b.WriteString(st)
		cur = st
	}
	if c.Rune == 0 {
		b.WriteByte(' ')
	} else {
		b.WriteRune(c.Rune)
	}
	return cur
}

func cloneFrame(f *Frame) *Frame {
	cp := &Frame{Rows: f.Rows, Cols: f.Cols, CursorRow: f.CursorRow, CursorCol: f.CursorCol, CursorShown: f.CursorShown}
	cp.cells = make([]engine.Cell, len(f.cells))
	copy(cp.cells, f.cells)
	return cp
}
```
(Note: `diff` is added in Task 4; for Task 3 add a temporary stub `func (r *Renderer) diff(b *bytes.Buffer, f *Frame) { r.fullRedraw(b, f) }` so the package compiles, clearly commented `// TEMP: replaced in Task 4`. Task 4 replaces it.)

- [ ] **Step 4: Run — expect PASS.**
Run: `go test ./internal/render/ -run TestRendererFirstFrame -v`

- [ ] **Step 5: Commit**
```bash
git add internal/render/render.go internal/render/render_test.go
git commit -m "feat(render): Renderer full redraw with SGR state tracking"
```

---

### Task 4: Renderer — diff path

**Files:** Modify `internal/render/render.go` (replace the `diff` stub); test `internal/render/render_test.go` (extend).

- [ ] **Step 1: Failing tests (append)**:
```go
func TestRendererDiffEmitsOnlyChangedCells(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)

	f1 := NewFrame(1, 5)
	putText(f1, 0, 0, "abcde")
	if err := r.Render(f1); err != nil {
		t.Fatalf("Render f1: %v", err)
	}

	// Second frame: change only column 2 ('c' -> 'X').
	buf.Reset()
	f2 := NewFrame(1, 5)
	putText(f2, 0, 0, "abXde")
	if err := r.Render(f2); err != nil {
		t.Fatalf("Render f2: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, clear) {
		t.Fatalf("diff must not clear screen; got %q", out)
	}
	if !strings.Contains(out, cursorTo(0, 2)) || !strings.Contains(out, "X") {
		t.Fatalf("diff should reposition to (0,2) and emit X; got %q", out)
	}
	// Must NOT re-emit unchanged 'a' or 'e' as content moves.
	if strings.Contains(out, cursorTo(0, 0)) {
		t.Fatalf("diff should not touch column 0; got %q", out)
	}
}

func TestRendererUnchangedFrameEmitsNoCellWrites(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	f := NewFrame(1, 3)
	putText(f, 0, 0, "xyz")
	_ = r.Render(f)

	buf.Reset()
	same := NewFrame(1, 3)
	putText(same, 0, 0, "xyz")
	same.CursorRow, same.CursorCol = f.CursorRow, f.CursorCol
	_ = r.Render(same)
	out := buf.String()
	// Only the trailing cursor reposition should be emitted; no clear, no glyphs.
	if strings.ContainsAny(out, "xyz") {
		t.Fatalf("unchanged frame should emit no glyphs; got %q", out)
	}
}

func TestRendererSizeChangeForcesFullRedraw(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf)
	_ = r.Render(NewFrame(1, 3))

	buf.Reset()
	_ = r.Render(NewFrame(2, 4)) // different size
	if !strings.Contains(buf.String(), clear) {
		t.Fatalf("size change should full-redraw (clear); got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (diff currently full-redraws, so `TestRendererDiffEmitsOnlyChangedCells` fails on the `clear`/column-0 assertions).

- [ ] **Step 3: Replace the `diff` stub** in `render.go` with the real diff:
```go
// diff emits only cells that changed from the last frame. Cells are compared
// for full equality (glyph + style). Each changed cell is repositioned and
// rewritten; runs of adjacent changed cells in a row reuse the cursor position
// (the terminal advances it as glyphs are written) and the active style.
func (r *Renderer) diff(b *bytes.Buffer, f *Frame) {
	for row := 0; row < f.Rows; row++ {
		cur := ""        // active style is unknown at each repositioning
		col := 0
		for col < f.Cols {
			if cellsEqual(f.At(row, col), r.last.At(row, col)) {
				col++
				continue
			}
			// Start of a changed run: reposition once, then write contiguous
			// changed cells, letting the terminal advance the cursor.
			b.WriteString(cursorTo(row, col))
			cur = ""
			for col < f.Cols && !cellsEqual(f.At(row, col), r.last.At(row, col)) {
				cur = writeCell(b, f.At(row, col), cur)
				col++
			}
		}
	}
}

// cellsEqual reports whether two cells are visually identical.
func cellsEqual(a, b engine.Cell) bool {
	return a.Rune == b.Rune && a.Width == b.Width && a.Attrs == b.Attrs && a.FG == b.FG && a.BG == b.BG
}
```
(`engine.Color` is a comparable struct of value fields, so `a.FG == b.FG` is valid.)

- [ ] **Step 4: Run the full render suite + vet**
Run: `go test ./internal/render/ -v` then `go vet ./internal/render/`
Expected: all render tests PASS; vet clean.

- [ ] **Step 5: Commit**
```bash
git add internal/render/render.go internal/render/render_test.go
git commit -m "feat(render): minimal diff emission of changed cells"
```

---

## Self-Review

**Spec coverage (render portion of SPEC.md):**
- "given a composited frame (2D cells), draw to the real terminal efficiently (diff against last frame; only emit changed cells)" → Tasks 3 (full) + 4 (diff) ✓
- "draw composited frame" — render is bar-agnostic; it draws whatever cells the compositor placed (chrome and shell content alike), faithfully emitting each cell's color/attrs ✓
- Renders to an `io.Writer` so main supplies os.Stdout; no terminal setup here ✓

**Placeholder scan:** none. Task 3 introduces a clearly-labeled TEMP `diff` stub that Task 4 replaces (cross-task TDD, same pattern used in the engine plan).

**Type consistency:** `Frame{Rows,Cols,cells,CursorRow,CursorCol,CursorShown}`, `NewFrame/At/Set/SameSize`, `Renderer{w,last}`, `NewRenderer`, `Render`, helpers `fullRedraw/diff/writeCell/cellsEqual/cloneFrame/encodeStyle/colorParams/cursorTo` — consistent across tasks. `cellsEqual` relies on `engine.Color` being a comparable value struct (it is: R,G,B,Index uint8 + two bools).

**Known limitations (documented):**
- Single-width cells only; wide/combining chars are a follow-up (see scope note). `cellsEqual` compares `Width` so a width change forces a rewrite, but multi-column layout is not yet handled.
- Diff resets active style at the start of each changed run (emits a full SGR per run) — correct but not maximally terse; cross-run style memo is a possible optimization.
- No scroll-region/insert-line optimizations; full-cell diff only. Adequate for the HUD; revisit if profiling shows redraw cost.

**Dependencies:** stdlib + `internal/engine` only. No new external deps; no CI change (existing `go test -race ./...` covers `internal/render`).

**Follow-ups (later plans):** compositor (produces Frames from engine grid + bars + scroll offset); main (raw mode, alt-screen, supplies os.Stdout, event loop).
