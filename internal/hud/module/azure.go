package module

import (
	"os/exec"
	"strings"
	"time"

	"terminal-hud/internal/cache"
)

const azureTTL = 60 * time.Second

// AzRunFunc returns the active Azure account name, or an error. Injected.
type AzRunFunc func() (string, error)

// Azure returns the active subscription/account name (cached azureTTL), "" if az
// is absent or errors.
func Azure(c *cache.Cache, run AzRunFunc) string {
	return c.GetOrRefresh("azure", azureTTL, func() (string, error) {
		out, err := run()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	})
}

// DefaultAzRun execs `az account show --query name -o tsv`.
func DefaultAzRun() (string, error) {
	out, err := exec.Command("az", "account", "show", "--query", "name", "-o", "tsv").Output()
	return string(out), err
}
