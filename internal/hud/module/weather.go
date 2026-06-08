package module

import (
	"io"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

const weatherTTL = 1800 * time.Second

// Weather returns a short current-conditions string from wttr.in (cached
// weatherTTL). The fetched value is trimmed; "" on error.
func Weather(c *cache.Cache, fetch FetchFunc) string {
	return c.GetOrRefresh("weather", weatherTTL, func() (string, error) {
		raw, err := fetch()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(raw), nil
	})
}

// DefaultWeatherFetch performs the real HTTP GET against wttr.in.
func DefaultWeatherFetch() (string, error) {
	return httpGetString("https://wttr.in/?format=%c+%l+%t", "terminal-hud")
}

// copyMax copies at most n bytes from r to w (guards against unbounded bodies).
func copyMax(w io.Writer, r io.Reader, n int64) (int64, error) {
	return io.Copy(w, io.LimitReader(r, n))
}
