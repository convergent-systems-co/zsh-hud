package engine

import "testing"

func TestEngineReadsBackPlainText(t *testing.T) {
	e, err := New(24, 80, 1000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if _, err := e.Write([]byte("Hello, \x1b[1;32mworld\x1b[0m!")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got []rune
	for col := 0; col < 14; col++ {
		c := e.Cell(0, col)
		if c.Rune != 0 {
			got = append(got, c.Rune)
		}
	}
	if string(got) != "Hello, world!" {
		t.Fatalf("row 0 = %q, want %q", string(got), "Hello, world!")
	}
}
