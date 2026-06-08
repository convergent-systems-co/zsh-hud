package module

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

// extIPTTL is how long a fetched external IP stays fresh.
const extIPTTL = 300 * time.Second

// FetchFunc returns a freshly fetched value or an error. Injected for testing.
type FetchFunc func() (string, error)

// ExtIP returns the public IP from api.ipify.org (cached extIPTTL). The fetched
// value is trimmed and validated as an IP; junk yields "".
func ExtIP(c *cache.Cache, fetch FetchFunc) string {
	return c.GetOrRefresh("extip", extIPTTL, func() (string, error) {
		raw, err := fetch()
		if err != nil {
			return "", err
		}
		ip := strings.TrimSpace(raw)
		if net.ParseIP(ip) == nil {
			return "", errInvalidIP
		}
		return ip, nil
	})
}

var errInvalidIP = &fetchError{"ext ip: response was not a valid IP"}

type fetchError struct{ msg string }

func (e *fetchError) Error() string { return e.msg }

// DefaultExtIPFetch performs the real HTTP GET against api.ipify.org with a
// bounded timeout.
func DefaultExtIPFetch() (string, error) {
	return httpGetString("https://api.ipify.org", "terminal-hud")
}

// httpGetString GETs url with the given User-Agent and a 5s timeout, returning
// the body as a string.
func httpGetString(url, userAgent string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	var b strings.Builder
	if _, err := copyMax(&b, resp.Body, 4096); err != nil {
		return "", err
	}
	return b.String(), nil
}
