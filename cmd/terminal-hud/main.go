// Command terminal-hud runs the user's shell inside a screen-owning terminal
// with frozen top/bottom HUD bars and a scrollable middle.
package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"terminal-hud/internal/cache"
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
	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			os.Stdout.WriteString(input.DisableMouse + "\x1b[?25h\x1b[?1049l")
			term.Restore(stdin, oldState) //nolint:errcheck — best-effort restore
		})
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
	// Note: this goroutine blocks on os.Stdin.Read and leaks until process exit (acceptable).
	go pump(os.Stdin, stdinBytes)

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	renderTick := time.NewTicker(time.Second) // clock + coalesced redraw
	defer renderTick.Stop()

	// HUD goroutine: runs compute() (slow — network/exec) off the event loop.
	// Sends fresh hud.Deps values to the loop via a buffered channel; the loop
	// owns deps exclusively (no shared mutable state).
	refresh := newHUDRefresher()
	hudDeps := make(chan hud.Deps, 1)
	go func() {
		deliver := func() {
			d := refresh.compute() // may block on network/exec — off the event loop
			select {
			case hudDeps <- d:
			default: // loop busy; drop, next tick retries
			}
		}
		deliver() // initial fetch
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			deliver()
		}
	}()

	var deps hud.Deps
	deps.Time = module.Time(time.Now()) // show clock immediately
	sess.SetDeps(deps)
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

		case d := <-hudDeps:
			deps = d
			sess.SetDeps(deps)
			rd.Render(sess.Frame())

		case <-renderTick.C:
			// Update only the clock on the last-known deps; don't clobber fetched values.
			deps.Time = module.Time(time.Now())
			sess.SetDeps(deps)
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

// hudRefresher computes a fresh hud.Deps on demand. compute() may block on
// network or exec; it is always called from the HUD goroutine, never from the
// event loop.
type hudRefresher struct{ c *cache.Cache }

func newHUDRefresher() *hudRefresher { return &hudRefresher{c: cache.New()} }

func (r *hudRefresher) compute() hud.Deps {
	cwd, _ := os.Getwd()
	return hud.Deps{
		Time:    module.Time(time.Now()),
		LocalIP: module.LocalIP(net.Dial),
		ExtIP:   module.ExtIP(r.c, module.DefaultExtIPFetch),
		Weather: module.Weather(r.c, module.DefaultWeatherFetch),
		Git:     module.Git(module.DefaultGitRun(cwd)),
		Azure:   module.Azure(r.c, module.DefaultAzRun),
		K8s:     module.K8s(kubeconfigPath()),
	}
}

// kubeconfigPath returns the first $KUBECONFIG entry if set and non-empty,
// else ~/.kube/config.
func kubeconfigPath() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		parts := strings.SplitN(kc, string(os.PathListSeparator), 2)
		if parts[0] != "" {
			return parts[0]
		}
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kube", "config")
}
