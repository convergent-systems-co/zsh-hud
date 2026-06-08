package clip

import (
	"strings"
	"testing"
)

func TestOSC52EncodesBase64(t *testing.T) {
	got := OSC52("hi")
	// ESC ] 52 ; c ; <base64> BEL ; base64("hi") = "aGk="
	if got != "\x1b]52;c;aGk=\x07" {
		t.Fatalf("OSC52 = %q", got)
	}
}

func TestOSC52Empty(t *testing.T) {
	if got := OSC52(""); got != "\x1b]52;c;\x07" {
		t.Fatalf("OSC52(empty) = %q", got)
	}
}

func TestCopyToWriterUsesOSC52(t *testing.T) {
	var b strings.Builder
	if err := CopyTo(&b, "ok"); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if !strings.Contains(b.String(), "\x1b]52;c;") {
		t.Fatalf("CopyTo should emit OSC52; got %q", b.String())
	}
}
