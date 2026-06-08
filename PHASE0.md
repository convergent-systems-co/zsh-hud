# Phase 0 — go/no-go results

**Date:** 2026-06-08
**Host:** macOS 26.5.1 (Tahoe), Darwin 25.5.0, arm64 (Apple Silicon)
**Goal:** Prove `go-libghostty` builds, links, and runs before any HUD code (per SPEC.md §Phase 0).

**Status — libghostty-vt path: NO-GO on this machine.** Root cause is a toolchain incompatibility — Zig 0.15.2 (pinned by ghostty) cannot link Mach-O on macOS 26. Not a go-libghostty API problem; not fixable here without upstream changes. This is exactly the "find out cheaply" outcome Phase 0 was designed to surface. See **Verdict**.

**Status — libvterm path (chosen, v3.1): GO, verified.** A Go cgo program built and ran against Homebrew `libvterm` 0.3.3 on this macOS 26 machine, parsed an SGR-colored byte stream, and read the grid back: `"Hello, world!"`. See **Phase 0b** at the bottom. This is the path forward.

---

## Pinned versions / SHAs (record these)

| Component | Value |
|-----------|-------|
| go-libghostty module path | `go.mitchellh.com/libghostty` (NOT `github.com/mitchellh/go-libghostty` as SPEC.md assumed) |
| go-libghostty HEAD SHA | `e9e1010f80b1ced0b7efcdb300f4838513c0816e` ("update ghostty") |
| ghostty source pin (CMakeLists.txt FetchContent) | `6d089a544db53f3457374c2c406bccee80722cdf` |
| Go | go1.26.4 darwin/arm64 (module requires go 1.26.0) ✓ |
| CMake | 4.3.3 ✓ |
| pkg-config | 2.5.1 ✓ |
| Zig REQUIRED by ghostty `6d089a5` | **0.15.2 exactly** (`requireZig`, hard `@compileError` otherwise) |
| Zig from Homebrew | 0.16.0 — rejected by ghostty's version check |
| Zig 0.15.2 (official, downloaded) | builds ghostty's `build.zig`, but **cannot link** on macOS 26 (see below) |

### Working paths (per upstream Makefile)
- Built lib pkgconfig (would set `PKG_CONFIG_PATH` here): `build/_deps/ghostty-src/zig-out/share/pkgconfig`
- Built lib dir (`DYLD_LIBRARY_PATH`): `build/_deps/ghostty-src/zig-out/lib`
- Zig global package cache: `/Users/itsfwcp/.cache/zig` (`p/` subdir holds fetched deps)

---

## What the live repo says (source of truth over SPEC.md)

- Build: `make build` (CMake → FetchContent fetches+builds ghostty via `zig build` → `go build ./...`)
- Test: `make test`
- Module import: `import "go.mitchellh.com/libghostty"`
- cgo links the static lib by default; `-tags dynamic` for shared.
- README "Hello, bold-green world" formatter example present; matches SPEC.md Phase 0 step 4.
- `flake.nix` pins Zig 0.15.2; `CMakeLists.txt` pins ghostty `6d089a5`.

---

## What was peeled back, layer by layer (all evidence, not guesses)

Three distinct blockers, each resolved/characterized in turn. Getting through #1 and #2 is what let #3 — the real, hard one — surface.

### Layer 1 — Cloudflare blocks ghostty's dependency mirror (RESOLVED)
`cmake --build` → `zig build` → fetch of `deps.files.ghostty.org/uucode-…tar.gz` returned `403 Forbidden`.
- The tarball is genuinely reachable: a manual `curl` got `200 OK` (2.2 MB) at the *same moment* Zig's fetcher got `403` → Cloudflare bot-protection blocks Zig's fetcher by fingerprint, independent of IP.
- Repeated attempts then tripped IP-level rate-limiting: `403` on 16/16 curl attempts over 12 min from egress `50.203.241.18` (the managed/corporate network).
- **Resolved by switching to a phone hotspot** (egress changed to a cellular IP); the mirror returned `200` and all deps fetched cleanly.

### Layer 2 — Zig version mismatch (RESOLVED)
With deps fetching, `zig build` evaluated ghostty's `build.zig` and hard-failed:
```
src/build/zig.zig:13: error: Your Zig version v0.16.0 does not meet the required build version of v0.15.2
build.zig:27: error: member function expected 4 argument(s), found 3   // readFileAlloc signature changed in 0.16
```
Ghostty `6d089a5` requires Zig **exactly 0.15.2**. **Resolved by downloading official Zig 0.15.2** (`zig-aarch64-macos-0.15.2.tar.xz`) and putting it first on PATH. Version check then passed; deps fetched; compilation proceeded to the link step.

### Layer 3 — Zig 0.15.2 cannot link Mach-O on macOS 26 (HARD BLOCKER, not resolvable here)
Linking `libghostty-vt.dylib` failed with ~25 `undefined symbol` errors for **libSystem/libc symbols**: `__availability_version_check`, `_abort`, `_malloc_size`, `_clock_gettime`, `_dispatch_*`, `_realpath$DARWIN_EXTSN`, `_sigaction`, `_posix_memalign`, etc. No artifact was emitted (`zig-out/` empty).

Isolated to a Zig/OS bug, independent of ghostty:
- A **trivial** `zig build-exe` of `pub fn main() void {}` reproduces it: exit 1, same libSystem `undefined symbol` flood. So it is **not** ghostty- or go-libghostty-specific.
- The macOS SDK is present and valid (Xcode 26.5, SDK 26.5, `libSystem.tbd` present, `xcrun` resolves it). Setting `SDKROOT` explicitly does not help.
- Zig 0.15.2's default Mach-O linker is its **self-hosted** linker, which is what fails. Forcing LLD (`-flld`) returns `error: using LLD to link macho files is unsupported` — so there is **no alternative linker** available in 0.15.2.

**Root cause:** Zig 0.15.2's self-hosted Mach-O linker is incompatible with the macOS 26 (Tahoe) SDK, and 0.15.2 offers no fallback linker. Because ghostty `6d089a5` pins Zig *exactly* 0.15.2, there is no version of the toolchain on this machine that both satisfies ghostty's version check *and* links successfully.

---

## Gotchas discovered (carry these forward)

1. **Module path drift:** SPEC.md says `go get github.com/mitchellh/go-libghostty`; the real module is `go.mitchellh.com/libghostty`. Use the latter in `go.mod`.
2. **Network:** `deps.files.ghostty.org` is Cloudflare-protected and blocks Zig's fetcher / corporate egress. The first build must run on a network that isn't blocked (hotspot worked). After one successful build, deps are cached and the built static lib is reusable offline.
3. **Zig version is exact, not minimum:** ghostty `requireZig(0.15.2)`. Homebrew's `zig` (0.16.0) is rejected. You need official 0.15.2.
4. **macOS 26 + Zig 0.15.2 = no Mach-O link.** The blocker. Will clear when go-libghostty bumps its ghostty pin to a release requiring Zig ≥ 0.16 (which supports macOS 26), or when building on macOS ≤ 15.

---

## Verdict

**NO-GO on this machine, today.** The go-libghostty *API* was never the problem — the wall is the build toolchain: ghostty pins Zig 0.15.2, and Zig 0.15.2 cannot produce a Mach-O binary on macOS 26 (its only Mach-O linker is broken against the 26.5 SDK, with no fallback). This is the cheap, early "the build is too raw right now" signal SPEC.md §Phase 0 and §Honest-constraints #5 explicitly plan for. Per the spec: do not paper over it.

Resolution paths, most to least promising:

1. **Revisit when upstream moves off Zig 0.15.2.** go-libghostty HEAD is actively developed; once its pinned ghostty requires Zig ≥ 0.16 (macOS-26-capable), re-run Phase 0. Lowest effort, just waiting. Re-test command below.
2. **Build on macOS ≤ 15 (Sequoia) or a Linux box** (Linux is SPEC.md's secondary target and uses a different link path). Build `libghostty-vt` there once, then reuse the prebuilt static lib + pkgconfig for `zsh-hud`'s `go build` (which links it via cgo and needs neither Zig nor the network afterward).
3. **Fall back to v2.1** (scrolling top bar, no libghostty dependency) per SPEC.md §Honest-constraints #5 — a valid different point on the tradeoff curve, not a failure.

Re-test command (run on a non-blocked network, with official Zig 0.15.2 on PATH — or whatever Zig the then-current ghostty pin requires):
```
git clone https://github.com/mitchellh/go-libghostty && cd go-libghostty
make build && make test
```
Success criterion (SPEC.md Phase 0 step 5): the README formatter example prints styled `Hello, world!` with no link or runtime errors.

---

## Phase 0b — libvterm (the chosen path): GO, verified

After the libghostty-vt NO-GO, we re-ran the go/no-go gate against **libvterm** (the C99 VT engine behind Neovim's `:terminal`), which fills the same "parse shell bytes → readable grid + scrollback hand-off" role.

- Installed `libvterm` 0.3.3 via Homebrew (bottle `arm64_tahoe` — built *for* macOS 26). `pkg-config --modversion vterm` → `0.3.3`.
- Verified the C API against the installed header (`vterm_new`, `vterm_set_utf8`, `vterm_obtain_screen`, `vterm_screen_reset`, `vterm_input_write`, `vterm_screen_get_cell`, and `sb_pushline`/`sb_popline` scrollback callbacks all present).
- Wrote a trivial Go **cgo** program (`#cgo pkg-config: vterm`): created a VTerm, fed `"Hello, \033[1;32mworld\033[0m!"`, read row 0 back through cgo.
- Result on this macOS 26 machine:
  ```
  libvterm version: 0.3
  grid row 0 read back via cgo: "Hello, world!"
  ```

**Conclusion:** the engine contract v3.1 needs (feed bytes, read grid, receive scrolled-off lines) works through cgo on macOS 26 with `brew install libvterm` + `go build` — no Zig, no Cloudflare-hosted deps, no exact-version toolchain pin, stable API. **GO.**

Tradeoff accepted vs libghostty-vt: libvterm gives no built-in scrollback storage (we keep the ring buffer via `sb_pushline`/`sb_popline`), no built-in selection (we build it), and **no reflow of scrollback history on resize** (active screen does reflow; history does not — [libvterm bug #1952530](https://bugs.launchpad.net/libvterm/+bug/1952530)). These are documented limitations, not blockers.

---

## Environment changes made during Phase 0 (for cleanup)

- `brew install zig` → **Zig 0.16.0 installed system-wide** (plus deps llvm@21, lld@21). Still present. Remove with `brew uninstall zig` if unwanted (llvm@21/lld@21 are large; `brew autoremove` clears unused deps).
- Official Zig 0.15.2 + go-libghostty checkout + build artifacts live under `/tmp/phase0/` (ephemeral; cleared on reboot).
- Zig global cache populated at `~/.cache/zig` (harmless; speeds a future retry).
- No changes to the `zsh-hud` repo except this `PHASE0.md`.
