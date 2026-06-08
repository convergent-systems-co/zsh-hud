package ptyhost

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

// chunk is a unit of data delivered by the background reader goroutine.
type chunk struct {
	data []byte
	err  error
}

// PtyHost owns a pseudo-terminal master and the child process attached to its
// slave. Read returns the child's output; Write sends input to the child.
// Not safe for concurrent Read+Read or Write+Write, but a single reader and a
// single writer on separate goroutines is fine (independent fd directions).
type PtyHost struct {
	master *os.File
	cmd    *exec.Cmd

	// reads are delivered through readCh from the background reader goroutine.
	// macOS pty fds do not support os.File deadline semantics, so we run a
	// dedicated goroutine and implement SetReadDeadline via select + timer.
	readCh chan chunk

	done      chan struct{} // closed once by Close; signals readLoop to stop
	closeOnce sync.Once     // guards master.Close + done close

	// pending holds leftover bytes from the last chunk that were not fully
	// consumed by a Read call.
	pendingMu sync.Mutex
	pending   []byte

	deadlineMu sync.Mutex
	deadline   time.Time // zero means no deadline
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

	p := &PtyHost{
		master: master,
		cmd:    cmd,
		readCh: make(chan chunk, 16),
		done:   make(chan struct{}),
	}
	go p.readLoop()
	return p, nil
}

// readLoop runs in a dedicated goroutine, reading from the pty master and
// forwarding chunks to readCh. It exits when the master is closed (EIO / EOF)
// or when done is closed by Close, whichever comes first.
func (p *PtyHost) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := p.master.Read(buf)
		if n > 0 {
			cp := make([]byte, n)
			copy(cp, buf[:n])
			select {
			case p.readCh <- chunk{data: cp}:
			case <-p.done:
				return
			}
		}
		if err != nil {
			select {
			case p.readCh <- chunk{err: err}:
			case <-p.done:
			}
			close(p.readCh) // no more data will ever arrive
			return
		}
	}
}

// Read returns bytes from the child's output. Implements io.Reader.
// If a deadline is set via SetReadDeadline and it expires before data arrives,
// Read returns (0, os.ErrDeadlineExceeded).
func (p *PtyHost) Read(b []byte) (int, error) {
	// Drain any leftover bytes from a previous chunk first.
	p.pendingMu.Lock()
	if len(p.pending) > 0 {
		n := copy(b, p.pending)
		p.pending = p.pending[n:]
		p.pendingMu.Unlock()
		return n, nil
	}
	p.pendingMu.Unlock()

	p.deadlineMu.Lock()
	dl := p.deadline
	p.deadlineMu.Unlock()

	var timer <-chan time.Time
	if !dl.IsZero() {
		d := time.Until(dl)
		if d <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		tm := time.NewTimer(d)
		defer tm.Stop()
		timer = tm.C
	}

	select {
	case c, ok := <-p.readCh:
		if !ok {
			return 0, io.EOF
		}
		if c.err != nil {
			return 0, c.err
		}
		n := copy(b, c.data)
		if n < len(c.data) {
			// Store the unconsumed remainder.
			p.pendingMu.Lock()
			p.pending = append(p.pending, c.data[n:]...)
			p.pendingMu.Unlock()
		}
		return n, nil
	case <-timer:
		return 0, os.ErrDeadlineExceeded
	}
}

// Write sends input bytes to the child. Implements io.Writer.
func (p *PtyHost) Write(b []byte) (int, error) { return p.master.Write(b) }

// SetReadDeadline sets a deadline for future Read calls; a zero time clears it.
// Useful for non-blocking reads in the event loop and for bounded test reads.
// Unlike os.File.SetReadDeadline, this is implemented via an internal timer
// because macOS pty fds do not support deadline semantics.
func (p *PtyHost) SetReadDeadline(t time.Time) error {
	p.deadlineMu.Lock()
	p.deadline = t
	p.deadlineMu.Unlock()
	return nil
}

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

// Close closes the pty master and signals readLoop to stop. Idempotent: safe
// to call more than once; only the first call has effect. Callers should still
// Wait on the child process to reap it.
func (p *PtyHost) Close() error {
	var err error
	p.closeOnce.Do(func() {
		err = p.master.Close()
		close(p.done)
	})
	return err
}
