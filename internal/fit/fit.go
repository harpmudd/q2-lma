// Package fit measures text as the Q2 draws it and shortens it to a width, so a row's label never
// scrolls. Q2 Pod's rows are scrolling labels (stock's s_scrlabel styles), which move whenever their
// text is wider than they are; a list full of them is hard to read.
//
// Widths are Liberation Sans Bold's advances (Classic's bold labels, navigation.c bold_label),
// the wider of the two fonts a row is drawn in: Minimal's is stock's, Arial, whose regular weight
// is narrower. A character neither font is likely to have (★, CJK) counts as a full em, as stock's
// CJK font draws it.
package fit

import (
	"strings"
	"unicode/utf8"
)

// ascii is Liberation Sans Bold's advance, in thousandths of an em, for ' ' to '~'.
var ascii = [95]uint16{278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278, 556, 556,
	556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611, 975, 722, 722, 722, 722, 667, 611, 778,
	722, 278, 556, 722, 611, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333,
	584, 556, 333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611, 611, 611, 389, 556,
	333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584}

var other = map[rune]uint16{0xa0: 278, 0xb7: 333, 0x2018: 278, 0x2019: 278, 0x201c: 500, 0x201d: 500,
	0x2013: 556, 0x2014: 1000, 0x2026: 1000}

func em(r rune) int {
	switch {
	case r >= ' ' && r <= '~':
		return int(ascii[r-' '])
	case other[r] != 0:
		return int(other[r])
	case r >= 0xc0 && r <= 0x24f: // Latin accents: a lowercase letter's width or an uppercase one's
		return 667
	}
	return 1000
}

// Width is s's width in pixels at font size px (rounded up).
func Width(s string, px int) int {
	t := 0
	for _, r := range s {
		t += em(r)
	}
	return (t*px + 999) / 1000
}

// Fits reports whether s fits in w pixels at size px.
func Fits(s string, px, w int) bool { return Width(s, px) <= w }

// Cut shortens s to fit w pixels at size px, ending in "…": at a word boundary when one falls in
// the last third of what fits, else mid-word. s is returned as it is when it fits.
func Cut(s string, px, w int) string {
	if Fits(s, px, w) {
		return s
	}
	room := w - Width("…", px)
	t, end := 0, 0
	for i, r := range s {
		if (t+em(r))*px > room*1000 {
			break
		}
		t += em(r)
		end = i + utf8.RuneLen(r)
	}
	cut := s[:end]
	if sp := strings.LastIndexAny(cut, " ,·-/"); sp > len(cut)*2/3 {
		cut = cut[:sp]
	}
	return strings.TrimRight(cut, " ,·-/") + "…"
}

// Wrap splits s over a row's two labels, so it reads whole rather than scrolling or being cut: as
// much as fits the first at size px1 and width w1, broken at a space, and the rest in the second
// (px2, w2), cut only if even that won't hold it.
func Wrap(s string, px1, w1, px2, w2 int) (first, second string) {
	if Fits(s, px1, w1) {
		return s, ""
	}
	t, end := 0, 0
	for i, r := range s {
		if (t+em(r))*px1 > w1*1000 {
			break
		}
		t += em(r)
		end = i + utf8.RuneLen(r)
	}
	if sp := strings.LastIndex(s[:end], " "); sp > 0 {
		end = sp
	}
	return s[:end], Cut(strings.TrimSpace(s[end:]), px2, w2)
}

// First is the first of options that fits w pixels at size px, else the last cut to fit.
func First(px, w int, options ...string) string {
	for _, o := range options {
		if Fits(o, px, w) {
			return o
		}
	}
	if len(options) == 0 {
		return ""
	}
	return Cut(options[len(options)-1], px, w)
}
