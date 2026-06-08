package module

import (
	"context"
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

// DefaultAzRun execs `az account show --query name -o tsv` with a 3s timeout.
func DefaultAzRun() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "az", "account", "show", "--query", "name", "-o", "tsv").Output()
	return string(out), err
}
