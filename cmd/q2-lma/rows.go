package main

import (
	"fmt"
	"strings"
	"time"

	"q2-lma/internal/fit"
	"q2-lma/internal/ia"
	"q2-lma/internal/q2spot"
)

// A row's labels as Q2 Pod lays them out (coverflow.c page_row_detail, row_into; addon.c
// progress_row): x 36, 303 px wide, or 280 beside a list row's chevron; the title in 20 px type,
// the subtitle in 16. A progress row puts its title in the left three fifths and its subtitle in
// the rest. A label wider than its text scrolls, so every row is fitted; a few pixels spare cover
// rounding in the firmware's own measure.
const (
	rowWide     = 303
	rowChevron  = 280
	titlePx     = 20
	subtitlePx  = 16
	spare       = 4
	progressCol = rowWide * 3 / 5
)

func chevron(r q2spot.Row) bool {
	return r.Kind == q2spot.KindList || r.Kind == q2spot.KindRemote ||
		r.Kind == q2spot.KindLocal && strings.HasSuffix(r.URI, "/")
}

// fitRows shortens every row's title and subtitle to its label, so nothing scrolls.
func fitRows(r *q2spot.Reply) {
	for i := range r.Rows {
		row := &r.Rows[i]
		tw, sw := rowWide, rowWide
		switch {
		case row.Kind == q2spot.KindProgress:
			tw, sw = progressCol, rowWide-progressCol
		case chevron(*row):
			tw, sw = rowChevron, rowChevron
		}
		row.Title = fit.Cut(row.Title, titlePx, tw-spare)
		row.Subtitle = fit.Cut(row.Subtitle, subtitlePx, sw-spare)
	}
}

// showRow is a year's row for one date: the date and venue on top, the city and the recording that
// plays underneath, each shortened by dropping detail before anything is cut.
//
//	May 8 · Barton Hall                 >
//	Ithaca, NY · SBD 4.8★ · 23 versions
func showRow(ctx string, s ia.Show, best ia.Recording) q2spot.Row {
	date := s.Date
	if t, err := time.Parse("2006-01-02", s.Date); err == nil {
		date = t.Format("Jan 2")
	}
	venue := strings.TrimSpace(string(best.Venue))
	city := tidyPlace(string(best.Coverage))
	if venue == "" {
		venue, city = city, ""
	}
	short, _, _ := strings.Cut(venue, ",")
	w := rowChevron - spare
	title := date
	if venue != "" {
		title = fit.First(titlePx, w, date+" · "+venue, date+" · "+strings.TrimSpace(short))
	}
	versions := ""
	if len(s.Versions) > 1 {
		versions = plural(len(s.Versions), "version")
	}
	// "SBD 4.8★": the source and rating as one part, the room a dot would take left to the city
	d := ia.ClassOf(best).Label()
	if best.Rating > 0 {
		d = strings.TrimSpace(fmt.Sprintf("%s %.1f★", d, float64(best.Rating)))
	}
	sub := fit.First(subtitlePx, w, join(city, d, versions), join(city, d), city, d)
	return list("show", ctx, title, sub)
}

// tidyPlace drops what tapers write when they don't know a place ("Unknown, Unknown") and a
// trailing full stop ("Mobile, AL.").
func tidyPlace(s string) string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimRight(strings.TrimSpace(p), ".")
		if p != "" && !strings.EqualFold(p, "unknown") && p != "?" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}
