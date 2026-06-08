# input Module Implementation Plan (Plan 6 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `input` module — a pure interpreter that turns the raw stdin byte stream into (a) bytes to forward to the shell's pty and (b) a stream of semantic HUD `Action`s (scroll, enter/exit copy-mode, move copy cursor, toggle selection, yank). Plus a small `clip` leaf for writing the clipboard.

**Architecture:** `input.Interpreter` is a stateful byte→action translator with a small buffer for escape sequences split across reads. It holds only a `Mode` (Normal/Copy) and the partial-sequence buffer — NOT terminal dimensions, the grid, scrollOffset, or selection coordinates. `main` owns those: it applies the actions (adjusts scrollOffset, moves a copy cursor, extracts selected text from the engine, calls `clip.Copy`). This keeps `input` fully unit-testable (feed sequences, assert mode + actions + forwarded bytes).

**Tech Stack:** Go 1.26, stdlib only (`bytes`, `os/exec`, `runtime`, `encoding/base64`). No new deps.

**Prerequisite:** none beyond the module scaffold (branches off `main`). `input` does not import engine/render/compositor/hud — it is decoupled by design.

---

## Copy-mode UX (DESIGN — review this)

A tmux-style model. Review and object here before implementation.

- **Normal mode** (default): every byte is forwarded to the shell, EXCEPT the scroll triggers:
  - `Shift+PageUp` (`ESC[5;2~`) → `EnterCopyMode` + `ScrollPageUp`.
  - Mouse **wheel up** (SGR `ESC[<64;…M`) → `EnterCopyMode` + `ScrollLineUp`.
  - (Shift+PageDown / wheel-down do nothing in normal mode — there's nothing below the live screen.)
- **Copy mode** (input is captured, nothing forwarded):
  - `Shift+PageUp`/`PageUp` → `ScrollPageUp`; `Shift+PageDown`/`PageDown` → `ScrollPageDown`; wheel up/down → `ScrollLineUp`/`ScrollLineDown`.
  - Arrow keys or `h`/`j`/`k`/`l` → `CopyMoveLeft`/`Down`/`Up`/`Right`.
  - `v` or Space → `CopyToggleSelect`.
  - `y` or Enter (`\r`) → `CopyYank` (main extracts the selection text, writes the clipboard, returns to live) then `ExitCopyMode`.
  - `q` → `ExitCopyMode` (reliable, single byte). `Esc` also exits, but a lone `Esc` is only recognized once the next byte arrives (ESC is also the CSI lead byte; we don't use input timeouts in v1). **`q` is the documented reliable exit.**
- `main` interprets the actions: `ScrollLineUp/Down` ±1, `ScrollPageUp/Down` ±(midHeight-1), clamped; `EnterCopyMode` shows scrollback + a copy cursor; `ExitCopyMode`/`CopyYank` reset scrollOffset to 0 (live).

---

## File Structure

```
internal/input/
  doc.go            ← package doc
  action.go         ← Mode, Action, Result types
  interpreter.go    ← Interpreter: Feed(bytes) Result, with CSI buffering
  interpreter_test.go
  mouse.go          ← SGR mouse sequence parsing + enable/disable escapes
  mouse_test.go
internal/clip/
  clip.go           ← Copy(text): pbcopy (darwin) / OSC52 fallback
  clip_test.go
```

---

### Task 1: types + interpreter skeleton + normal-mode forwarding + Shift+PageUp

**Files:** Create `internal/input/doc.go`, `action.go`, `interpreter.go`; test `interpreter_test.go`.

- [ ] **Step 1: doc + types.** `internal/input/doc.go`:
```go
// Package input interprets the raw terminal input stream into bytes to forward
// to the shell and semantic HUD actions (scroll, copy-mode navigation, yank).
// It is pure and holds no terminal geometry: main applies the actions against
// the engine/compositor state.
package input
```
`internal/input/action.go`:
```go
package input

// Mode is the interpreter's current input mode.
type Mode int

const (
	ModeNormal Mode = iota // bytes forwarded to the shell
	ModeCopy               // bytes captured for scrollback/selection
)

// Action is a semantic HUD command produced from input.
type Action int

const (
	ScrollPageUp Action = iota
	ScrollPageDown
	ScrollLineUp
	ScrollLineDown
	EnterCopyMode
	ExitCopyMode
	CopyMoveUp
	CopyMoveDown
	CopyMoveLeft
	CopyMoveRight
	CopyToggleSelect
	CopyYank
)

// Result is the outcome of feeding bytes: Forward goes to the pty; Actions are
// applied by main in order.
type Result struct {
	Forward []byte
	Actions []Action
}
```

- [ ] **Step 2: failing test** — `internal/input/interpreter_test.go`:
```go
package input

import (
	"bytes"
	"testing"
)

func TestNormalModeForwardsPlainBytes(t *testing.T) {
	it := New()
	r := it.Feed([]byte("ls -la\n"))
	if !bytes.Equal(r.Forward, []byte("ls -la\n")) {
		t.Fatalf("forward = %q, want 'ls -la\\n'", r.Forward)
	}
	if len(r.Actions) != 0 {
		t.Fatalf("actions = %v, want none", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("should stay in normal mode")
	}
}

func TestShiftPageUpEntersCopyModeAndScrolls(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[5;2~")) // Shift+PageUp
	if len(r.Forward) != 0 {
		t.Fatalf("Shift+PgUp must not be forwarded; got %q", r.Forward)
	}
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode || r.Actions[1] != ScrollPageUp {
		t.Fatalf("actions = %v, want [EnterCopyMode ScrollPageUp]", r.Actions)
	}
	if it.Mode() != ModeCopy {
		t.Fatal("should be in copy mode")
	}
}

func TestSplitEscapeSequenceBuffered(t *testing.T) {
	it := New()
	if r := it.Feed([]byte("\x1b[5")); len(r.Forward) != 0 || len(r.Actions) != 0 {
		t.Fatalf("partial CSI should buffer, not emit; got %+v", r)
	}
	r := it.Feed([]byte(";2~")) // completes Shift+PageUp
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode {
		t.Fatalf("completed sequence actions = %v", r.Actions)
	}
}

func TestUnrecognizedCSIForwardedInNormalMode(t *testing.T) {
	it := New()
	// F5 = ESC[15~ ; not a hotkey, must be forwarded verbatim to the shell.
	r := it.Feed([]byte("\x1b[15~"))
	if !bytes.Equal(r.Forward, []byte("\x1b[15~")) {
		t.Fatalf("unrecognized CSI forward = %q, want verbatim", r.Forward)
	}
}
```

- [ ] **Step 3: Run — expect FAIL.** `go test ./internal/input/ -v`

- [ ] **Step 4: implement** — `internal/input/interpreter.go`:
```go
package input

// Interpreter translates the input byte stream into forwarded bytes + actions.
// It buffers a partial escape sequence between Feed calls. Not safe for
// concurrent use; the event loop feeds it serially.
type Interpreter struct {
	mode Mode
	buf  []byte // pending bytes (a possibly-incomplete escape sequence)
}

// New returns an Interpreter in normal mode.
func New() *Interpreter { return &Interpreter{mode: ModeNormal} }

// Mode reports the current mode.
func (it *Interpreter) Mode() Mode { return it.mode }

const esc = 0x1b

// Feed consumes p and returns bytes to forward plus actions. A trailing
// incomplete escape sequence is retained for the next call.
func (it *Interpreter) Feed(p []byte) Result {
	it.buf = append(it.buf, p...)
	var res Result
	for len(it.buf) > 0 {
		tok, n, complete := nextToken(it.buf)
		if !complete {
			break // wait for more bytes
		}
		it.buf = it.buf[n:]
		it.handle(tok, &res)
	}
	return res
}

// token is one decoded unit of input.
type token struct {
	csi   bool   // true if this is an ESC[ ... sequence
	bytes []byte // the raw bytes of the token (the whole CSI, or one plain byte)
	final byte   // CSI final byte (e.g. '~', 'A', 'M', 'm'); 0 for plain
	params string // CSI parameter bytes between '[' and final (e.g. "5;2", "<64;1;1")
}

// nextToken decodes the next token from buf. complete=false means buf holds an
// incomplete sequence and the caller should wait for more bytes.
func nextToken(buf []byte) (tok token, consumed int, complete bool) {
	if buf[0] != esc {
		return token{bytes: buf[:1]}, 1, true // plain byte
	}
	// ESC...
	if len(buf) < 2 {
		return token{}, 0, false // lone ESC: wait (could be CSI lead)
	}
	if buf[1] != '[' {
		// ESC followed by something else: treat the ESC as a standalone byte
		// (Escape key). The following byte(s) are decoded on the next loop.
		return token{bytes: buf[:1]}, 1, true
	}
	// CSI: ESC [ params... final(0x40-0x7e)
	for i := 2; i < len(buf); i++ {
		b := buf[i]
		if b >= 0x40 && b <= 0x7e {
			return token{csi: true, bytes: buf[:i+1], final: b, params: string(buf[2:i])}, i + 1, true
		}
	}
	return token{}, 0, false // CSI not yet terminated
}

// handle applies one token to the result, possibly changing mode.
func (it *Interpreter) handle(tok token, res *Result) {
	if it.mode == ModeNormal {
		it.handleNormal(tok, res)
		return
	}
	it.handleCopy(tok, res)
}

func (it *Interpreter) handleNormal(tok token, res *Result) {
	if tok.csi {
		switch {
		case tok.final == '~' && tok.params == "5;2": // Shift+PageUp
			it.mode = ModeCopy
			res.Actions = append(res.Actions, EnterCopyMode, ScrollPageUp)
			return
		case tok.final == 'M' && isWheelUp(tok.params): // mouse wheel up
			it.mode = ModeCopy
			res.Actions = append(res.Actions, EnterCopyMode, ScrollLineUp)
			return
		case tok.final == 'M' || tok.final == 'm': // other mouse events: swallow
			return
		}
		res.Forward = append(res.Forward, tok.bytes...) // unrecognized CSI: forward
		return
	}
	res.Forward = append(res.Forward, tok.bytes...) // plain byte: forward
}
```
(`isWheelUp` and `handleCopy` are added in Tasks 2 and 3. For Task 1, add a temporary stub so the package compiles:)
```go
// TEMP: real implementations land in Tasks 2 (isWheelUp) and 3 (handleCopy).
func isWheelUp(string) bool { return false }
func (it *Interpreter) handleCopy(tok token, res *Result) { /* Task 3 */ }
```

- [ ] **Step 5: Run — expect PASS** (4 tests). gofmt, vet.
- [ ] **Step 6: Commit**
```bash
git add internal/input/doc.go internal/input/action.go internal/input/interpreter.go internal/input/interpreter_test.go
git commit -m "feat(input): interpreter skeleton, normal-mode forwarding, Shift+PageUp"
```

---

### Task 2: mouse SGR parsing

**Files:** Create `internal/input/mouse.go`; test `internal/input/mouse_test.go`. Modify `interpreter.go` only by removing the `isWheelUp` stub.

- [ ] **Step 1: failing tests** — `internal/input/mouse_test.go`:
```go
package input

import "testing"

func TestIsWheelUpDown(t *testing.T) {
	// SGR mouse params are "<button;col;row". Wheel up = 64, wheel down = 65.
	if !isWheelUp("<64;10;5") {
		t.Fatal("64 should be wheel up")
	}
	if isWheelUp("<65;10;5") {
		t.Fatal("65 is wheel down, not up")
	}
	if !isWheelDown("<65;10;5") {
		t.Fatal("65 should be wheel down")
	}
	if isWheelUp("<0;10;5") {
		t.Fatal("0 is left-click, not wheel")
	}
}

func TestWheelUpInNormalModeEntersCopy(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[<64;10;5M"))
	if it.Mode() != ModeCopy {
		t.Fatal("wheel up should enter copy mode")
	}
	if len(r.Actions) != 2 || r.Actions[0] != EnterCopyMode || r.Actions[1] != ScrollLineUp {
		t.Fatalf("actions = %v, want [EnterCopyMode ScrollLineUp]", r.Actions)
	}
	if len(r.Forward) != 0 {
		t.Fatalf("mouse event must not be forwarded; got %q", r.Forward)
	}
}

func TestLeftClickSwallowedNotForwarded(t *testing.T) {
	it := New()
	r := it.Feed([]byte("\x1b[<0;3;3M"))
	if len(r.Forward) != 0 || len(r.Actions) != 0 {
		t.Fatalf("unhandled mouse event should be swallowed; got %+v", r)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`isWheelUp` stub returns false; `isWheelDown` undefined).

- [ ] **Step 3: Remove the stub `isWheelUp` from interpreter.go** and implement `internal/input/mouse.go`:
```go
package input

import "strconv"

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
	semi := indexByte(rest, ';')
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

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt, vet, `go build ./...`.
- [ ] **Step 5: Commit**
```bash
git add internal/input/mouse.go internal/input/mouse_test.go internal/input/interpreter.go
git commit -m "feat(input): SGR mouse parsing and wheel scroll"
```

---

### Task 3: copy-mode key handling

**Files:** Modify `internal/input/interpreter.go` (replace the `handleCopy` stub); test `interpreter_test.go` (extend).

- [ ] **Step 1: failing tests (append)**:
```go
func feedCopy(t *testing.T) *Interpreter {
	t.Helper()
	it := New()
	it.Feed([]byte("\x1b[5;2~")) // enter copy mode
	if it.Mode() != ModeCopy {
		t.Fatal("setup: expected copy mode")
	}
	return it
}

func TestCopyModeMotionKeys(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("jjkl")) // down down up right (vim keys)
	want := []Action{CopyMoveDown, CopyMoveDown, CopyMoveUp, CopyMoveRight}
	if len(r.Actions) != len(want) {
		t.Fatalf("actions = %v, want %v", r.Actions, want)
	}
	for i := range want {
		if r.Actions[i] != want[i] {
			t.Fatalf("action[%d] = %v, want %v", i, r.Actions[i], want[i])
		}
	}
	if len(r.Forward) != 0 {
		t.Fatalf("copy mode must capture, not forward; got %q", r.Forward)
	}
}

func TestCopyModeArrowKeys(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[A\x1b[B\x1b[C\x1b[D")) // up down right left
	want := []Action{CopyMoveUp, CopyMoveDown, CopyMoveRight, CopyMoveLeft}
	for i := range want {
		if r.Actions[i] != want[i] {
			t.Fatalf("arrow action[%d] = %v, want %v", i, r.Actions[i], want[i])
		}
	}
}

func TestCopyModeSelectAndYank(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("vy")) // toggle select, yank
	if len(r.Actions) != 3 || r.Actions[0] != CopyToggleSelect || r.Actions[1] != CopyYank || r.Actions[2] != ExitCopyMode {
		t.Fatalf("actions = %v, want [CopyToggleSelect CopyYank ExitCopyMode]", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("yank should return to normal mode")
	}
}

func TestCopyModeQuitExits(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("q"))
	if len(r.Actions) != 1 || r.Actions[0] != ExitCopyMode {
		t.Fatalf("actions = %v, want [ExitCopyMode]", r.Actions)
	}
	if it.Mode() != ModeNormal {
		t.Fatal("q should exit copy mode")
	}
}

func TestCopyModePageScroll(t *testing.T) {
	it := feedCopy(t)
	r := it.Feed([]byte("\x1b[6~")) // PageDown
	if len(r.Actions) != 1 || r.Actions[0] != ScrollPageDown {
		t.Fatalf("actions = %v, want [ScrollPageDown]", r.Actions)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (stub `handleCopy` does nothing).

- [ ] **Step 3: replace the `handleCopy` stub** in interpreter.go:
```go
func (it *Interpreter) handleCopy(tok token, res *Result) {
	if tok.csi {
		switch {
		case tok.final == 'A':
			res.Actions = append(res.Actions, CopyMoveUp)
		case tok.final == 'B':
			res.Actions = append(res.Actions, CopyMoveDown)
		case tok.final == 'C':
			res.Actions = append(res.Actions, CopyMoveRight)
		case tok.final == 'D':
			res.Actions = append(res.Actions, CopyMoveLeft)
		case tok.final == '~' && (tok.params == "5" || tok.params == "5;2"): // PageUp / Shift+PageUp
			res.Actions = append(res.Actions, ScrollPageUp)
		case tok.final == '~' && (tok.params == "6" || tok.params == "6;2"): // PageDown / Shift+PageDown
			res.Actions = append(res.Actions, ScrollPageDown)
		case tok.final == 'M' && isWheelUp(tok.params):
			res.Actions = append(res.Actions, ScrollLineUp)
		case tok.final == 'M' && isWheelDown(tok.params):
			res.Actions = append(res.Actions, ScrollLineDown)
		}
		return // all other CSI in copy mode: ignored (captured)
	}
	// plain byte commands
	switch tok.bytes[0] {
	case 'k':
		res.Actions = append(res.Actions, CopyMoveUp)
	case 'j':
		res.Actions = append(res.Actions, CopyMoveDown)
	case 'h':
		res.Actions = append(res.Actions, CopyMoveLeft)
	case 'l':
		res.Actions = append(res.Actions, CopyMoveRight)
	case 'v', ' ':
		res.Actions = append(res.Actions, CopyToggleSelect)
	case 'y', '\r':
		res.Actions = append(res.Actions, CopyYank, ExitCopyMode)
		it.mode = ModeNormal
	case 'q', esc:
		res.Actions = append(res.Actions, ExitCopyMode)
		it.mode = ModeNormal
	}
	// any other byte in copy mode: ignored (captured, not forwarded)
}
```

- [ ] **Step 4: Run the full input suite + race + vet + build**
Run: `go test -race ./internal/input/ -v` (all pass), `go vet ./internal/input/`, `go build ./...`.

- [ ] **Step 5: Commit**
```bash
git add internal/input/interpreter.go internal/input/interpreter_test.go
git commit -m "feat(input): copy-mode motion, selection, yank, scroll, exit"
```

---

### Task 4: clip package (clipboard sink)

**Files:** Create `internal/clip/clip.go`; test `internal/clip/clip_test.go`.

- [ ] **Step 1: failing tests** — `internal/clip/clip_test.go`:
```go
package clip

import (
	"strings"
	"testing"
)

func TestOSC52EncodesBase64(t *testing.T) {
	got := OSC52("hi")
	// ESC ] 52 ; c ; <base64> BEL ; base64("hi") = "aGk="
	if got != "\x1b]52;c;aGk=\x07" {
		t.Fatalf("OSC52 = %q", got)
	}
}

func TestOSC52Empty(t *testing.T) {
	if got := OSC52(""); got != "\x1b]52;c;\x07" {
		t.Fatalf("OSC52(empty) = %q", got)
	}
}

func TestCopyToWriterUsesOSC52(t *testing.T) {
	var b strings.Builder
	if err := CopyTo(&b, "ok"); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if !strings.Contains(b.String(), "\x1b]52;c;") {
		t.Fatalf("CopyTo should emit OSC52; got %q", b.String())
	}
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/clip/ -v`

- [ ] **Step 3: implement** — `internal/clip/clip.go`:
```go
// Package clip writes text to the system clipboard. On macOS it shells out to
// pbcopy; everywhere it can emit an OSC 52 escape that terminals forward to the
// clipboard, which CopyTo uses for the in-terminal path.
package clip

import (
	"encoding/base64"
	"io"
	"os/exec"
	"runtime"
)

// OSC52 returns the OSC 52 clipboard-set escape for s (base64-encoded).
func OSC52(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	return "\x1b]52;c;" + enc + "\x07"
}

// CopyTo writes s to the clipboard by emitting OSC 52 to w (the terminal). Use
// this from the render path where w is the real terminal.
func CopyTo(w io.Writer, s string) error {
	_, err := io.WriteString(w, OSC52(s))
	return err
}

// Copy writes s to the clipboard out-of-band. On macOS it uses pbcopy; on other
// platforms it returns ErrNoNativeClipboard so callers fall back to CopyTo
// (OSC 52). Keeping this separate lets main choose per platform.
func Copy(s string) error {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = stringsReader(s)
		return cmd.Run()
	}
	return ErrNoNativeClipboard
}

// ErrNoNativeClipboard signals no native clipboard tool; fall back to OSC 52.
var ErrNoNativeClipboard = errNoClip{}

type errNoClip struct{}

func (errNoClip) Error() string { return "clip: no native clipboard; use OSC 52" }

func stringsReader(s string) io.Reader { return &sr{s: s} }

type sr struct {
	s string
	i int
}

func (r *sr) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
```
> Note: `stringsReader` reimplements a tiny reader to avoid importing `strings` solely for `strings.NewReader` — if you prefer, use `strings.NewReader(s)` and import `strings` instead; either is fine. (Pick one and keep the test import consistent.)

- [ ] **Step 4: Run — expect PASS.** gofmt, vet, build.
- [ ] **Step 5: Commit**
```bash
git add internal/clip/clip.go internal/clip/clip_test.go
git commit -m "feat(clip): OSC52 + pbcopy clipboard sink"
```

---

## Self-Review

**Spec coverage (input portion):**
- "forward keys to pty; intercept HUD hotkeys (scroll, copy-mode)" → Tasks 1-3 ✓
- scroll via Shift+PageUp/Down + mouse wheel → Tasks 1, 2 ✓
- copy-mode selection + yank to clipboard (pbcopy/OSC52) → Task 3 (actions) + Task 4 (clip sink) ✓ (text extraction from grid + invoking clip is main's wiring — documented)

**Design decisions surfaced (review):** tmux-style copy-mode; `q` reliable exit, lone-`Esc` recognized on next byte (no input timeout in v1); scroll auto-enters copy-mode. These are in the "Copy-mode UX" section.

**Placeholder scan:** Task 1 adds clearly-labeled TEMP stubs (`isWheelUp`, `handleCopy`) removed/replaced in Tasks 2/3 — cross-task TDD, same pattern as prior plans.

**Type consistency:** `New()`, `(*Interpreter).Feed([]byte) Result`, `.Mode() Mode`; `Result{Forward []byte; Actions []Action}`; `Action` constants; `mouseButton/isWheelUp/isWheelDown`; `EnableMouse/DisableMouse` consts; `clip.OSC52/CopyTo/Copy`. No external deps.

**Known limitations (documented):**
- Lone `Esc` needs a following byte to be recognized (no timeout); `q` is the reliable exit.
- No mouse drag-selection in v1 (wheel scroll only); selection is keyboard-driven. Drag-select is a follow-up.
- main owns: scrollOffset math (clamp), the copy cursor + selection coords, extracting selected text from engine grid+scrollback, and calling clip (Copy then CopyTo fallback). Plus writing `input.EnableMouse`/`DisableMouse` at startup/shutdown.

**Follow-ups (later):** main wires Feed→pty/actions, owns selection + text extraction + clip; mouse drag-select; configurable keybindings.
