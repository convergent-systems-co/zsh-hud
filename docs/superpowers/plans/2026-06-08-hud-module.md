# hud Module Implementation Plan (Plan 5 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `hud` module — the segment functions that produce the data shown in the top and bottom bars, plus a concurrency-safe TTL cache and the bar-assembly that turns segments into styled `[]engine.Cell` rows the compositor places.

**Architecture:** Pure-ish logic with **injected dependencies** (clock, HTTP fetcher, command runner, file path) so every segment is unit-testable with no real network or CLI. An in-memory TTL `cache` (not the v2.1 file cache — v3.1 is a long-lived process, so files/locks are unnecessary; the SPEC notes "in-process a mutex suffices"). Bar assembly composes segments → cells with named-ANSI role colors, separators, and right-alignment for the weather segment.

**Tech Stack:** Go 1.26, stdlib only (`net`, `net/http`, `os`, `os/exec`, `time`, `strings`, `sync`). Imports `terminal-hud/internal/engine` for `Cell`/`Color`.

**Prerequisite:** `engine` on the branch (it's on `main`). hud does not depend on render/compositor/ptyhost.

**Scope (decided):**
- **Defer `path` and `exit` segments** — they need OSC parsing (OSC 7 cwd, custom `$?` OSC) that was deferred from the engine. A later plan adds OSC to the engine then these two segments.
- 7 segments this plan: `Time`, `LocalIP`, `ExtIP`, `Weather`, `Git`, `Azure`, `K8s`.
- Bar chrome uses named-ANSI indexed colors (host theme owns the RGB) per the SPEC color policy; shell content (not hud's concern) passes through elsewhere.
- Single-rune-width segment text (consistent with render/compositor v1).

---

## File Structure

```
internal/cache/
  cache.go            ← in-memory TTL cache, GetOrRefresh (single-flight)
  cache_test.go
internal/hud/
  color.go            ← role → engine.Color (named ANSI) + Segment type
  color_test.go
  bar.go              ← AssembleBar(cols, left, right) []engine.Cell
  bar_test.go
  module/
    time.go localip.go extip.go weather.go git.go azure.go k8s.go
    *_test.go
  bars.go             ← TopBar/BottomBar: wire modules → segments → AssembleBar
  bars_test.go
```

---

### Task 1: in-memory TTL cache

**Files:** Create `internal/cache/cache.go`; test `internal/cache/cache_test.go`.

- [ ] **Step 1: Failing tests** — `internal/cache/cache_test.go`:
```go
package cache

import (
	"errors"
	"testing"
	"time"
)

func TestGetOrRefreshFetchesThenCaches(t *testing.T) {
	c := New()
	calls := 0
	fetch := func() (string, error) { calls++; return "v1", nil }

	if got := c.GetOrRefresh("k", time.Minute, fetch); got != "v1" {
		t.Fatalf("first = %q, want v1", got)
	}
	if got := c.GetOrRefresh("k", time.Minute, fetch); got != "v1" {
		t.Fatalf("second = %q, want v1 (cached)", got)
	}
	if calls != 1 {
		t.Fatalf("fetch called %d times, want 1 (second served from cache)", calls)
	}
}

func TestGetOrRefreshRefetchesAfterTTL(t *testing.T) {
	c := New()
	vals := []string{"a", "b"}
	i := 0
	fetch := func() (string, error) { v := vals[i]; i++; return v, nil }

	if got := c.GetOrRefresh("k", time.Nanosecond, fetch); got != "a" {
		t.Fatalf("first = %q, want a", got)
	}
	time.Sleep(time.Millisecond) // exceed the 1ns TTL
	if got := c.GetOrRefresh("k", time.Nanosecond, fetch); got != "b" {
		t.Fatalf("after TTL = %q, want b (refetched)", got)
	}
}

func TestGetOrRefreshReturnsStaleOnError(t *testing.T) {
	c := New()
	// Seed a value, then force an error on the next (post-TTL) refresh.
	c.GetOrRefresh("k", time.Nanosecond, func() (string, error) { return "good", nil })
	time.Sleep(time.Millisecond)
	got := c.GetOrRefresh("k", time.Nanosecond, func() (string, error) { return "", errors.New("net down") })
	if got != "good" {
		t.Fatalf("on error = %q, want stale 'good'", got)
	}
}

func TestGetOrRefreshEmptyOnErrorWithNoPriorValue(t *testing.T) {
	c := New()
	got := c.GetOrRefresh("k", time.Minute, func() (string, error) { return "", errors.New("boom") })
	if got != "" {
		t.Fatalf("error with no prior value = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`undefined: New`). `go test ./internal/cache/ -v`

- [ ] **Step 3: Implement** — `internal/cache/cache.go`:
```go
// Package cache is a small in-memory TTL cache for HUD segment values. Unlike
// the v2.1 file cache, v3.1 is a long-lived process, so values live in memory
// guarded by a mutex; no files or file locks are needed.
package cache

import (
	"sync"
	"time"
)

type item struct {
	val     string
	expires time.Time
	ok      bool // a value has been stored at least once
}

// Cache is a concurrency-safe key→string store with per-call TTL.
type Cache struct {
	mu sync.Mutex
	m  map[string]item
}

// New returns an empty Cache.
func New() *Cache { return &Cache{m: make(map[string]item)} }

// GetOrRefresh returns the cached value for key if it is still within ttl.
// Otherwise it calls refresh, stores the result, and returns it. On a refresh
// error it returns the previous (stale) value if one exists, else "". The whole
// operation holds the lock, giving single-flight semantics; callers run it from
// the background refresh goroutine, not the render path, so serialization is
// acceptable. refresh implementations MUST bound their own time (HTTP/exec
// timeouts) so the lock is never held indefinitely.
func (c *Cache) GetOrRefresh(key string, ttl time.Duration, refresh func() (string, error)) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if it, found := c.m[key]; found && it.ok && now.Before(it.expires) {
		return it.val
	}

	val, err := refresh()
	if err != nil {
		if it, found := c.m[key]; found && it.ok {
			return it.val // serve stale on error
		}
		return ""
	}
	c.m[key] = item{val: val, expires: now.Add(ttl), ok: true}
	return val
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt, `go vet ./internal/cache/`.
- [ ] **Step 5: Commit**
```bash
git add internal/cache/cache.go internal/cache/cache_test.go
git commit -m "feat(cache): in-memory TTL cache with single-flight GetOrRefresh"
```

---

### Task 2: role colors + Segment + bar assembly

**Files:** Create `internal/hud/color.go`, `internal/hud/bar.go`; tests `internal/hud/color_test.go`, `internal/hud/bar_test.go`.

- [ ] **Step 1: Failing tests** — `internal/hud/color_test.go`:
```go
package hud

import (
	"testing"

	"terminal-hud/internal/engine"
)

func TestNamedColorsAreIndexed(t *testing.T) {
	cases := map[engine.Color]uint8{
		ColorWhite: 7, ColorGreen: 2, ColorYellow: 3, ColorCyan: 6,
		ColorBlue: 4, ColorRed: 1, ColorDim: 8,
	}
	for c, idx := range cases {
		if !c.IsIndexed || c.Index != idx {
			t.Fatalf("color %+v: want indexed %d", c, idx)
		}
	}
}
```

`internal/hud/bar_test.go`:
```go
package hud

import (
	"testing"

	"terminal-hud/internal/engine"
)

func barText(cells []engine.Cell) string {
	r := make([]rune, len(cells))
	for i, c := range cells {
		if c.Rune == 0 {
			r[i] = ' '
		} else {
			r[i] = c.Rune
		}
	}
	return string(r)
}

func TestAssembleBarJoinsLeftWithSeparator(t *testing.T) {
	left := []Segment{{Text: "AA", Color: ColorWhite}, {Text: "BB", Color: ColorGreen}}
	cells := AssembleBar(20, left, nil)
	if len(cells) != 20 {
		t.Fatalf("len = %d, want 20", len(cells))
	}
	// "AA" + " | " separator + "BB", left-aligned, rest blank
	if got := barText(cells); got != "AA | BB             " {
		t.Fatalf("bar = %q", got)
	}
	// color of the 'A' cells is white(7); 'B' cells green(2)
	if cells[0].FG != ColorWhite || cells[5].FG != ColorGreen {
		t.Fatalf("segment colors not applied: %+v %+v", cells[0].FG, cells[5].FG)
	}
}

func TestAssembleBarRightAlignsRightSegments(t *testing.T) {
	left := []Segment{{Text: "L", Color: ColorWhite}}
	right := []Segment{{Text: "R", Color: ColorCyan}}
	cells := AssembleBar(5, left, right)
	// "L" at col 0, "R" at col 4, middle blank
	if got := barText(cells); got != "L   R" {
		t.Fatalf("bar = %q, want 'L   R'", got)
	}
	if cells[4].FG != ColorCyan {
		t.Fatalf("right segment color not applied")
	}
}

func TestAssembleBarClipsToWidth(t *testing.T) {
	left := []Segment{{Text: "0123456789", Color: ColorWhite}}
	cells := AssembleBar(4, left, nil)
	if got := barText(cells); got != "0123" {
		t.Fatalf("bar = %q, want '0123'", got)
	}
}

func TestAssembleBarSkipsEmptySegments(t *testing.T) {
	left := []Segment{{Text: "A", Color: ColorWhite}, {Text: "", Color: ColorGreen}, {Text: "B", Color: ColorYellow}}
	// empty segment must not produce a stray separator: "A | B"
	if got := barText(AssembleBar(10, left, nil)); got != "A | B     " {
		t.Fatalf("bar = %q, want 'A | B     '", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (undefined `ColorWhite`, `Segment`, `AssembleBar`).

- [ ] **Step 3: Implement** — `internal/hud/color.go`:
```go
package hud

import "terminal-hud/internal/engine"

// Named-ANSI role colors for bar chrome (indices 0-7 + bright). The host
// terminal theme maps these to actual RGB, per the SPEC color policy.
var (
	ColorRed    = engine.Color{IsIndexed: true, Index: 1}
	ColorGreen  = engine.Color{IsIndexed: true, Index: 2}
	ColorYellow = engine.Color{IsIndexed: true, Index: 3}
	ColorBlue   = engine.Color{IsIndexed: true, Index: 4}
	ColorCyan   = engine.Color{IsIndexed: true, Index: 6}
	ColorWhite  = engine.Color{IsIndexed: true, Index: 7}
	ColorDim    = engine.Color{IsIndexed: true, Index: 8} // bright black (separators)
)

// Segment is a piece of bar text with a single color. Empty Text is omitted.
type Segment struct {
	Text  string
	Color engine.Color
}
```

`internal/hud/bar.go`:
```go
package hud

import "terminal-hud/internal/engine"

const separator = " | " // dim-colored, placed between non-empty segments

// AssembleBar lays out left segments (joined by a dim separator) starting at
// column 0 and right segments (joined likewise) ending at the last column,
// over a row of exactly width cells. Empty segments are skipped (no stray
// separators). Content is clipped to width; right segments are dropped if they
// would collide with the left run.
func AssembleBar(width int, left, right []Segment) []engine.Cell {
	row := make([]engine.Cell, max(width, 0))

	// Left run from column 0.
	col := 0
	writeJoined(row, &col, left, false)

	// Right run ending at width-1: build its cells, then place flush-right.
	var rcells []engine.Cell
	rcol := 0
	tmp := make([]engine.Cell, width) // scratch sized to width (upper bound)
	writeJoined(tmp, &rcol, right, false)
	rcells = tmp[:rcol]
	start := width - len(rcells)
	for i := 0; i < len(rcells) && start+i >= 0 && start+i < width; i++ {
		if start+i >= col { // don't overwrite the left run
			row[start+i] = rcells[i]
		}
	}
	return row
}

// writeJoined appends each non-empty segment's cells into row starting at *col,
// inserting the dim separator between consecutive non-empty segments. Stops at
// len(row) (clip).
func writeJoined(row []engine.Cell, col *int, segs []Segment, _ bool) {
	wroteOne := false
	for _, s := range segs {
		if s.Text == "" {
			continue
		}
		if wroteOne {
			putRunes(row, col, separator, ColorDim)
		}
		putRunes(row, col, s.Text, s.Color)
		wroteOne = true
	}
}

// putRunes writes s's runes into row from *col with color, advancing *col, up
// to len(row).
func putRunes(row []engine.Cell, col *int, s string, color engine.Color) {
	for _, r := range s {
		if *col >= len(row) {
			return
		}
		row[*col] = engine.Cell{Rune: r, Width: 1, FG: color}
		*col++
	}
}
```
(Go 1.26 has the builtin `max`. If unavailable, replace `max(width,0)` with an inline guard.)

- [ ] **Step 4: Run — expect PASS** (all color + bar tests). gofmt, vet.
- [ ] **Step 5: Commit**
```bash
git add internal/hud/color.go internal/hud/bar.go internal/hud/color_test.go internal/hud/bar_test.go
git commit -m "feat(hud): role colors, Segment, and bar assembly with right-align"
```

---

### Task 3: time + localip segments

**Files:** Create `internal/hud/module/time.go`, `internal/hud/module/localip.go`; tests alongside.

- [ ] **Step 1: Failing tests** — `internal/hud/module/time_test.go`:
```go
package module

import (
	"testing"
	"time"
)

func TestTimeFormatsHHMMSS(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 32, 7, 0, time.UTC)
	if got := Time(now); got != "14:32:07" {
		t.Fatalf("Time = %q, want 14:32:07", got)
	}
}
```
`internal/hud/module/localip_test.go`:
```go
package module

import (
	"errors"
	"net"
	"testing"
)

type fakeAddr struct{ ip string }

func (f fakeAddr) Network() string { return "udp" }
func (f fakeAddr) String() string  { return f.ip + ":54321" }

type fakeConn struct{ local net.Addr }

func (c fakeConn) Read([]byte) (int, error)         { return 0, nil }
func (c fakeConn) Write([]byte) (int, error)        { return 0, nil }
func (c fakeConn) Close() error                     { return nil }
func (c fakeConn) LocalAddr() net.Addr              { return c.local }
func (c fakeConn) RemoteAddr() net.Addr             { return nil }
func (c fakeConn) SetDeadline(t time.Time) error    { return nil }
func (c fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestLocalIPExtractsIP(t *testing.T) {
	dial := func(_, _ string) (net.Conn, error) {
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP("192.168.1.42"), Port: 54321}}, nil
	}
	if got := LocalIP(dial); got != "192.168.1.42" {
		t.Fatalf("LocalIP = %q, want 192.168.1.42", got)
	}
}

func TestLocalIPEmptyOnDialError(t *testing.T) {
	dial := func(_, _ string) (net.Conn, error) { return nil, errors.New("no route") }
	if got := LocalIP(dial); got != "" {
		t.Fatalf("LocalIP on error = %q, want empty", got)
	}
}

// import time for the SetDeadline signatures above
var _ = time.Now
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/hud/module/ -v`

- [ ] **Step 3: Implement** — `internal/hud/module/time.go`:
```go
// Package module holds the individual HUD segment functions. Each returns the
// segment's text, or "" to omit it, and takes its external dependencies as
// arguments so it is unit-testable without real I/O.
package module

import "time"

// Time returns the clock as HH:MM:SS.
func Time(now time.Time) string { return now.Format("15:04:05") }
```
`internal/hud/module/localip.go`:
```go
package module

import "net"

// DialFunc matches net.Dial. Injected so tests can stub the routing decision.
type DialFunc func(network, address string) (net.Conn, error)

// LocalIP returns this host's primary local IP using the UDP routing-decision
// trick: dialing a public address selects the outbound interface without
// sending packets. Returns "" on any error.
func LocalIP(dial DialFunc) string {
	conn, err := dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	ua, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || ua.IP == nil {
		return ""
	}
	return ua.IP.String()
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt, vet.
- [ ] **Step 5: Commit**
```bash
git add internal/hud/module/time.go internal/hud/module/localip.go internal/hud/module/time_test.go internal/hud/module/localip_test.go
git commit -m "feat(hud/module): time and localip segments"
```

---

### Task 4: extip + weather segments (network via injected fetcher + cache)

**Files:** Create `internal/hud/module/extip.go`, `internal/hud/module/weather.go`; tests alongside.

- [ ] **Step 1: Failing tests** — `internal/hud/module/extip_test.go`:
```go
package module

import (
	"errors"
	"testing"

	"terminal-hud/internal/cache"
)

func TestExtIPValidatesAndCaches(t *testing.T) {
	c := cache.New()
	fetch := func() (string, error) { return "72.14.201.9\n", nil }
	if got := ExtIP(c, fetch); got != "72.14.201.9" {
		t.Fatalf("ExtIP = %q, want trimmed IP", got)
	}
}

func TestExtIPRejectsNonIP(t *testing.T) {
	c := cache.New()
	fetch := func() (string, error) { return "not-an-ip", nil }
	if got := ExtIP(c, fetch); got != "" {
		t.Fatalf("ExtIP on junk = %q, want empty", got)
	}
}

func TestExtIPEmptyOnFetchError(t *testing.T) {
	c := cache.New()
	fetch := func() (string, error) { return "", errors.New("down") }
	if got := ExtIP(c, fetch); got != "" {
		t.Fatalf("ExtIP on error = %q, want empty", got)
	}
}
```
`internal/hud/module/weather_test.go`:
```go
package module

import (
	"testing"

	"terminal-hud/internal/cache"
)

func TestWeatherReturnsTrimmedFetch(t *testing.T) {
	c := cache.New()
	fetch := func() (string, error) { return "☀ Atlanta 84°F\n", nil }
	if got := Weather(c, fetch); got != "☀ Atlanta 84°F" {
		t.Fatalf("Weather = %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** — `internal/hud/module/extip.go`:
```go
package module

import (
	"net"
	"net/http"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

// extIPTTL is how long a fetched external IP stays fresh.
const extIPTTL = 300 * time.Second

// FetchFunc returns a freshly fetched value or an error. Injected for testing.
type FetchFunc func() (string, error)

// ExtIP returns the public IP from api.ipify.org (cached extIPTTL). The fetched
// value is trimmed and validated as an IP; junk yields "".
func ExtIP(c *cache.Cache, fetch FetchFunc) string {
	return c.GetOrRefresh("extip", extIPTTL, func() (string, error) {
		raw, err := fetch()
		if err != nil {
			return "", err
		}
		ip := strings.TrimSpace(raw)
		if net.ParseIP(ip) == nil {
			return "", errInvalidIP
		}
		return ip, nil
	})
}

var errInvalidIP = &fetchError{"ext ip: response was not a valid IP"}

type fetchError struct{ msg string }

func (e *fetchError) Error() string { return e.msg }

// DefaultExtIPFetch performs the real HTTP GET against api.ipify.org with a
// bounded timeout.
func DefaultExtIPFetch() (string, error) { return httpGetString("https://api.ipify.org", "terminal-hud") }

// httpGetString GETs url with the given User-Agent and a 5s timeout, returning
// the body as a string.
func httpGetString(url, userAgent string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var b strings.Builder
	if _, err := copyMax(&b, resp.Body, 4096); err != nil {
		return "", err
	}
	return b.String(), nil
}
```
`internal/hud/module/weather.go`:
```go
package module

import (
	"io"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

const weatherTTL = 1800 * time.Second

// Weather returns a short current-conditions string from wttr.in (cached
// weatherTTL). The fetched value is trimmed; "" on error.
func Weather(c *cache.Cache, fetch FetchFunc) string {
	return c.GetOrRefresh("weather", weatherTTL, func() (string, error) {
		raw, err := fetch()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(raw), nil
	})
}

// DefaultWeatherFetch performs the real HTTP GET against wttr.in.
func DefaultWeatherFetch() (string, error) {
	return httpGetString("https://wttr.in/?format=%c+%l+%t", "terminal-hud")
}

// copyMax copies at most n bytes from r to w (guards against unbounded bodies).
func copyMax(w io.Writer, r io.Reader, n int64) (int64, error) {
	return io.Copy(w, io.LimitReader(r, n))
}
```

- [ ] **Step 4: Run — expect PASS.** gofmt, vet.
- [ ] **Step 5: Commit**
```bash
git add internal/hud/module/extip.go internal/hud/module/weather.go internal/hud/module/extip_test.go internal/hud/module/weather_test.go
git commit -m "feat(hud/module): extip and weather segments (cached, injected fetch)"
```

---

### Task 5: git + azure + k8s segments

**Files:** Create `internal/hud/module/git.go`, `azure.go`, `k8s.go`; tests alongside.

- [ ] **Step 1: Failing tests** — `internal/hud/module/git_test.go`:
```go
package module

import (
	"errors"
	"testing"
)

func TestGitCleanBranch(t *testing.T) {
	run := func(args ...string) (string, error) {
		switch {
		case args[0] == "rev-parse" && args[1] == "--abbrev-ref":
			return "main\n", nil
		case args[0] == "status":
			return "", nil // clean
		}
		return "", errors.New("unexpected " + args[0])
	}
	if got := Git(run); got != "main ✓" {
		t.Fatalf("Git clean = %q, want 'main ✓'", got)
	}
}

func TestGitDirtyCounts(t *testing.T) {
	run := func(args ...string) (string, error) {
		if args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return "feature\n", nil
		}
		if args[0] == "status" {
			return " M a.go\n?? b.go\n", nil // 2 changed entries
		}
		return "", errors.New("x")
	}
	if got := Git(run); got != "feature +2" {
		t.Fatalf("Git dirty = %q, want 'feature +2'", got)
	}
}

func TestGitEmptyWhenNotARepo(t *testing.T) {
	run := func(args ...string) (string, error) { return "", errors.New("not a git repo") }
	if got := Git(run); got != "" {
		t.Fatalf("Git non-repo = %q, want empty", got)
	}
}
```
`internal/hud/module/azure_test.go`:
```go
package module

import (
	"errors"
	"testing"

	"terminal-hud/internal/cache"
)

func TestAzureReturnsAccountName(t *testing.T) {
	c := cache.New()
	run := func() (string, error) { return "jmf-prod\n", nil }
	if got := Azure(c, run); got != "jmf-prod" {
		t.Fatalf("Azure = %q", got)
	}
}

func TestAzureEmptyOnError(t *testing.T) {
	c := cache.New()
	run := func() (string, error) { return "", errors.New("az missing") }
	if got := Azure(c, run); got != "" {
		t.Fatalf("Azure on error = %q, want empty", got)
	}
}
```
`internal/hud/module/k8s_test.go`:
```go
package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestK8sReadsCurrentContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfg := "apiVersion: v1\ncurrent-context: aks-dev\ncontexts:\n- name: aks-dev\n  context:\n    namespace: default\n"
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := K8s(path); got != "aks-dev/default" {
		t.Fatalf("K8s = %q, want 'aks-dev/default'", got)
	}
}

func TestK8sEmptyWhenFileMissing(t *testing.T) {
	if got := K8s(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Fatalf("K8s missing file = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** — `internal/hud/module/git.go`:
```go
package module

import (
	"os/exec"
	"strconv"
	"strings"
)

// RunFunc runs a git subcommand in the working directory and returns trimmed
// stdout, or an error. Injected for testing.
type RunFunc func(args ...string) (string, error)

// Git returns "<branch> ✓" when clean or "<branch> +N" with N changed entries.
// Returns "" when not in a repo (run returns an error for rev-parse).
func Git(run RunFunc) string {
	branch, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return ""
	}
	status, err := run("status", "--porcelain")
	if err != nil {
		return branch
	}
	n := 0
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	if n == 0 {
		return branch + " ✓"
	}
	return branch + " +" + strconv.Itoa(n)
}

// DefaultGitRun runs git in cwd with a short timeout-bounded context handled by
// the caller; here it execs directly. Returns "" trimmed stdout.
func DefaultGitRun(cwd string) RunFunc {
	return func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		return string(out), err
	}
}
```
`internal/hud/module/azure.go`:
```go
package module

import (
	"os/exec"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

const azureTTL = 60 * time.Second

// AzRunFunc returns the active Azure account name, or an error. Injected.
type AzRunFunc func() (string, error)

// Azure returns the active subscription/account name (cached azureTTL), "" if az
// is absent or errors.
func Azure(c *cache.Cache, run AzRunFunc) string {
	return c.GetOrRefresh("azure", azureTTL, func() (string, error) {
		out, err := run()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	})
}

// DefaultAzRun execs `az account show --query name -o tsv`.
func DefaultAzRun() (string, error) {
	out, err := exec.Command("az", "account", "show", "--query", "name", "-o", "tsv").Output()
	return string(out), err
}
```
`internal/hud/module/k8s.go`:
```go
package module

import (
	"os"
	"strings"
)

// K8s reads a kubeconfig file and returns "<current-context>/<namespace>", or
// just "<current-context>" if no namespace is set, or "" on any problem. This
// is a deliberately small line-scan (no kubectl, no YAML lib) sufficient for the
// common single-document kubeconfig.
func K8s(kubeconfigPath string) string {
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return ""
	}
	var current, namespace string
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "current-context:") {
			current = strings.TrimSpace(strings.TrimPrefix(t, "current-context:"))
		}
		if strings.HasPrefix(t, "namespace:") {
			namespace = strings.TrimSpace(strings.TrimPrefix(t, "namespace:"))
		}
	}
	if current == "" {
		return ""
	}
	if namespace == "" {
		return current
	}
	return current + "/" + namespace
}
```
> Note: `K8s` returns the first `namespace:` found, which for typical single-context configs is the active one. Correctly associating a namespace with the current context across multiple contexts is a documented follow-up; the SPEC accepts a simple line-scan.

- [ ] **Step 4: Run — expect PASS** (all module tests). gofmt, vet.
- [ ] **Step 5: Commit**
```bash
git add internal/hud/module/git.go internal/hud/module/azure.go internal/hud/module/k8s.go internal/hud/module/git_test.go internal/hud/module/azure_test.go internal/hud/module/k8s_test.go
git commit -m "feat(hud/module): git, azure, k8s segments"
```

---

### Task 6: bar wiring (TopBar / BottomBar)

**Files:** Create `internal/hud/bars.go`; test `internal/hud/bars_test.go`.

- [ ] **Step 1: Failing test** — `internal/hud/bars_test.go`:
```go
package hud

import (
	"testing"
)

func TestTopBarComposesSegments(t *testing.T) {
	d := Deps{
		Time:    "14:32:07",
		LocalIP: "192.168.1.42",
		ExtIP:   "72.14.201.9",
		Weather: "☀ ATL 84F",
	}
	cells := TopBar(40, d)
	got := barText(cells)
	// left: time | localip | extip ; weather right-aligned
	if !contains(got, "14:32:07 | 192.168.1.42 | 72.14.201.9") {
		t.Fatalf("top left wrong: %q", got)
	}
	if !endsWithTrimmed(got, "☀ ATL 84F") {
		t.Fatalf("weather not right-aligned: %q", got)
	}
}

func TestBottomBarOmitsEmptySegments(t *testing.T) {
	d := Deps{Git: "main ✓", Azure: "", K8s: "aks-dev/default"}
	got := barText(BottomBar(40, d))
	// azure empty -> no stray separator between git and k8s
	if !contains(got, "main ✓ | aks-dev/default") {
		t.Fatalf("bottom bar wrong: %q", got)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func endsWithTrimmed(s, suf string) bool {
	// trim trailing spaces then check suffix
	end := len(s)
	for end > 0 && s[end-1] == ' ' {
		end--
	}
	return end >= len(suf) && s[end-len(suf):end] == suf
}
```

- [ ] **Step 2: Run — expect FAIL** (undefined `Deps`, `TopBar`, `BottomBar`).

- [ ] **Step 3: Implement** — `internal/hud/bars.go`:
```go
package hud

// Deps holds the already-computed segment strings for one frame's bars. main's
// refresh goroutine fills these from the module functions; the bars are
// assembled synchronously on the render path (cheap string→cell work).
type Deps struct {
	// top bar
	Time    string
	LocalIP string
	ExtIP   string
	Weather string
	// bottom bar (path and exit are deferred until OSC support lands)
	Git   string
	Azure string
	K8s   string
}

// TopBar lays out time | localip | extip on the left and weather on the right.
func TopBar(width int, d Deps) []engine_Cell {
	left := []Segment{
		{Text: d.Time, Color: ColorWhite},
		{Text: d.LocalIP, Color: ColorGreen},
		{Text: d.ExtIP, Color: ColorYellow},
	}
	right := []Segment{{Text: d.Weather, Color: ColorCyan}}
	return AssembleBar(width, left, right)
}

// BottomBar lays out git | azure | k8s on the left.
func BottomBar(width int, d Deps) []engine_Cell {
	left := []Segment{
		{Text: d.Git, Color: ColorWhite},
		{Text: d.Azure, Color: ColorBlue},
		{Text: d.K8s, Color: ColorCyan},
	}
	return AssembleBar(width, left, nil)
}
```
> IMPORTANT: replace `engine_Cell` above with the real return type `[]engine.Cell` and add the import `"terminal-hud/internal/engine"` to `bars.go`. (The placeholder name is only to flag that you must import engine; do not leave `engine_Cell` in the code.)

- [ ] **Step 4: Run — expect PASS.** gofmt, vet.

- [ ] **Step 5: Full suite + race + build**
Run: `go test -race ./... ` (all packages pass), `go vet ./...`, `go build ./...`.

- [ ] **Step 6: Commit**
```bash
git add internal/hud/bars.go internal/hud/bars_test.go
git commit -m "feat(hud): TopBar/BottomBar wiring (path/exit deferred)"
```

---

## Self-Review

**Spec coverage (hud portion):**
- time, localip (UDP trick), extip (ipify, 300s, IP-validated), weather (wttr.in, 1800s, UA terminal-hud), git (branch + porcelain count), azure (`az account show`, 60s), k8s (kubeconfig scan) → Tasks 3-5 ✓
- named-ANSI role colors (time=white, localip=green, extip=yellow, weather=cyan, git=white, azure=blue, k8s=cyan, separator=dim) → Task 2 ✓
- bar assembly into cells with separators + weather right-aligned → Task 2 (AssembleBar) + Task 6 (wiring) ✓
- cache (TTL) → Task 1 (in-memory, replacing v2.1 file cache — documented deviation) ✓
- path, exit → DEFERRED (OSC), per decision ✓

**Deviations (documented):**
- In-memory cache instead of v2.1 file cache + file lock (v3.1 is long-lived in-process; SPEC says a mutex suffices). No `internal/lock` package built.
- `git` dirty count counts porcelain lines (changed entries) rather than separate staged/unstaged; the SPEC mockup shows "+2" which this matches. The 200ms timeout from the SPEC is applied by the caller (main) via context or by `DefaultGitRun` wrapping; noted as a refinement (the injected RunFunc makes timeout policy the caller's choice).

**Placeholder scan:** the only intentional sentinel is `engine_Cell` in Task 6 Step 3, explicitly flagged to be replaced with `[]engine.Cell` + the engine import. No other placeholders.

**Type consistency:** `cache.New()`, `(*cache.Cache).GetOrRefresh(key, ttl, refresh)`; `Segment{Text,Color}`; `AssembleBar(width, left, right) []engine.Cell`; module fns `Time(now)`, `LocalIP(DialFunc)`, `ExtIP(*cache.Cache, FetchFunc)`, `Weather(*cache.Cache, FetchFunc)`, `Git(RunFunc)`, `Azure(*cache.Cache, AzRunFunc)`, `K8s(path)`; `Deps`, `TopBar/BottomBar(width, Deps)`. `FetchFunc` shared by extip/weather. Color vars are `engine.Color` values (comparable).

**Follow-ups (later plans):** OSC parsing in engine → path + exit segments; main wires the refresh ticker (calls module fns with real deps: DefaultExtIPFetch, DefaultGitRun(cwd from OSC7), etc.), fills Deps, and calls TopBar/BottomBar each frame; git 200ms timeout policy; per-context k8s namespace.
