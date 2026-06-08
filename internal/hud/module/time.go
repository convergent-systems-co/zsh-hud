// Package module holds the individual HUD segment functions. Each returns the
// segment's text, or "" to omit it, and takes its external dependencies as
// arguments so it is unit-testable without real I/O.
package module

import "time"

// Time returns the clock as HH:MM:SS.
func Time(now time.Time) string { return now.Format("15:04:05") }
