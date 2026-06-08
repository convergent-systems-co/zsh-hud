# compositor Module Implementation Plan (Plan 4 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `compositor` module — the pure layer that assembles a `render.Frame` for the whole screen: a frozen top-bar row, the shell view (live or scrolled into scrollback) in the middle, and a frozen bottom-bar row. This is the piece that realizes "scroll the middle while the bars stay frozen."

**Architecture:** Pure logic. `compositor.Compose(rows, cols, src, scrollOffset, topBar, bottomBar)` returns a `*render.Frame`. The shell content comes from a `GridSource` interface (the `engine` satisfies it), so compositor is testable with a fake grid and doesn't pull in cgo. Bars are passed in as `[]engine.Cell` (the `hud` module will produce them later); compositor places and clips them. Dependency direction: `engine ← render`, `engine ← compositor`, `render ← compositor`.

**Tech Stack:** Go 1.26, stdlib only. Imports `terminal-hud/internal/engine` (cell types) and `terminal-hud/internal/render` (Frame).

**Prerequisite:** `engine` and `render` on the branch (they're on `main`; this branches off `main`).

**Scope notes:**
- The "bar chrome = named ANSI / shell content = passthrough" color policy is realized **upstream**: `hud` builds bar cells with indexed (named-ANSI) colors; `engine` cells carry the shell's own colors. Compositor is policy-neutral — it just places cells.
- main keeps the engine sized to the middle (`rows-2` × `cols`); compositor reads `src` defensively regardless.
- Where `scrollOffset` comes from (scroll hotkeys) is the `input`/`main` concern; compositor just consumes it.
- Single-width cells (consistent with render's v1).

---

## File Structure

```
internal/compositor/
  doc.go             ← package doc
  source.go          ← GridSource interface + padOrClip helper
  source_test.go
  compositor.go      ← Compose: three-region assembly + scroll mapping
  compositor_test.go
```

---

### Task 1: GridSource interface + padOrClip + fake grid for tests

**Files:** Create `internal/compositor/doc.go`, `internal/compositor/source.go`; test `internal/compositor/source_test.go`.

- [ ] **Step 1: Package doc** — `internal/compositor/doc.go`:
```go
// Package compositor assembles a render.Frame for the whole screen: a frozen
// top-bar row, the shell view (live or scrolled into scrollback) in the middle,
// and a frozen bottom-bar row. It is pure: the shell content arrives through the
// GridSource interface (satisfied by *engine.Engine), so no cgo is pulled in.
package compositor
```

- [ ] **Step 2: Failing test** — `internal/compositor/source_test.go`:
```go
package compositor

import (
	"testing"

	"terminal-hud/internal/engine"
)

func cellsRunes(cs []engine.Cell) string {
	var r []rune
	for _, c := range cs {
		if c.Rune == 0 {
			r = append(r, ' ')
		} else {
			r = append(r, c.Rune)
		}
	}
	return string(r)
}

func mkCells(s string) []engine.Cell {
	out := make([]engine.Cell, 0, len(s))
	for _, r := range s {
		out = append(out, engine.Cell{Rune: r, Width: 1})
	}
	return out
}

func TestPadOrClipPads(t *testing.T) {
	got := padOrClip(mkCells("hi"), 5)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if cellsRunes(got) != "hi   " {
		t.Fatalf("padded = %q, want 'hi   '", cellsRunes(got))
	}
}

func TestPadOrClipClips(t *testing.T) {
	got := padOrClip(mkCells("toolong"), 4)
	if len(got) != 4 || cellsRunes(got) != "tool" {
		t.Fatalf("clipped = %q (len %d), want 'tool'", cellsRunes(got), len(got))
	}
}

func TestPadOrClipExact(t *testing.T) {
	got := padOrClip(mkCells("abcd"), 4)
	if cellsRunes(got) != "abcd" {
		t.Fatalf("exact = %q", cellsRunes(got))
	}
}
```

- [ ] **Step 3: Run — expect FAIL** (`undefined: padOrClip`).
Run: `go test ./internal/compositor/ -run TestPadOrClip -v`

- [ ] **Step 4: Implement** — `internal/compositor/source.go`:
```go
package compositor

import "terminal-hud/internal/engine"

// GridSource is the read-only view of the shell's screen + scrollback that the
// compositor needs. *engine.Engine satisfies it. ScrollbackLine(0) is the
// newest line that scrolled off the top of the screen.
type GridSource interface {
	Size() (rows, cols int)
	Cell(row, col int) engine.Cell
	CursorPos() (row, col int)
	ScrollbackLen() int
	ScrollbackLine(n int) []engine.Cell
}

// padOrClip returns a slice exactly width long: cells beyond width are dropped;
// a short input is padded with blank cells (rune 0). Returns an empty slice for
// width <= 0.
func padOrClip(cells []engine.Cell, width int) []engine.Cell {
	if width <= 0 {
		return []engine.Cell{}
	}
	out := make([]engine.Cell, width)
	copy(out, cells) // copies min(len(cells), width); remainder stays blank
	return out
}

// Compile-time check that the real engine satisfies GridSource.
var _ GridSource = (*engine.Engine)(nil)
```

- [ ] **Step 5: Run — expect PASS.** gofmt.
- [ ] **Step 6: Commit**
```bash
git add internal/compositor/doc.go internal/compositor/source.go internal/compositor/source_test.go
git commit -m "feat(compositor): GridSource interface and padOrClip helper"
```

---

### Task 2: Compose — bars + live shell view (scrollOffset = 0)

**Files:** Create `internal/compositor/compositor.go`; test `internal/compositor/compositor_test.go`.

- [ ] **Step 1: Add a fake grid + failing test** — `internal/compositor/compositor_test.go`:
```go
package compositor

import (
	"testing"

	"terminal-hud/internal/engine"
)

// fakeGrid is a test GridSource: a fixed screen of text rows plus an ordered
// scrollback (index 0 = OLDEST here; newest returned by ScrollbackLine(0)).
type fakeGrid struct {
	rows, cols int
	screen     []string // len rows; each padded/clipped to cols on read
	sb         []string // oldest..newest
	curRow     int
	curCol     int
}

func (g *fakeGrid) Size() (int, int) { return g.rows, g.cols }
func (g *fakeGrid) CursorPos() (int, int) { return g.curRow, g.curCol }
func (g *fakeGrid) ScrollbackLen() int { return len(g.sb) }

func (g *fakeGrid) Cell(row, col int) engine.Cell {
	if row < 0 || row >= g.rows || col < 0 || col >= len(g.screen[row]) {
		return engine.Cell{}
	}
	return engine.Cell{Rune: rune(g.screen[row][col]), Width: 1}
}

// ScrollbackLine(0) = newest scrolled-off line.
func (g *fakeGrid) ScrollbackLine(n int) []engine.Cell {
	if n < 0 || n >= len(g.sb) {
		return nil
	}
	s := g.sb[len(g.sb)-1-n] // n=0 -> last element = newest
	return mkCells(s)
}

func midRow(t *testing.T, f interface{ At(r, c int) engine.Cell }, row, cols int) string {
	var r []rune
	for c := 0; c < cols; c++ {
		ch := f.At(row, c).Rune
		if ch == 0 {
			ch = ' '
		}
		r = append(r, ch)
	}
	return string(r)
}

func TestComposeLiveViewPlacesBarsAndScreen(t *testing.T) {
	g := &fakeGrid{rows: 2, cols: 5, screen: []string{"AB", "CD"}, curRow: 1, curCol: 2}
	// full screen: 4 rows (top bar, 2 middle, bottom bar), 5 cols
	f := Compose(4, 5, g, 0, mkCells("TOP"), mkCells("BOT"))

	if f.Rows != 4 || f.Cols != 5 {
		t.Fatalf("frame dims = %dx%d, want 4x5", f.Rows, f.Cols)
	}
	if got := midRow(t, f, 0, 5); got != "TOP  " {
		t.Fatalf("top bar = %q, want 'TOP  '", got)
	}
	if got := midRow(t, f, 3, 5); got != "BOT  " {
		t.Fatalf("bottom bar = %q, want 'BOT  '", got)
	}
	if got := midRow(t, f, 1, 5); got != "AB   " {
		t.Fatalf("middle row 0 = %q, want 'AB   '", got)
	}
	if got := midRow(t, f, 2, 5); got != "CD   " {
		t.Fatalf("middle row 1 = %q, want 'CD   '", got)
	}
	// cursor: engine (1,2) -> frame (2,2), shown
	if !f.CursorShown || f.CursorRow != 2 || f.CursorCol != 2 {
		t.Fatalf("cursor = (%d,%d) shown=%v, want (2,2) true", f.CursorRow, f.CursorCol, f.CursorShown)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: Compose`).

- [ ] **Step 3: Implement** — `internal/compositor/compositor.go`:
```go
package compositor

import (
	"terminal-hud/internal/engine"
	"terminal-hud/internal/render"
)

// Compose builds a Frame of rows x cols. Row 0 is the top bar, row rows-1 the
// bottom bar, and rows 1..rows-2 the shell view. scrollOffset scrolls the middle
// back into scrollback: 0 shows the live screen at the bottom; k shows the
// content k lines higher. scrollOffset is clamped to [0, ScrollbackLen].
//
// Bars are clipped/padded to cols. The cursor is mapped from the engine's screen
// position into the middle region and shown only in the live view (scrollOffset
// == 0); when scrolled back it is hidden.
func Compose(rows, cols int, src GridSource, scrollOffset int, topBar, bottomBar []engine.Cell) *render.Frame {
	f := render.NewFrame(rows, cols)
	if rows >= 1 {
		writeRow(f, 0, padOrClip(topBar, cols))
	}
	if rows >= 2 {
		writeRow(f, rows-1, padOrClip(bottomBar, cols))
	}

	midHeight := rows - 2
	if midHeight <= 0 {
		f.CursorShown = false
		return f
	}

	sb := src.ScrollbackLen()
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if scrollOffset > sb {
		scrollOffset = sb
	}
	screenRows, _ := src.Size()

	// Virtual coordinate: 0..sb-1 = scrollback oldest..newest, sb..sb+screenRows-1
	// = screen rows. The viewport bottom (live) shows the last screen row; scroll
	// moves the window up by scrollOffset.
	for i := 0; i < midHeight; i++ {
		frameRow := 1 + i
		virtual := (sb - scrollOffset) + i
		switch {
		case virtual < 0:
			// nothing (above oldest history) — leave blank
		case virtual < sb:
			// scrollback line; virtual is the oldest-based index, ScrollbackLine
			// wants newest-based n = sb-1-virtual.
			writeRow(f, frameRow, padOrClip(src.ScrollbackLine(sb-1-virtual), cols))
		default:
			screenRow := virtual - sb
			if screenRow < screenRows {
				for c := 0; c < cols; c++ {
					f.Set(frameRow, c, src.Cell(screenRow, c))
				}
			}
			// else: screen shorter than middle — leave blank
		}
	}

	if scrollOffset == 0 {
		cr, cc := src.CursorPos()
		f.CursorRow = 1 + cr
		f.CursorCol = cc
		f.CursorShown = true
	} else {
		f.CursorShown = false
	}
	return f
}

// writeRow places a row of cells (already sized to cols) at frame row r.
func writeRow(f *render.Frame, r int, cells []engine.Cell) {
	for c := 0; c < len(cells); c++ {
		f.Set(r, c, cells[c])
	}
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt + `go vet ./internal/compositor/`.
- [ ] **Step 5: Commit**
```bash
git add internal/compositor/compositor.go internal/compositor/compositor_test.go
git commit -m "feat(compositor): three-region layout with live shell view"
```

---

### Task 3: Scrollback view (scrollOffset > 0) + clamping

**Files:** Modify nothing in `compositor.go` (the logic is already there); test `internal/compositor/compositor_test.go` (extend). This task verifies the scroll mapping that Task 2 implemented, against worked examples.

- [ ] **Step 1: Append tests**:
```go
func TestComposeScrollbackOffset(t *testing.T) {
	// sb (oldest..newest) = ["s0","s1","s2","s3","s4"], screen 3 rows = ["L0","L1","L2"].
	g := &fakeGrid{
		rows: 3, cols: 4,
		screen: []string{"L0", "L1", "L2"},
		sb:     []string{"s0", "s1", "s2", "s3", "s4"},
		curRow: 2, curCol: 0,
	}
	// full screen 5 rows => midHeight 3. scrollOffset 2.
	// virtual for i=0,1,2 = (5-2)+i = 3,4,5 => sb[3]="s3", sb[4]="s4", screen[0]="L0"
	f := Compose(5, 4, g, 2, mkCells("T"), mkCells("B"))
	if got := midRow(t, f, 1, 4); got != "s3  " {
		t.Fatalf("mid row 0 = %q, want 's3  '", got)
	}
	if got := midRow(t, f, 2, 4); got != "s4  " {
		t.Fatalf("mid row 1 = %q, want 's4  '", got)
	}
	if got := midRow(t, f, 3, 4); got != "L0  " {
		t.Fatalf("mid row 2 = %q, want 'L0  '", got)
	}
	// scrolled back => cursor hidden
	if f.CursorShown {
		t.Fatal("cursor should be hidden when scrolled back")
	}
}

func TestComposeScrollOffsetClampedToScrollbackLen(t *testing.T) {
	g := &fakeGrid{
		rows: 2, cols: 3,
		screen: []string{"L0", "L1"},
		sb:     []string{"a", "b"}, // sb len 2
		curRow: 0, curCol: 0,
	}
	// full screen 4 rows => midHeight 2. Ask for scrollOffset 99 -> clamps to 2.
	// virtual i=0,1 = (2-2)+i = 0,1 => sb[0]="a", sb[1]="b" (oldest two).
	f := Compose(4, 3, g, 99, mkCells("T"), mkCells("B"))
	if got := midRow(t, f, 1, 3); got != "a  " {
		t.Fatalf("mid row 0 = %q, want 'a  '", got)
	}
	if got := midRow(t, f, 2, 3); got != "b  " {
		t.Fatalf("mid row 1 = %q, want 'b  '", got)
	}
}

func TestComposeNegativeOffsetTreatedAsLive(t *testing.T) {
	g := &fakeGrid{rows: 2, cols: 2, screen: []string{"L0", "L1"}, curRow: 0, curCol: 1}
	f := Compose(4, 2, g, -5, mkCells("T"), mkCells("B"))
	if got := midRow(t, f, 1, 2); got != "L0" {
		t.Fatalf("mid row 0 = %q, want 'L0'", got)
	}
	if !f.CursorShown || f.CursorRow != 1 || f.CursorCol != 1 {
		t.Fatalf("cursor = (%d,%d) shown=%v, want (1,1) true", f.CursorRow, f.CursorCol, f.CursorShown)
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the Task-2 implementation already handles these).
Run: `go test ./internal/compositor/ -run TestComposeScroll -v` and `-run TestComposeNegative`.
If any fail, the scroll mapping in `compositor.go` is wrong — re-derive against the worked examples in the comments before changing tests.

- [ ] **Step 3: Commit**
```bash
git add internal/compositor/compositor_test.go
git commit -m "test(compositor): scrollback view mapping and offset clamping"
```

---

### Task 4: Tiny-terminal edge cases + full suite

**Files:** Test `internal/compositor/compositor_test.go` (extend). Modify `compositor.go` only if an edge case panics.

- [ ] **Step 1: Append edge-case tests**:
```go
func TestComposeTinyTerminalNoMiddle(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 5, screen: nil}
	// rows=2 => top bar at 0, bottom bar at 1, no middle. Must not panic.
	f := Compose(2, 5, g, 0, mkCells("TOP"), mkCells("BOT"))
	if f.Rows != 2 {
		t.Fatalf("rows = %d, want 2", f.Rows)
	}
	if got := midRow(t, f, 0, 5); got != "TOP  " {
		t.Fatalf("top = %q", got)
	}
	if got := midRow(t, f, 1, 5); got != "BOT  " {
		t.Fatalf("bottom = %q", got)
	}
	if f.CursorShown {
		t.Fatal("no middle => cursor hidden")
	}
}

func TestComposeSingleRowOnlyTopBar(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 3}
	f := Compose(1, 3, g, 0, mkCells("HI"), mkCells("XX")) // must not panic
	if f.Rows != 1 {
		t.Fatalf("rows = %d, want 1", f.Rows)
	}
	if got := midRow(t, f, 0, 3); got != "HI " {
		t.Fatalf("row0 = %q, want 'HI '", got)
	}
}

func TestComposeZeroRows(t *testing.T) {
	g := &fakeGrid{rows: 0, cols: 0}
	f := Compose(0, 0, g, 0, nil, nil) // must not panic
	if f.Rows != 0 {
		t.Fatalf("rows = %d, want 0", f.Rows)
	}
}
```

- [ ] **Step 2: Run — expect PASS** (the `rows >= 1` / `rows >= 2` / `midHeight <= 0` guards in Compose handle these). If any panics, fix the guard in `compositor.go` (do not weaken the test).
Run: `go test ./internal/compositor/ -run TestComposeTiny -v`, `-run TestComposeSingle`, `-run TestComposeZero`.

- [ ] **Step 3: Full module suite + race + vet + build**
Run: `go test -race ./internal/compositor/ -v` (all pass), `go vet ./internal/compositor/`, `go build ./...`.

- [ ] **Step 4: Commit**
```bash
git add internal/compositor/compositor_test.go
git commit -m "test(compositor): tiny-terminal edge cases (no middle, single row, zero)"
```

---

## Self-Review

**Spec coverage (compositor portion of SPEC.md):**
- "three-region math: reserve rows 0 and N-1; map engine's screen+scrollback into rows 1..N-2; place bar strings in the frozen rows" → Tasks 2 (live), 3 (scrollback) ✓
- "scroll the middle, keep bars frozen, preserve real scrollback" → the virtual-coordinate mapping over `[scrollback ++ screen]` with `scrollOffset` (Task 3) ✓
- bars placed in rows 0 / N-1, clipped to width → `padOrClip` + `writeRow` (Tasks 1, 2) ✓
- cursor mapped into the middle and shown only live → Task 2/3 ✓

**Placeholder scan:** none. Task 3 deliberately adds tests for logic already written in Task 2 (the scroll mapping) — this is verification of the crux, not a stub.

**Type consistency:** `GridSource{Size,Cell,CursorPos,ScrollbackLen,ScrollbackLine}` exactly matches `*engine.Engine`'s methods (verified). `Compose(rows, cols int, src GridSource, scrollOffset int, topBar, bottomBar []engine.Cell) *render.Frame`. `padOrClip(cells, width)`, `writeRow(f, r, cells)`. Uses `render.NewFrame`, `(*render.Frame).Set`, fields `Rows/Cols/CursorRow/CursorCol/CursorShown` — all exported on render.Frame.

**Scroll mapping invariant (the crux), with the worked example from the tests:** virtual `v = (sb - clamp(scrollOffset)) + i` for viewport row `i`. `v < sb` → scrollback (newest-based index `sb-1-v`); `v >= sb` → screen row `v-sb`. At `scrollOffset=0`, `v = sb+i` → screen row `i` (live view at bottom). Verified by `TestComposeScrollbackOffset` (sb=5, screen=3, offset=2 → s3,s4,L0).

**Known follow-ups (later plans):** hud (produces the top/bottom bar cells with named-ANSI chrome colors); input (scroll hotkeys that drive scrollOffset); main (owns dimensions, engine sizing to mid-height, the event loop). Wide-cell handling deferred (consistent with render).

**Verification this is engine-compatible:** `source.go` includes `var _ GridSource = (*engine.Engine)(nil)` (Task 1 Step 4), a compile-time assertion that the real engine satisfies the interface. Verified that engine's method set matches: `Cell(int,int) engine.Cell`, `Size() (int,int)`, `CursorPos() (int,int)`, `ScrollbackLen() int`, `ScrollbackLine(int) []engine.Cell`.
