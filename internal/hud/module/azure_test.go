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
