package module

import (
	"errors"
	"testing"
)

func TestGitCleanBranch(t *testing.T) {
	run := func(args ...string) (string, error) {
		switch {
		case args[0] == "rev-parse" && args[1] == "--abbrev-ref":
			return "main\n", nil
		case args[0] == "status":
			return "", nil // clean
		}
		return "", errors.New("unexpected " + args[0])
	}
	if got := Git(run); got != "main ✓" {
		t.Fatalf("Git clean = %q, want 'main ✓'", got)
	}
}

func TestGitDirtyCounts(t *testing.T) {
	run := func(args ...string) (string, error) {
		if args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return "feature\n", nil
		}
		if args[0] == "status" {
			return " M a.go\n?? b.go\n", nil // 2 changed entries
		}
		return "", errors.New("x")
	}
	if got := Git(run); got != "feature +2" {
		t.Fatalf("Git dirty = %q, want 'feature +2'", got)
	}
}

func TestGitEmptyWhenNotARepo(t *testing.T) {
	run := func(args ...string) (string, error) { return "", errors.New("not a git repo") }
	if got := Git(run); got != "" {
		t.Fatalf("Git non-repo = %q, want empty", got)
	}
}
