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
