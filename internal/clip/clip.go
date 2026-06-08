// Package clip writes text to the system clipboard. On macOS it shells out to
// pbcopy; everywhere it can emit an OSC 52 escape that terminals forward to the
// clipboard, which CopyTo uses for the in-terminal path.
package clip

import (
	"encoding/base64"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

// OSC52 returns the OSC 52 clipboard-set escape for s (base64-encoded).
func OSC52(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	return "\x1b]52;c;" + enc + "\x07"
}

// CopyTo writes s to the clipboard by emitting OSC 52 to w (the terminal). Use
// this from the render path where w is the real terminal.
func CopyTo(w io.Writer, s string) error {
	_, err := io.WriteString(w, OSC52(s))
	return err
}

// Copy writes s to the clipboard out-of-band. On macOS it uses pbcopy; on other
// platforms it returns ErrNoNativeClipboard so callers fall back to CopyTo
// (OSC 52). Keeping this separate lets main choose per platform.
func Copy(s string) error {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(s)
		return cmd.Run()
	}
	return ErrNoNativeClipboard
}

// ErrNoNativeClipboard signals no native clipboard tool; fall back to OSC 52.
var ErrNoNativeClipboard = errNoClip{}

type errNoClip struct{}

func (errNoClip) Error() string { return "clip: no native clipboard; use OSC 52" }
