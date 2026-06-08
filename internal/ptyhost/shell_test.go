package ptyhost

import "testing"

func TestResolveShellPrefersExplicit(t *testing.T) {
	if got := ResolveShell("/bin/zsh"); got != "/bin/zsh" {
		t.Fatalf("ResolveShell(/bin/zsh) = %q, want /bin/zsh", got)
	}
}

func TestResolveShellFallsBackToEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := ResolveShell(""); got != "/usr/bin/fish" {
		t.Fatalf("ResolveShell(\"\") with $SHELL set = %q, want /usr/bin/fish", got)
	}
}

func TestResolveShellDefaultsToBinSh(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := ResolveShell(""); got != "/bin/sh" {
		t.Fatalf("ResolveShell(\"\") with no $SHELL = %q, want /bin/sh", got)
	}
}
