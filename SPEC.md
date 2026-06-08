# terminal-hud design spec
**Version:** 3.1.0
**Target:** macOS (primary), Linux (secondary) · Go + cgo · libvterm state engine · genuinely pinned bars

---

## What changed from v3.0 and why

v3.0 specified the same screen-owning architecture but sourced its VT state engine from **libghostty-vt** via the `go-libghostty` cgo bindings. Phase 0 (the go/no-go gate) was executed and **libghostty-vt failed to build on the target Mac** — not because of its API, but because of its toolchain. The full record is in `PHASE0.md`; the short version:

- go-libghostty's pinned ghostty source requires **Zig exactly 0.15.2**.
- Zig 0.15.2's only macOS linker (its self-hosted Mach-O linker) is **incompatible with the macOS 26 (Tahoe) SDK** — it emits libSystem `undefined symbol` errors and produces no library, with no fallback linker available.
- This is unresolvable on the current machine without upstream changes. It is exactly the "find out cheaply" outcome Phase 0 exists to surface.

v3.1 keeps the v3 architecture verbatim and **swaps the engine to libvterm** — the C99 VT state engine that powers Neovim's `:terminal`. A Phase 0b proof (also in `PHASE0.md`) confirmed libvterm builds, links, and runs through Go cgo on the target macOS 26 machine, parsing an SGR-colored byte stream and returning the grid (`"Hello, world!"`). libvterm is a verified GO.

The tradeoff: libvterm is a lower-level engine than libghostty-vt. It hands us lines as they scroll off (via callbacks) but does not store scrollback itself, does not provide selection helpers, and does not reflow scrollback history on resize. In the screen-owning model those were partly our responsibility anyway. We accept owning a scrollback ring buffer and selection logic in exchange for a stable, ubiquitous, system-packaged dependency with no exotic toolchain.

---

## Honest constraints the implementer must accept up front

1. **This program owns the screen.** It is, by definition, a terminal emulator. It does NOT reimplement VT parsing / grid state — that is libvterm's job. It supplies the novel parts: a three-region layout (frozen top row, scrolling middle, frozen bottom row), a pty hosting the user's shell, a render loop, a scrollback store, selection, and the HUD module data.

2. **libvterm is a state engine, not a renderer or a pty.** It parses VT bytes into a grid and calls back when lines scroll off. It does NOT draw pixels, open a window, or spawn a shell. This program provides the pty, the render-to-screen loop, input handling, and scrollback storage.

3. **libvterm must be installed before `go build`.** It is linked via cgo + `pkg-config`. Install: macOS `brew install libvterm`; Debian/Ubuntu `apt install libvterm-dev`. The build needs `pkg-config --exists vterm` to succeed. Pinned/verified version: **0.3.3** (API stable since 0.3).

4. **No scrollback reflow on resize.** libvterm reflows the active screen on width change but NOT scrollback history (a documented libvterm limitation: the `sb_pushline` callback does not carry soft-wrap info). Accepted: resizing leaves already-scrolled-off lines wrapped at their previous width. See Known limitations.

5. **This is the larger project** (unchanged from v3.0). It is a standalone binary you run instead of (or launched by) your terminal, that runs your shell inside it. Accepting this scope is the price of true pinning. The fallback remains v2.1 (scrolling top bar, trivially simple), a different point on the tradeoff curve, not a failure.

---

## Architecture

```
your shell (zsh)  ──pty──▶  terminal-hud (this program)
                                │
                                ├── ptyhost: open pty, spawn $SHELL, pump bytes, handle SIGWINCH
                                ├── engine (cgo over libvterm):
                                │     • vterm_input_write(shell bytes) → parse into grid
                                │     • read grid via vterm_screen_get_cell
                                │     • sb_pushline/sb_popline → OUR scrollback ring buffer
                                │     • OSC hook: OSC 7 (cwd), custom OSC (exit code)
                                ├── compositor: top row | middle (shell view + scroll offset) | bottom row
                                ├── render: diff composited frame → draw to real terminal
                                ├── input: forward keys to pty; intercept hotkeys (scroll, copy-mode)
                                └── hud modules: time, ip, weather, git, azure, k8s, path, exit
```

- libvterm holds the authoritative grid of the shell's current screen.
- The **engine module owns the scrollback ring buffer** (default cap 10,000 lines, configurable), fed by libvterm's `sb_pushline` callback. This is what lets the middle region scroll back while the bars stay frozen.
- The compositor reserves row 0 (top bar) and row N-1 (bottom bar); the shell's view maps to rows 1..N-2.
- Copy-paste: selection operates on the engine's grid + ring buffer for the middle region only; the bars are excluded by construction.

---

## Phase 0 — DONE (go/no-go executed; see PHASE0.md)

Phase 0 has already run. Outcome:

- **libghostty-vt: NO-GO** on macOS 26 — Zig 0.15.2 cannot link Mach-O on this SDK (toolchain, not API). Recorded with full evidence in `PHASE0.md`.
- **libvterm: GO, verified** — `brew install libvterm` (0.3.3) + a Go cgo program built and ran on macOS 26, parsed an SGR stream, and read the grid back. Recorded as Phase 0b in `PHASE0.md`.

No further gating is required to start v3.1. Re-verify only if the build host changes materially (different OS, no libvterm available).

---

## Repository layout (v3.1)

```
terminal-hud/
  cmd/
    terminal-hud/
      main.go            ← parse flags, set up pty, run event loop
  internal/
    ptyhost/
      ptyhost.go         ← open pty, spawn $SHELL, pump bytes, handle SIGWINCH
    engine/
      engine.go          ← cgo over libvterm: feed bytes, read grid, expose scrollback
      scrollback.go      ← bounded ring buffer fed by sb_pushline; serves sb_popline + history reads
      callbacks.go       ← //export'd cgo callbacks + cgo.Handle routing (the one unsafe-adjacent unit)
      osc.go             ← OSC 7 (cwd) + custom exit-code OSC parsing → Events()
    compositor/
      compositor.go      ← three-region layout math; map shell view + scroll offset to rows 1..N-2
    render/
      render.go          ← diff + draw composited frame to real terminal
    input/
      input.go           ← forward keys to pty; intercept HUD hotkeys (scroll, copy-mode)
    hud/
      bar_top.go         ← assemble top bar string
      bar_bottom.go      ← assemble bottom bar string
      module/
        time.go localip.go extip.go weather.go git.go azure.go k8s.go path.go exit.go
    cache/
      cache.go           ← atomic read/write, mtime TTL  (carried from v2.1, unchanged)
    lock/
      lock.go            ← single-flight refresh lock     (carried from v2.1, unchanged)
    color/
      color.go           ← named ANSI fg for chrome; passthrough for shell content
  go.mod                 ← no exotic deps; cgo links system libvterm via pkg-config
  Makefile
  PHASE0.md              ← recorded Phase 0 results (libghostty-vt NO-GO, libvterm GO)
```

---

## Visual layout (unchanged from the approved mockup)

Top bar — frozen row 0, full width:
```
14:32:07  │  192.168.1.42  │  72.14.201.9              ☀ Atlanta 84°F
```

Bottom bar — frozen row N-1, full width:
```
~/SET-Sandbox/next  │   main ✓ +2  │  ⬡ jmf-prod  │  ⎈ aks-dev/default
```

Shell runs in rows 1..N-2, scrolls naturally, with scrollback served from the engine's ring buffer. The prompt `❯` is emitted by the user's own zsh inside the shell, NOT by this program — the bars are HUD chrome, the prompt belongs to the shell.

Because the shell lives inside this program, the bottom-bar data (path, git, azure, k8s, exit) is gathered by THIS program, not the shell. Path comes from OSC 7 (if the shell emits it); exit code comes from a custom OSC emitted by an opt-in zsh precmd snippet (see HUD modules and Open questions).

---

## The `engine` module (the part that changed)

Go-facing interface (engine hides all cgo):
```go
type Cell  struct { Rune rune; FG, BG Color; Attrs Attr }
type Event struct { Kind EventKind; Str string; Int int } // OSC7Cwd, ExitCode, Bell, Title

type Engine interface {
    Write(p []byte) (int, error)   // io.Writer; ptyhost pumps shell bytes in
    Resize(rows, cols int)         // SIGWINCH → vterm_set_size for middle region
    Cell(row, col int) Cell        // visible grid cell
    CursorPos() (row, col int)
    ScrollbackLen() int
    ScrollbackLine(n int) []Cell
    Events() <-chan Event
    Close() error
}
```

Key implementation facts:
- **Scrollback ring buffer:** bounded `[][]Cell`, default cap 10,000 lines (configurable via flag/env). libvterm's `sb_pushline` hands off cells from the top of the screen; we **copy** them into the ring (libvterm reuses its buffer — copying is mandatory). `sb_popline` is served from the ring on scroll-down.
- **cgo callback boundary:** `sb_pushline`/`sb_popline` are C function pointers calling into Go; routed to the correct `*engine` via a `cgo.Handle` in libvterm's `user` pointer, using `//export`ed functions. Isolated in `callbacks.go` and tested carefully — the only unsafe-adjacent code.
- **Color mapping:** `VTermColor` → `Color`. Bar chrome uses named ANSI indices (host theme owns RGB); shell content passes through faithfully (indexed/RGB as emitted). The render layer turns `Color`/`Attr` into SGR.
- **OSC:** intercept via libvterm's parser-level OSC callback (reassembles fragments). Parse OSC 7 (cwd) and a custom OSC code carrying `$?`. *Implementation note: confirm the exact OSC callback signature against `vterm.h` at build time; the grid + scrollback API was verified during Phase 0b, the OSC callback was not.*

---

## HUD modules (logic carried from v2.1, sourcing noted)

Each module returns `string`; `""` omits the segment. Cache + lock + atomic-write layers reused verbatim from v2.1. Refresh runs on a ticker inside this long-lived program (NOT per-prompt); an in-process mutex coordinates, with the v2.1 file lock retained only for external callers.

- time — `time.Now().Format("15:04:05")`.
- localip — UDP routing-decision trick (`net.Dial("udp","8.8.8.8:80")` → `LocalAddr`); cross-platform, no exec.
- extip — cache 300s; fetch `https://api.ipify.org`, validate as IP before caching.
- weather — cache 1800s; fetch `https://wttr.in/?format=%c+%l+%t` with `User-Agent: terminal-hud`.
- git — detect `.git`; branch from HEAD; detached→short SHA; rebase/merge→state marker; `git status --porcelain` with 200ms timeout. CWD comes from the engine's OSC 7 event, not this program's getwd.
- azure — cache 60s; exec `az account show --query name -o tsv` if `az` present.
- k8s — read `~/.kube/config` (honor `$KUBECONFIG` first file); line-scan `current-context` + namespace; no kubectl on hot path.
- path — from the engine's OSC 7 cwd event if available; else hidden. Shorten to last 2 components with leading `…`.
- exit — last command exit code, from a custom OSC emitted by an opt-in zsh precmd snippet, parsed by the engine. Hidden if the snippet isn't installed.

---

## Redraw model

Bars are frozen rows redrawn in place every frame, so there is no scrollback stacking. The top bar updates live (clock ticks via the render ticker) and never scrolls away. Renders are coalesced: byte/input/event sources set a dirty flag; the render tick and resize flush it, so heavy shell output does not cause a redraw per byte.

---

## Color system

Same philosophy as v2.1: emit only the 8 named ANSI foreground codes for **bar chrome** so the user's terminal theme owns the actual colors. No `%{ %}` / `%`-doubling concerns (no `$PS1` involvement). Shell content in the middle is rendered with whatever styles libvterm parsed — passed through faithfully; this program does not recolor shell output.

Role assignments (bar chrome only): time=white, local ip=green, ext ip=yellow, weather=cyan, separator=bright_black/dim, git branch=white, clean ✓=green, dirty counts=yellow, detached/state marker=yellow, azure=blue, k8s=cyan, exit=red. No bold/italic/background, no 256-color/truecolor for chrome.

---

## Build order (Phase 0 already passed)

1. `engine` — cgo over libvterm: create VTerm, feed bytes, read grid; wire `sb_pushline`/`sb_popline` to the ring buffer; OSC parsing. Unit-test against known VT input (the Phase 0b proof is the first test).
2. `ptyhost` — open a pty, spawn `$SHELL`, pump output into `engine`, forward stdin, handle SIGWINCH (resize pty AND engine AND layout).
3. `render` — given a composited frame, draw to the real terminal efficiently (diff against last frame; emit only changed cells). Most perceived quality lives here.
4. `compositor` — three-region math: reserve rows 0 and N-1; map grid + scroll offset into rows 1..N-2; place bar strings in frozen rows.
5. `hud/*` + reuse `cache`,`lock`,`color` from v2.1 — produce the two bar strings on a ticker.
6. `input` — passthrough to pty, plus intercepted hotkeys for scrollback navigation and copy-mode selection.
7. `main` — wire into one event loop (pty read, render tick, input, resize, hud refresh, engine events). Terminal restore on exit AND panic is the top correctness invariant.

Build leaf-first so each layer is testable before the binary assembles.

---

## Error handling (deliberate, per failure)

- **Terminal restore is sacred.** Raw mode + alt-screen restored via `defer` and a panic-recover that restores the tty before re-panicking. A crash must never leave the user's shell in raw mode.
- **Shell exit / pty EOF** → clean shutdown: restore tty, show cursor, exit with the shell's status.
- **cgo callbacks** cannot return Go errors across C; they log structured and degrade (scrollback push failure drops the oldest line, never panics).
- **HUD modules fail soft** → network/exec errors return `""` or last cached value, with timeouts; never stall or crash the loop.
- **Render write errors** → fail loudly.

---

## Testing strategy

Testing pyramid; most of the system is purely, deterministically testable:
- `engine` (many fast unit tests): feed known VT bytes, assert grid/cursor/scrollback; SGR color, cursor moves, wrap→`sb_pushline`, alt-screen, OSC 7, custom exit OSC, resize.
- `compositor` (pure): grid + bars + scroll offset → expected frame.
- `render` (pure): prev + next frame → expected minimal emit ops.
- `hud/*` (unit, injected inputs): mock kubeconfig, injected clock, fake OSC events, stubbed network behind cache.
- `ptyhost` + `input` (fewer integration tests): scripted program in a real pty; assert flow + SIGWINCH.
- end-to-end (a couple smoke tests): launch binary against a scripted shell; assert no crash AND tty restored.

CI: GitHub Actions on macOS + Linux; libvterm via `brew`/`apt`; cgo build runs in CI. Dependencies pinned; vulnerability scan in CI.

---

## Open design questions (decide during build, not blockers)

1. **Copy-mode UX.** Hotkey to enter copy-mode, motion keys, yank to clipboard via `pbcopy` (macOS) or OSC 52 (fallback). Define after render+input work.
2. **Scroll keybindings.** Which keys enter/navigate scroll mode (e.g. shift-pageup, a prefix key). Settle during the input phase.
3. **Launch model.** Manual (`terminal-hud` takes over the window) first for testing; terminal-profile command once stable.
4. **OSC callback wiring.** Confirm libvterm's exact OSC parser-callback signature against `vterm.h` at implementation time (grid/scrollback API already verified; OSC not yet).
5. **Opt-in zsh snippets.** Ship and document the one-line precmd for exit-code OSC and (optionally) OSC 7 for cwd.

---

## Known limitations (accepted)

- **No scrollback reflow on resize** (libvterm limitation): the active screen reflows, history does not.
- libvterm exposes the common VT feature set (it backs Neovim's `:terminal`); any feature it does not surface is not available. Verify needed features during the engine phase.
- KUBECONFIG multi-file merge not implemented in v3.1 (first existing file only).
- Exit-code and CWD segments require optional shell integration (Open questions 5).

---

## Why this is durable

libvterm is a small, stable C99 library (API frozen since 0.3) available as a system package on every target platform, with no version-pinned compiler toolchain and no network-fetched build dependencies. The cgo surface is small (feed bytes, read cells, two scrollback callbacks). This is a deliberately boring, survivable foundation — the opposite of the unstable, exotic-toolchain dependency that v3.0 could not build. True pinning still requires owning the screen; that conclusion from v2.1/v3.0 stands, and v3.1 honors it with a dependency that actually builds.
