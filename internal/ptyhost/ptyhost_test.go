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
