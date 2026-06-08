package module

import (
	"os/exec"
	"strconv"
	"strings"
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

// DefaultGitRun runs git in cwd. Callers are responsible for bounding execution
// time (e.g. context+timeout) before this reaches the render path.
func DefaultGitRun(cwd string) RunFunc {
	return func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		out, err := cmd.Output()
		return string(out), err
	}
}
