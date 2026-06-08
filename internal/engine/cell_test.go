package engine

import "testing"

func TestColorZeroValueIsRGBBlack(t *testing.T) {
	var c Color
	if c.IsDefault || c.IsIndexed || c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatalf("zero Color should be RGB black, got %+v", c)
	}
}

func TestAttrBoldSetAndClear(t *testing.T) {
	var a Attr
	if a.Has(AttrBold) {
		t.Fatal("zero Attr should have no bold")
	}
	a |= AttrBold
	if !a.Has(AttrBold) {
		t.Fatal("AttrBold not set")
	}
}
