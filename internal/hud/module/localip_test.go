package module

import (
	"errors"
	"net"
	"testing"
	"time"
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
