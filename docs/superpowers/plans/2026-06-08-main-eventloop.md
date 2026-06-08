# main / session Implementation Plan (Plan 7 of N — the runnable binary)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make terminal-hud a runnable binary: a testable `session` package that owns scroll state and builds frames (compositor + hud bars), and a thin `cmd/terminal-hud` main that sets up the terminal, spawns the shell, and runs the event loop wiring pty ↔ engine ↔ session ↔ render, with input forwarding, scroll, SIGWINCH, a HUD refresh ticker, and guaranteed terminal restore.

**Architecture:** `session` is pure/testable (no real I/O): it holds `rows/cols/scrollOffset/hud.Deps`, applies `input.Action`s (scroll), and produces a `*render.Frame` via `compositor.Compose` + `hud.TopBar/BottomBar`. It reads the shell grid through `compositor.GridSource` (engine satisfies it). `cmd/terminal-hud` is the I/O shell: raw mode, alt-screen + mouse enable, pty via ptyhost, engine sized to the middle, goroutines feeding a single event loop that owns the (non-thread-safe) engine and session, and `defer`+panic-recover terminal restore.

**Tech Stack:** Go 1.26; new dep `golang.org/x/term` (Go-team maintained; raw mode + size). Imports all prior internal packages.

**Prerequisite:** engine, ptyhost, render, compositor, hud, input, clip on `main` (they are).

**Scope (this plan):** runnable binary with **scrolling**. **Deferred to Plan 7b:** wiring copy-mode (cursor, selection, `SelectionText`, `clip` yank) and OSC (path/exit). The input module already emits copy-mode actions; session will accept and currently no-op the non-scroll ones (documented), except `ExitCopyMode`/`CopyYank` which reset scroll to live.

---

## File Structure

```
go.mod / go.sum               ← add golang.org/x/term
internal/session/
  session.go                  ← Session: state, Apply(action), Frame(), Resize, SetDeps
  session_test.go
cmd/terminal-hud/
  main.go                     ← terminal setup + event loop (I/O shell)
  loop.go                     ← the select loop, split out for clarity (optional)
```

---

### Task 1: session package — scroll state + Frame

**Files:** Create `internal/session/session.go`; test `internal/session/session_test.go`.

- [ ] **Step 1: failing tests** — `internal/session/session_test.go`:
```go
package session

import (
	"testing"

	"terminal-hud/internal/engine"
	"terminal-hud/internal/hud"
)

// fakeGrid satisfies compositor.GridSource for tests.
type fakeGrid struct {
	rows, cols int
	sb         int // scrollback length
}

func (g *fakeGrid) Size() (int, int)               { return g.rows, g.cols }
func (g *fakeGrid) Cell(r, c int) engine.Cell      { return engine.Cell{Rune: 'x', Width: 1} }
func (g *fakeGrid) CursorPos() (int, int)          { return 0, 0 }
func (g *fakeGrid) ScrollbackLen() int             { return g.sb }
func (g *fakeGrid) ScrollbackLine(n int) []engine.Cell {
	return []engine.Cell{{Rune: 's', Width: 1}}
}

func TestNewSessionLiveScroll(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 100}, 10, 20)
	if s.ScrollOffset() != 0 {
		t.Fatalf("initial scrollOffset = %d, want 0", s.ScrollOffset())
	}
}

func TestApplyScrollClampsToScrollbackLen(t *testing.T) {
	g := &fakeGrid{rows: 8, cols: 20, sb: 3}
	s := New(g, 10, 20)
	for i := 0; i < 10; i++ {
		s.Apply(actionScrollLineUp())
	}
	if s.ScrollOffset() != 3 {
		t.Fatalf("scrollOffset = %d, want clamped to 3", s.ScrollOffset())
	}
}

func TestApplyScrollDownClampsToZero(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 50}, 10, 20)
	s.Apply(actionScrollLineDown()) // already at 0
	if s.ScrollOffset() != 0 {
		t.Fatalf("scrollOffset = %d, want 0", s.ScrollOffset())
	}
}

func TestExitCopyModeResetsToLive(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 50}, 10, 20)
	s.Apply(actionScrollPageUp())
	if s.ScrollOffset() == 0 {
		t.Fatal("page up should scroll")
	}
	s.Apply(actionExitCopyMode())
	if s.ScrollOffset() != 0 {
		t.Fatalf("exit copy should reset to live, got %d", s.ScrollOffset())
	}
}

func TestFrameHasBarsAndDimensions(t *testing.T) {
	s := New(&fakeGrid{rows: 8, cols: 20, sb: 0}, 10, 20)
	s.SetDeps(hud.Deps{Time: "12:00:00"})
	f := s.Frame()
	if f.Rows != 10 || f.Cols != 20 {
		t.Fatalf("frame dims = %dx%d, want 10x20", f.Rows, f.Cols)
	}
	// top bar row 0 should contain the time text's first rune
	if f.At(0, 0).Rune != '1' {
		t.Fatalf("top bar not rendered; (0,0)=%q", f.At(0, 0).Rune)
	}
}
```
(`actionScrollLineUp()` etc. are tiny helpers returning the `input.Action` constants; define them at the bottom of the test file:)
```go
import "terminal-hud/internal/input"

func actionScrollLineUp() input.Action   { return input.ScrollLineUp }
func actionScrollLineDown() input.Action { return input.ScrollLineDown }
func actionScrollPageUp() input.Action   { return input.ScrollPageUp }
func actionExitCopyMode() input.Action   { return input.ExitCopyMode }
```
(Merge that import into the test's import block.)

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/session/ -v`

- [ ] **Step 3: implement** — `internal/session/session.go`:
```go
// Package session owns the interactive state layered over the shell's grid:
// the scrollback offset (and, in a later plan, copy-mode cursor/selection). It
// builds a render.Frame each time the screen needs drawing, combining the
// compositor's three-region layout with the hud bars. It is pure: the shell
// grid is read through compositor.GridSource, so session is unit-testable.
package session

import (
	"terminal-hud/internal/compositor"
	"terminal-hud/internal/hud"
	"terminal-hud/internal/input"
	"terminal-hud/internal/render"
)

// Session holds interactive view state over a GridSource.
type Session struct {
	src        compositor.GridSource
	rows, cols int // full screen dimensions (including both bars)
	scrollOff  int
	deps       hud.Deps
}

// New returns a Session for a rows x cols screen reading from src (live view).
func New(src compositor.GridSource, rows, cols int) *Session {
	return &Session{src: src, rows: rows, cols: cols}
}

// Resize updates the full-screen dimensions. The caller resizes the engine to
// the middle (rows-2 x cols) separately.
func (s *Session) Resize(rows, cols int) {
	s.rows, s.cols = rows, cols
	s.clampScroll()
}

// SetDeps sets the HUD segment strings used for the next Frame.
func (s *Session) SetDeps(d hud.Deps) { s.deps = d }

// ScrollOffset reports the current scrollback offset (0 = live).
func (s *Session) ScrollOffset() int { return s.scrollOff }

// midHeight is the number of shell-view rows between the two bars.
func (s *Session) midHeight() int {
	if s.rows < 2 {
		return 0
	}
	return s.rows - 2
}

// Apply mutates session state for a HUD action. Scroll actions move the
// scrollback offset (clamped); ExitCopyMode/CopyYank return to the live view.
// Copy-mode cursor/selection actions are accepted but not yet acted on (Plan 7b).
func (s *Session) Apply(a input.Action) {
	page := s.midHeight() - 1
	if page < 1 {
		page = 1
	}
	switch a {
	case input.ScrollLineUp:
		s.scrollOff++
	case input.ScrollLineDown:
		s.scrollOff--
	case input.ScrollPageUp:
		s.scrollOff += page
	case input.ScrollPageDown:
		s.scrollOff -= page
	case input.ExitCopyMode, input.CopyYank:
		s.scrollOff = 0
	default:
		// EnterCopyMode, CopyMove*, CopyToggleSelect: no-op until Plan 7b
	}
	s.clampScroll()
}

func (s *Session) clampScroll() {
	if s.scrollOff < 0 {
		s.scrollOff = 0
	}
	if max := s.src.ScrollbackLen(); s.scrollOff > max {
		s.scrollOff = max
	}
}

// Frame composes the current screen: top/bottom HUD bars and the shell view at
// the current scroll offset.
func (s *Session) Frame() *render.Frame {
	top := hud.TopBar(s.cols, s.deps)
	bottom := hud.BottomBar(s.cols, s.deps)
	return compositor.Compose(s.rows, s.cols, s.src, s.scrollOff, top, bottom)
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt, vet, build.
- [ ] **Step 5: Commit**
```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat(session): scroll state and frame composition"
```

---

### Task 2: add golang.org/x/term dependency

**Files:** Modify `go.mod`/`go.sum`.

- [ ] **Step 1:** Run `go get golang.org/x/term@latest`. Confirm `go.mod` gains the require.
- [ ] **Step 2:** Sanity-build: `go build ./...` (still clean; nothing imports it yet).
- [ ] **Step 3: Commit**
```bash
git add go.mod go.sum
git commit -m "build: add golang.org/x/term for raw mode and terminal size"
```

---

### Task 3: cmd/terminal-hud main — terminal setup + event loop

**Files:** Create `cmd/terminal-hud/main.go`.

This task is the I/O shell. It is integration code; correctness is verified by building and a manual run (it takes over the terminal), plus a compile-time check. Keep logic minimal — all stateful decisions live in `session`/`engine`/`input`, which are already tested.

- [ ] **Step 1: implement** — `cmd/terminal-hud/main.go`:
```go
// Command terminal-hud runs the user's shell inside a screen-owning terminal
// with frozen top/bottom HUD bars and a scrollable middle.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"terminal-hud/internal/engine"
	"terminal-hud/internal/hud"
	"terminal-hud/internal/hud/module"
	"terminal-hud/internal/input"
	"terminal-hud/internal/ptyhost"
	"terminal-hud/internal/render"
	"terminal-hud/internal/session"
)

const scrollbackLines = 10000

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "terminal-hud:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	stdin := int(os.Stdin.Fd())
	if !term.IsTerminal(stdin) {
		return fmt.Errorf("stdin is not a terminal")
	}

	// Raw mode + alt-screen + mouse. Restore on any exit path, including panic.
	oldState, err := term.MakeRaw(stdin)
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	restore := func() {
		os.Stdout.WriteString(input.DisableMouse + "\x1b[?25h\x1b[?1049l")
		term.Restore(stdin, oldState)
	}
	defer restore()
	defer func() {
		if r := recover(); r != nil {
			restore()
			panic(r) // re-panic after restoring the terminal
		}
	}()
	os.Stdout.WriteString("\x1b[?1049h" + input.EnableMouse + "\x1b[?25l")

	cols, rows, err := term.GetSize(stdin)
	if err != nil {
		return fmt.Errorf("size: %w", err)
	}
	mid := rows - 2
	if mid < 1 {
		mid = 1
	}

	eng, err := engine.New(mid, cols, scrollbackLines)
	if err != nil {
		return err
	}
	defer eng.Close()

	ph, err := ptyhost.Start(ptyhost.ResolveShell(""), nil, mid, cols)
	if err != nil {
		return err
	}
	defer ph.Close()

	sess := session.New(eng, rows, cols)
	rd := render.NewRenderer(os.Stdout)
	interp := input.New()

	// Channels feeding the single event loop (which owns eng + sess).
	ptyBytes := make(chan []byte, 64)
	stdinBytes := make(chan []byte, 64)
	go pump(ph, ptyBytes)
	go pump(os.Stdin, stdinBytes)

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	renderTick := time.NewTicker(time.Second) // clock + coalesced redraw
	defer renderTick.Stop()
	hudTick := time.NewTicker(2 * time.Second)
	defer hudTick.Stop()

	refresh := newHUDRefresher()
	sess.SetDeps(refresh.snapshot())
	rd.Render(sess.Frame())

	for {
		select {
		case b, ok := <-ptyBytes:
			if !ok {
				return nil // shell exited / pty closed
			}
			eng.Write(b)
			rd.Render(sess.Frame())

		case b := <-stdinBytes:
			res := interp.Feed(b)
			if len(res.Forward) > 0 {
				ph.Write(res.Forward)
			}
			for _, a := range res.Actions {
				sess.Apply(a)
			}
			if len(res.Actions) > 0 {
				rd.Render(sess.Frame())
			}

		case <-sigwinch:
			if c, r, e := term.GetSize(stdin); e == nil {
				cols, rows = c, r
				m := rows - 2
				if m < 1 {
					m = 1
				}
				eng.Resize(m, cols)
				ph.Resize(m, cols)
				sess.Resize(rows, cols)
				rd = render.NewRenderer(os.Stdout) // force full redraw at new size
				rd.Render(sess.Frame())
			}

		case <-hudTick.C:
			refresh.update()
			sess.SetDeps(refresh.snapshot())

		case <-renderTick.C:
			refresh.tickClock()
			sess.SetDeps(refresh.snapshot())
			rd.Render(sess.Frame())
		}
	}
}

// pump reads from r and sends copied chunks to ch until r errors/EOF, then
// closes ch.
func pump(r interface{ Read([]byte) (int, error) }, ch chan<- []byte) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			cp := make([]byte, n)
			copy(cp, buf[:n])
			ch <- cp
		}
		if err != nil {
			close(ch)
			return
		}
	}
}
```

- [ ] **Step 2: implement the HUD refresher** — append to `main.go` (or a `hud_refresh.go` in the same package):
```go
// hudRefresher computes HUD segment strings off the render path. update() runs
// the (possibly slow, cached) segment fetches; tickClock refreshes only the
// clock; snapshot returns the current Deps.
type hudRefresher struct {
	deps hud.Deps
	dial module.DialFunc
}

func newHUDRefresher() *hudRefresher {
	r := &hudRefresher{}
	r.tickClock()
	r.update()
	return r
}

func (r *hudRefresher) tickClock() { r.deps.Time = module.Time(time.Now()) }

func (r *hudRefresher) update() {
	r.deps.Time = module.Time(time.Now())
	r.deps.LocalIP = module.LocalIP(net_Dial)
	// extip/weather/azure use the package cache via the module defaults; for the
	// runnable milestone we call them directly with the default fetchers. They
	// are cheap when cached. (A shared *cache.Cache is wired here.)
	// NOTE: see Step 3 — these use a package-level cache.
}

func (r *hudRefresher) snapshot() hud.Deps { return r.deps }
```
> IMPLEMENTATION NOTE (resolve in Step 3): wire the real segment calls with a shared `*cache.Cache` and the default fetchers/runners: `module.ExtIP(c, module.DefaultExtIPFetch)`, `module.Weather(c, module.DefaultWeatherFetch)`, `module.Git(module.DefaultGitRun(cwd))`, `module.Azure(c, module.DefaultAzRun)`, `module.K8s(kubeconfigPath())`, `module.LocalIP(net.Dial)`. `cwd` is the process cwd for now (OSC 7 lands with the path segment in a later plan); `kubeconfigPath()` honors `$KUBECONFIG` first entry else `~/.kube/config`. Replace the `net_Dial` placeholder with `net.Dial` (import `net`). The `update()` body above is a sketch — fill in the real calls.

- [ ] **Step 3: Flesh out `update()` with the real segment calls** (per the note), add the `net`, `cache`, `os`, `path/filepath`, `strings` imports as needed, and a shared `cache.New()` stored on `hudRefresher`. Each segment failure returns "" (already the module contract), so the bar simply omits it.

- [ ] **Step 4: Build + vet.**
Run: `go build ./...` and `go vet ./...`. Expected: clean. (No unit test for main; it is verified by running.)

- [ ] **Step 5: Manual smoke (the implementer reports, does not automate):**
Run: `go build -o /tmp/terminal-hud ./cmd/terminal-hud` — confirm it builds. Note that running it takes over the terminal; do not run it inside the agent's non-interactive shell (it requires a real TTY and will error "stdin is not a terminal", which is the correct guard).

- [ ] **Step 6: Commit**
```bash
git add cmd/terminal-hud/
git commit -m "feat(main): terminal setup and event loop (runnable; scroll wired)"
```

---

## Self-Review

**Spec coverage (main/event-loop portion):**
- pty hosts $SHELL; bytes pumped into engine; render loop draws composited frame → Task 3 ✓
- frozen bars + scrolling middle via session/compositor/render → Tasks 1, 3 ✓
- input forwarded to pty; scroll actions applied → Task 3 ✓
- SIGWINCH resizes pty + engine + layout + full redraw → Task 3 ✓
- HUD refresh on a ticker (not per-prompt), off the render path → Task 3 (hudRefresher) ✓
- terminal restored on exit AND panic (sacred invariant) → Task 3 (defer restore + recover) ✓
- coalesced redraw (render on pty/input/tick, not per byte beyond the chunk) → Task 3 ✓

**Deferred (documented):** copy-mode cursor/selection/`SelectionText`/clip yank wiring (session no-ops the non-scroll copy actions for now) → Plan 7b; OSC 7 path + exit segments → engine-OSC plan; mouse drag-select.

**Placeholder scan:** the `update()` body in Task 2's sketch and `net_Dial` are explicitly flagged to be completed in Task 3 Step 3 (with the exact module calls listed). This is a real instruction, not a silent placeholder — Task 3 must produce compiling code with no `net_Dial`.

**Type/dep notes:** `golang.org/x/term` API used: `term.IsTerminal(fd) bool`, `term.MakeRaw(fd) (*term.State, error)`, `term.Restore(fd, *State) error`, `term.GetSize(fd) (w, h int, err error)`. `engine.New(rows, cols, sbLines)`, `(*Engine).Write/Resize/Close`; `ptyhost.Start(name, args, rows, cols)`, `.Read/Write/Resize/Close`; `session.New(src, rows, cols)`, `.Apply/Frame/Resize/SetDeps`; `render.NewRenderer(w)`, `.Render(*Frame)`; `input.New()`, `.Feed`; module fns + `cache.New`.

**Testing posture:** `session` is fully unit-tested (Task 1). `main` is integration glue verified by `go build` + manual run; the constitution's smoke-test note applies — a true end-to-end test needs a controlling TTY, which is a follow-up (could use a pty in the test to drive it). Flagged rather than faked.

**Known risks to verify on first run:** rendering throughput under heavy shell output (coalescing currently renders per pty chunk — may need a dirty-flag + single render per loop iteration if flickery); cursor placement; mouse escape leakage if a terminal lacks SGR mouse support.
