package module

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// RunFunc runs a git subcommand in the working directory and returns trimmed
// stdout, or an error. Injected for testing.
type RunFunc func(args ...string) (string, error)

// Git returns "<branch> ✓" when clean or "<branch> +N" with N changed entries.
// Returns "" when not in a repo (run returns an error for rev-parse).
func Git(run RunFunc) string {
	branch, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return ""
	}
	status, err := run("status", "--porcelain")
	if err != nil {
		return branch
	}
	n := 0
	for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	if n == 0 {
		return branch + " ✓"
	}
	return branch + " +" + strconv.Itoa(n)
}

// DefaultGitRun runs git in cwd with a 3s timeout.
func DefaultGitRun(cwd string) RunFunc {
	return func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		return string(out), err
	}
}
