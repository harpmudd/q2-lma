package fit

import (
	"strings"
	"testing"
)

func TestWidth(t *testing.T) {
	// "Barton Hall" in Liberation Sans Bold: B722 a556 r389 t333 o611 n611 sp278 H722 a556 l278 l278
	if got := Width("Barton Hall", 1000); got != 5334 {
		t.Errorf("width %d", got)
	}
	if Width("★", 20) != 20 || Width("", 20) != 0 {
		t.Error("em and empty")
	}
}

func TestCut(t *testing.T) {
	s := "May 8 · War Memorial Auditorium, Buffalo"
	c := Cut(s, 20, 280)
	if !Fits(c, 20, 280) || !strings.HasSuffix(c, "…") || !strings.HasPrefix(c, "May 8 · War Memorial") {
		t.Errorf("%q (%d px)", c, Width(c, 20))
	}
	if Cut("Barton Hall", 20, 280) != "Barton Hall" {
		t.Error("cut what fits")
	}
	if c := Cut("Ünïcödé ★★★★★★★★★★★★★★★★★★★", 20, 100); !Fits(c, 20, 100) || !strings.HasPrefix(c, "Ünïcöd") {
		t.Errorf("%q", c)
	}
}

func TestFirst(t *testing.T) {
	if got := First(16, 100, "a very long first option indeed", "short"); got != "short" {
		t.Errorf("%q", got)
	}
	if got := First(16, 30, "too long", "also too long"); !Fits(got, 16, 30) {
		t.Errorf("%q", got)
	}
}
