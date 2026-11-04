package main

import (
	"fmt"
	"strings"
	"time"

	"q2-lma/internal/fit"
	"q2-lma/internal/ia"
	"q2-lma/internal/q2spot"
	"q2-lma/internal/stream"
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

// wrapped is an inert row whose text runs over its title and subtitle, then detail if there's room.
func wrapped(text, detail string) q2spot.Row {
	first, second := fit.Wrap(text, titlePx, rowWide-spare, subtitlePx, rowWide-spare)
	return info(first, join(second, detail))
}

// nowState is how the stream's song stands: "1 of 20" once it plays, else how its download goes.
func (a *app) nowState(n stream.Now) string {
	if n.Ready {
		return fmt.Sprintf("%d of %d", n.Index+1, n.Total)
	}
	if p := a.progress(); p.active && p.id == n.ID && p.index == n.Index && p.total > 0 {
		return fmt.Sprintf("Loading %d%%", p.done*100/p.total)
	}
	if strings.HasPrefix(n.State, "Waiting") {
		return "Waiting"
	}
	return "Loading"
}

// nowRow is the row a show's page opens with while it streams: the song, then how it stands.
func (a *app) nowRow(rec string) (q2spot.Row, bool) {
	n, ok := a.eng.NowPlaying()
	if !ok || n.ID != rec {
		return q2spot.Row{}, false
	}
	return wrapped(n.Title, a.nowState(n)), true
}

// nowMenuRow is the row the main menu opens with while a show streams: one row, so the menu keeps
// its room.
//
//	Grateful Dead
//	May 8, 1977 · Barton Hall · 1 of 20
func (a *app) nowMenuRow() (q2spot.Row, bool) {
	n, ok := a.eng.NowPlaying()
	if !ok {
		return q2spot.Row{}, false
	}
	return info(n.Artist, showLine(n.Date, n.Album, a.nowState(n))), true
}

// showLine is the date and venue and state on one line that fits: the date and state always, the
// whole venue if there's room, else its first part, cut if it must be. album is the session's,
// the date (YYYY-MM-DD) then the place.
func showLine(day, album, state string) string {
	date := day
	if t, err := time.Parse("2006-01-02", day); err == nil {
		date = t.Format("Jan 2, 2006")
	}
	w := rowWide - spare
	venue := strings.TrimSpace(strings.TrimPrefix(album, day))
	short, _, _ := strings.Cut(venue, ",")
	short = strings.TrimSpace(short)
	if venue == "" {
		return join(date, state)
	}
	line := func(v string) string { return join(date, v, state) }
	if fit.Fits(line(venue), subtitlePx, w) {
		return line(venue)
	}
	if fit.Fits(line(short), subtitlePx, w) {
		return line(short)
	}
	// what the venue may take: all but the date, the state, and its two separators
	room := w - fit.Width(join(date, state)+" · ", subtitlePx)
	if room < fit.Width("Xxxxxxx…", subtitlePx) { // too little to read: the date and state alone
		return join(date, state)
	}
	return line(fit.Cut(short, subtitlePx, room))
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
