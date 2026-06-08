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
