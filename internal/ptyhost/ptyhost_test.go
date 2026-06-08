package ptyhost

import (
	"bytes"
	"io"
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

func TestPtyHostCloseUnblocksAndReturnsEOF(t *testing.T) {
	// A long-lived child that produces no output; a blocked Read must be
	// released by Close, and subsequent reads must report EOF (readCh closed).
	p, err := Start("/bin/sleep", []string{"5"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Give readLoop a moment to observe the closed master and close readCh.
	if err := p.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 64)
	// Drain until EOF or deadline; must terminate (not hang) and end in EOF.
	// Bounded loop: at most 100 iterations to avoid an infinite busy-spin if
	// the deadline keeps firing instead of EOF arriving.
	for i := 0; i < 100; i++ {
		_, rerr := p.Read(buf)
		if rerr == io.EOF {
			return // success
		}
		if rerr != nil {
			// Any other terminal error (e.g. EIO surfaced as the error chunk)
			// is acceptable — continue to reach the EOF after the error chunk.
			continue
		}
	}
	t.Fatal("Read did not reach EOF")
}

func TestPtyHostDoubleCloseIsSafe(t *testing.T) {
	p, err := Start("/bin/sleep", []string{"5"}, 24, 80)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Must not panic; second close returns either nil or an error, both fine.
	_ = p.Close()
}

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
