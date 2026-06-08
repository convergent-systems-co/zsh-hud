package module

import (
	"testing"
	"time"
)

func TestTimeFormatsHHMMSS(t *testing.T) {
	now := time.Date(2026, 6, 8, 14, 32, 7, 0, time.UTC)
	if got := Time(now); got != "14:32:07" {
		t.Fatalf("Time = %q, want 14:32:07", got)
	}
}
