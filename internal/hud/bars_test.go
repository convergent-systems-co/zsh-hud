package hud

import (
	"testing"
)

func TestTopBarComposesSegments(t *testing.T) {
	d := Deps{
		Time:    "14:32:07",
		LocalIP: "192.168.1.42",
		ExtIP:   "72.14.201.9",
		Weather: "☀ ATL 84F",
	}
	cells := TopBar(50, d)
	got := barText(cells)
	if !contains(got, "14:32:07 | 192.168.1.42 | 72.14.201.9") {
		t.Fatalf("top left wrong: %q", got)
	}
	if !endsWithTrimmed(got, "☀ ATL 84F") {
		t.Fatalf("weather not right-aligned: %q", got)
	}
}

func TestBottomBarOmitsEmptySegments(t *testing.T) {
	d := Deps{Git: "main ✓", Azure: "", K8s: "aks-dev/default"}
	got := barText(BottomBar(40, d))
	if !contains(got, "main ✓ | aks-dev/default") {
		t.Fatalf("bottom bar wrong: %q", got)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func endsWithTrimmed(s, suf string) bool {
	end := len(s)
	for end > 0 && s[end-1] == ' ' {
		end--
	}
	return end >= len(suf) && s[end-len(suf):end] == suf
}
