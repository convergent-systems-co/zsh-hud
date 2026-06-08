package engine

import "testing"

func line(n int) []Cell { return []Cell{{Rune: rune('0' + n)}} }

func TestScrollbackPushAndRead(t *testing.T) {
	sb := newScrollback(3)
	sb.push(line(1))
	sb.push(line(2))
	if sb.len() != 2 {
		t.Fatalf("len = %d, want 2", sb.len())
	}
	// line(0) is the most recently pushed-off line (closest to the screen).
	if got := sb.line(0); got[0].Rune != '2' {
		t.Fatalf("line(0) = %q, want '2'", got[0].Rune)
	}
	if got := sb.line(1); got[0].Rune != '1' {
		t.Fatalf("line(1) = %q, want '1'", got[0].Rune)
	}
}

func TestScrollbackEvictsOldestAtCap(t *testing.T) {
	sb := newScrollback(2)
	sb.push(line(1))
	sb.push(line(2))
	sb.push(line(3)) // evicts line(1)
	if sb.len() != 2 {
		t.Fatalf("len = %d, want 2", sb.len())
	}
	if got := sb.line(1); got[0].Rune != '2' {
		t.Fatalf("oldest line = %q, want '2' (line 1 evicted)", got[0].Rune)
	}
}

func TestScrollbackPopReturnsMostRecent(t *testing.T) {
	sb := newScrollback(3)
	sb.push(line(1))
	sb.push(line(2))
	got, ok := sb.pop()
	if !ok || got[0].Rune != '2' {
		t.Fatalf("pop = %v,%v want '2',true", got, ok)
	}
	if sb.len() != 1 {
		t.Fatalf("len after pop = %d, want 1", sb.len())
	}
}
