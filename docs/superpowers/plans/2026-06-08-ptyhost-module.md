# ptyhost Module Implementation Plan (Plan 2 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `ptyhost` module — open a pseudo-terminal, spawn a child process (the user's `$SHELL` in production), and expose its I/O as `io.Reader`/`io.Writer` plus resize, wait, and close. This is the OS-facing leaf that feeds bytes to the engine and forwards keystrokes to the shell.

**Architecture:** A thin wrapper over `github.com/creack/pty`. `ptyhost` deliberately does NOT import `engine` — it exposes the shell's output as a stream that `main` wires to `engine.Write`, and accepts input via `Write`. This keeps the module decoupled and testable in isolation (spawn `cat`/`sh -c` and round-trip bytes). SIGWINCH handling lives in `main`, which calls `ptyhost.Resize`.

**Tech Stack:** Go 1.26, `github.com/creack/pty` v1.1.24 (MIT — the de-facto Go pty library; verified API: `Start`, `StartWithSize`, `Setsize`, `GetsizeFull`, `Winsize{Rows,Cols,X,Y uint16}`).

**Prerequisite:** builds on the engine branch (the `terminal-hud` go module + scaffold from Plan 1 must be present). No libvterm interaction in this module.

**Scope note:** This plan is the pty lifecycle only. Wiring ptyhost↔engine and the SIGWINCH signal handler belong to the `main`/event-loop plan. Shell-integration OSC is out of scope (later).

---

## File Structure

```
go.mod / go.sum          ← add github.com/creack/pty
internal/ptyhost/
  doc.go                 ← package doc
  shell.go               ← ResolveShell (pure: explicit → $SHELL → /bin/sh)
  shell_test.go
  ptyhost.go             ← PtyHost: Start/Read/Write/Resize/Size/SetReadDeadline/Wait/Close
  ptyhost_test.go
```

Responsibilities: `shell.go` is pure shell-path resolution (no I/O). `ptyhost.go` owns the pty master `*os.File` + the `*exec.Cmd` and the lifecycle.

---

### Task 1: Add creack/pty dependency + ResolveShell

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/ptyhost/doc.go`
- Create: `internal/ptyhost/shell.go`
- Test: `internal/ptyhost/shell_test.go`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/creack/pty@v1.1.24`
Expected: `go.mod` gains `require github.com/creack/pty v1.1.24`; `go.sum` updated.

- [ ] **Step 2: Create the package doc**

`internal/ptyhost/doc.go`:
```go
// Package ptyhost opens a pseudo-terminal, spawns a child process (the user's
// shell in production), and exposes the child's I/O as an io.Reader/io.Writer
// plus resize, wait, and close. It does not depend on the engine: main wires
// the reader to engine.Write and forwards input through Write.
package ptyhost
```

- [ ] **Step 3: Write the failing test**

`internal/ptyhost/shell_test.go`:
```go
package ptyhost

import "testing"

func TestResolveShellPrefersExplicit(t *testing.T) {
	if got := ResolveShell("/bin/zsh"); got != "/bin/zsh" {
		t.Fatalf("ResolveShell(/bin/zsh) = %q, want /bin/zsh", got)
	}
}

func TestResolveShellFallsBackToEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := ResolveShell(""); got != "/usr/bin/fish" {
		t.Fatalf("ResolveShell(\"\") with $SHELL set = %q, want /usr/bin/fish", got)
	}
}

func TestResolveShellDefaultsToBinSh(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := ResolveShell(""); got != "/bin/sh" {
		t.Fatalf("ResolveShell(\"\") with no $SHELL = %q, want /bin/sh", got)
	}
}
```

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/ptyhost/ -run TestResolveShell -v`
Expected: FAIL — `undefined: ResolveShell`.

- [ ] **Step 5: Implement ResolveShell**

`internal/ptyhost/shell.go`:
```go
package ptyhost

import "os"

// defaultShell is the POSIX fallback when neither an explicit shell nor $SHELL
// is available.
const defaultShell = "/bin/sh"

// ResolveShell picks the shell to spawn: an explicit path wins; otherwise $SHELL;
// otherwise /bin/sh. It does not verify the path exists — that surfaces as a
// Start error, which is the correct boundary for that failure.
func ResolveShell(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return defaultShell
}
```

- [ ] **Step 6: Run to verify pass**

Run: `go test ./internal/ptyhost/ -run TestResolveShell -v`
Expected: PASS (all 3).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/ptyhost/doc.go internal/ptyhost/shell.go internal/ptyhost/shell_test.go
git commit -m "feat(ptyhost): add creack/pty dep and ResolveShell"
```

---

### Task 2: PtyHost Start / Read / Write / Close + round-trip tests

**Files:**
- Create: `internal/ptyhost/ptyhost.go`
- Test: `internal/ptyhost/ptyhost_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/ptyhost/ptyhost_test.go`:
```go
package ptyhost

import (
	"bytes"
	"testing"
	"time"
)

// readUntil reads from p until needle appears or the deadline passes.
func readUntil(t *testing.T, p *PtyHost, needle []byte, within time.Duration) []byte {
	t.Helper()
	if err := p.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	var acc []byte
	buf := make([]byte, 1024)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			if bytes.Contains(acc, needle) {
				return acc
			}
		}
		if err != nil {
			return acc // EOF, EIO (child exited), or deadline — return what we have
		}
	}
}

func TestPtyHostReadsChildOutput(t *testing.T) {
	p, err := Start("/bin/sh", []string{"-c", "printf 'PTYHOST_READY\\n'"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	out := readUntil(t, p, []byte("PTYHOST_READY"), 3*time.Second)
	if !bytes.Contains(out, []byte("PTYHOST_READY")) {
		t.Fatalf("did not see marker in child output; got %q", out)
	}
}

func TestPtyHostForwardsInput(t *testing.T) {
	// cat echoes stdin back to stdout (the pty also echoes by default); either
	// way the bytes we Write must appear when we Read.
	p, err := Start("/bin/cat", nil, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	if _, err := p.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := readUntil(t, p, []byte("ping"), 3*time.Second)
	if !bytes.Contains(out, []byte("ping")) {
		t.Fatalf("did not read back forwarded input; got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ptyhost/ -run TestPtyHost -v`
Expected: FAIL — `undefined: Start` / `undefined: PtyHost`.

- [ ] **Step 3: Implement PtyHost core**

`internal/ptyhost/ptyhost.go`:
```go
package ptyhost

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
)

// PtyHost owns a pseudo-terminal master and the child process attached to its
// slave. Read returns the child's output; Write sends input to the child.
// Not safe for concurrent Read+Read or Write+Write, but a single reader and a
// single writer on separate goroutines is fine (independent fd directions).
type PtyHost struct {
	master *os.File
	cmd    *exec.Cmd
}

// Start spawns name with args attached to a new pty sized rows x cols and
// returns a PtyHost. name must be non-empty (use ResolveShell for the default).
func Start(name string, args []string, rows, cols int) (*PtyHost, error) {
	if name == "" {
		return nil, fmt.Errorf("ptyhost: empty command name")
	}
	if rows < 1 || cols < 1 {
		return nil, fmt.Errorf("ptyhost: invalid size %dx%d", rows, cols)
	}
	cmd := exec.Command(name, args...)
	ws := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	master, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, fmt.Errorf("ptyhost: start %q: %w", name, err)
	}
	return &PtyHost{master: master, cmd: cmd}, nil
}

// Read returns bytes from the child's output. Implements io.Reader.
func (p *PtyHost) Read(b []byte) (int, error) { return p.master.Read(b) }

// Write sends input bytes to the child. Implements io.Writer.
func (p *PtyHost) Write(b []byte) (int, error) { return p.master.Write(b) }

// SetReadDeadline sets a deadline on Read; a zero time clears it. Useful for
// non-blocking reads in the event loop and for bounded test reads.
func (p *PtyHost) SetReadDeadline(t time.Time) error { return p.master.SetReadDeadline(t) }

// Close closes the pty master. This sends EOF to the child's stdin; if the
// child is still running it will typically exit. Callers should still Wait to
// reap the process.
func (p *PtyHost) Close() error { return p.master.Close() }
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/ptyhost/ -run TestPtyHost -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ptyhost/ptyhost.go internal/ptyhost/ptyhost_test.go
git commit -m "feat(ptyhost): pty lifecycle (Start/Read/Write/SetReadDeadline/Close)"
```

---

### Task 3: Resize + Size

**Files:**
- Modify: `internal/ptyhost/ptyhost.go`
- Test: `internal/ptyhost/ptyhost_test.go` (extend)

- [ ] **Step 1: Write the failing test (append)**

```go
func TestPtyHostResize(t *testing.T) {
	// `sleep 5` keeps the slave open so the size query is meaningful.
	p, err := Start("/bin/sleep", []string{"5"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	if err := p.Resize(30, 100); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	rows, cols, err := p.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if rows != 30 || cols != 100 {
		t.Fatalf("Size after resize = (%d,%d), want (30,100)", rows, cols)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ptyhost/ -run TestPtyHostResize -v`
Expected: FAIL — `undefined: (*PtyHost).Resize`.

- [ ] **Step 3: Implement Resize + Size (append to ptyhost.go)**

```go
// Resize sets the pty window size to rows x cols. The kernel delivers SIGWINCH
// to the child.
func (p *PtyHost) Resize(rows, cols int) error {
	if rows < 1 || cols < 1 {
		return fmt.Errorf("ptyhost: invalid size %dx%d", rows, cols)
	}
	ws := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	if err := pty.Setsize(p.master, ws); err != nil {
		return fmt.Errorf("ptyhost: resize: %w", err)
	}
	return nil
}

// Size reports the current pty window size.
func (p *PtyHost) Size() (rows, cols int, err error) {
	ws, err := pty.GetsizeFull(p.master)
	if err != nil {
		return 0, 0, fmt.Errorf("ptyhost: getsize: %w", err)
	}
	return int(ws.Rows), int(ws.Cols), nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/ptyhost/ -run TestPtyHostResize -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ptyhost/ptyhost.go internal/ptyhost/ptyhost_test.go
git commit -m "feat(ptyhost): Resize and Size via pty.Setsize/GetsizeFull"
```

---

### Task 4: Wait + exit status

**Files:**
- Modify: `internal/ptyhost/ptyhost.go`
- Test: `internal/ptyhost/ptyhost_test.go` (extend)

- [ ] **Step 1: Write the failing tests (append)**

```go
import-additions: add "errors" and "os/exec" to the test file's import block.

func TestPtyHostWaitReturnsExitCode(t *testing.T) {
	p, err := Start("/bin/sh", []string{"-c", "exit 7"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	err = p.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("Wait err = %v, want *exec.ExitError", err)
	}
	if ee.ExitCode() != 7 {
		t.Fatalf("exit code = %d, want 7", ee.ExitCode())
	}
}

func TestPtyHostWaitSucceedsOnCleanExit(t *testing.T) {
	p, err := Start("/bin/sh", []string{"-c", "exit 0"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()

	if err := p.Wait(); err != nil {
		t.Fatalf("Wait on clean exit = %v, want nil", err)
	}
}
```
(The test file already imports `bytes`, `testing`, `time`; add `errors` and `os/exec`.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ptyhost/ -run TestPtyHostWait -v`
Expected: FAIL — `undefined: (*PtyHost).Wait`.

- [ ] **Step 3: Implement Wait (append to ptyhost.go)**

```go
// Wait blocks until the child process exits and returns its exit status. A
// non-zero exit is reported as *exec.ExitError (use errors.As). Call Wait
// exactly once.
func (p *PtyHost) Wait() error { return p.cmd.Wait() }
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/ptyhost/ -run TestPtyHostWait -v`
Expected: PASS (both).

- [ ] **Step 5: Run the full module suite + vet**

Run: `go test ./internal/ptyhost/ -v` then `go vet ./internal/ptyhost/`
Expected: all ptyhost tests PASS; vet clean.

- [ ] **Step 6: Commit**

```bash
git add internal/ptyhost/ptyhost.go internal/ptyhost/ptyhost_test.go
git commit -m "feat(ptyhost): Wait returns child exit status"
```

---

## Self-Review

**Spec coverage (ptyhost portion of SPEC.md):**
- "open a pty, spawn $SHELL" → `Start` + `ResolveShell` (Tasks 1, 2) ✓
- "pump shell output" → exposed as `Read` (io.Reader); main wires it to engine (deliberate decoupling, documented above) ✓
- "forward stdin to the pty" → `Write` (io.Writer) (Task 2) ✓
- "handle SIGWINCH (resize pty AND engine AND layout)" → `Resize`/`Size` provide the pty half (Task 3); the SIGWINCH signal handler + engine/layout calls belong to the main/event-loop plan (noted in scope) ✓
- Clean lifecycle: `Close` + `Wait` with exit status (Tasks 2, 4) ✓

**Placeholder scan:** none. Note: Task 4 Step 1 has a human-readable "import-additions" line describing which imports to add to the test file — that is an instruction, not code to paste verbatim.

**Type consistency:** `Start(name string, args []string, rows, cols int)`, `ResolveShell(explicit string) string`, methods `Read/Write/SetReadDeadline/Resize/Size/Wait/Close` — consistent across tasks. `Resize(rows, cols int)` matches the engine's `Resize(rows, cols int)` signature for easy wiring in main.

**Platform note:** `creack/pty` supports macOS + Linux (CI covers both). Reading the master after the child exits returns EOF or EIO depending on OS; `readUntil` handles this by returning accumulated bytes on any error. Tests use `/bin/sh`, `/bin/cat`, `/bin/sleep` which exist on both platforms.

**CI:** no change needed — the existing workflow runs `go test -race ./...`, which now includes `internal/ptyhost`.

**Known follow-ups (later plans):** wiring ptyhost.Read → engine.Write in the event loop; installing the SIGWINCH handler in main; copy-mode/input hotkeys; OSC-based shell integration.
