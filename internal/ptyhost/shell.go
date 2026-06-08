package ptyhost

import "os"

// defaultShell is the POSIX fallback when neither an explicit shell nor $SHELL
// is available.
const defaultShell = "/bin/sh"

// ResolveShell picks the shell to spawn: an explicit path wins; otherwise $SHELL;
// otherwise /bin/sh. It does not verify the path exists — that surfaces as a
// Start error, which is the correct boundary for that failure.
func ResolveShell(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return defaultShell
}
