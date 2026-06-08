package module

import (
	"errors"
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

func TestWeatherEmptyOnFetchError(t *testing.T) {
	c := cache.New()
	fetch := func() (string, error) { return "", errors.New("wttr down") }
	if got := Weather(c, fetch); got != "" {
		t.Fatalf("Weather on error = %q, want empty", got)
	}
}
