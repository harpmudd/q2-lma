package ia

import (
	"sort"
	"strings"
)

// Class is how a recording was made, best first: from the board, a mix of board and audience, not
// known, or from the audience.
type Class int

const (
	Audience Class = iota
	Unknown
	Matrix
	Soundboard
)

// Label is the class as a show row's subtitle names it ("" when not known).
func (c Class) Label() string {
	return [...]string{"AUD", "", "Matrix", "SBD"}[c]
}

// Taper's words in an identifier or a source line. Microphone makes mean an audience recording.
var (
	sbdWords = map[string]bool{"sbd": true, "soundboard": true, "board": true, "fm": true}
	mtxWords = map[string]bool{"mtx": true, "matrix": true}
	audWords = map[string]bool{"aud": true, "audience": true, "schoeps": true, "akg": true, "neumann": true,
		"dpa": true, "sennheiser": true, "senn": true, "nak": true, "nakamichi": true, "shure": true,
		"mk4": true, "mk41": true, "mk21": true, "mk22": true, "km184": true, "km140": true, "at4051": true,
		"at853": true, "cmc6": true}
)

// ClassOf reads the class from the identifier first, then the source line: an identifier that
// says sbd wins over a source that says matrix (the Cornell '77 soundboard,
// gd77-05-08.sbd.hicks…, has source "Matrix (see notes)").
func ClassOf(r Recording) Class {
	for _, s := range []string{r.ID, string(r.Source)} {
		words := strings.FieldsFunc(strings.ToLower(s), func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9')
		})
		var sbd, mtx, aud bool
		for _, w := range words {
			sbd = sbd || sbdWords[w]
			mtx = mtx || mtxWords[w]
			aud = aud || audWords[w]
		}
		switch {
		case sbd:
			return Soundboard
		case mtx:
			return Matrix
		case aud:
			return Audience
		}
	}
	return Unknown
}

// Rating damps the average for few reviews: five reviews at 5.0 rank below 300 at 4.8. No rating
// counts as 4.0.
func Rating(r Recording) float64 {
	const prior, weight = 4.0, 5.0
	n := float64(r.Reviews)
	if r.Rating <= 0 {
		n = 0
	}
	return (float64(r.Rating)*n + prior*weight) / (n + weight)
}

// Rank sorts recordings of one date best first: class, then damped rating, then downloads.
func Rank(rs []Recording) {
	sort.SliceStable(rs, func(i, j int) bool {
		ci, cj := ClassOf(rs[i]), ClassOf(rs[j])
		if ci != cj {
			return ci > cj
		}
		if ri, rj := Rating(rs[i]), Rating(rs[j]); ri != rj {
			return ri > rj
		}
		return rs[i].Downloads > rs[j].Downloads
	})
}

// Show is one date and its recordings, best first.
type Show struct {
	Date     string
	Versions []Recording
}

// Shows groups recordings by date, dates in order, each date's recordings ranked.
func Shows(rs []Recording) []Show {
	by := map[string][]Recording{}
	var dates []string
	for _, r := range rs {
		d := r.Day()
		if d == "" {
			continue
		}
		if by[d] == nil {
			dates = append(dates, d)
		}
		by[d] = append(by[d], r)
	}
	sort.Strings(dates)
	out := make([]Show, len(dates))
	for i, d := range dates {
		Rank(by[d])
		out[i] = Show{d, by[d]}
	}
	return out
}

// Place is where a recording was made, for a show's row: the venue, and the city when there's
// room.
func Place(r Recording) string {
	v, c := strings.TrimSpace(string(r.Venue)), strings.TrimSpace(string(r.Coverage))
	switch {
	case v == "":
		return c
	case c == "" || strings.Contains(v, c):
		return v
	}
	return v + ", " + c
}
