package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"q2-lma/internal/ia"
	"q2-lma/internal/q2spot"
	"q2-lma/internal/stream"
)

// Each screen is one Q2SPOT/1 level (q2-pod patch/addon.c): a list row opens the level its URI
// names with its context as the argument, an action row sends its URI and context and toasts the
// reply's message. Contexts are space-separated identifiers, dates and numbers, never display
// text, and stay far under addon.c's 400-byte argument buffer. Long lists are sent 50 rows a page
// with the message "pages", so the firmware loads them whole (500 rows at most).

func list(op, ctx, title, sub string) q2spot.Row {
	return q2spot.Row{Kind: q2spot.KindList, URI: op, Context: ctx, Title: title, Subtitle: sub}
}

func act(op, ctx, title, sub string) q2spot.Row {
	return q2spot.Row{Kind: q2spot.KindAction, URI: op, Context: ctx, Title: title, Subtitle: sub}
}

func info(title, sub string) q2spot.Row {
	return q2spot.Row{Kind: q2spot.KindInfo, Title: title, Subtitle: sub}
}

// join puts the non-empty parts together with " · ".
func join(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

func gb(n int64) string {
	if n < 1<<30 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

func clock(sec int) string {
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// page is rows from cursor (an offset) on, 50 at most, with the next cursor and "pages" when more
// follow.
func page(rows []q2spot.Row, cursor string) q2spot.Reply {
	from, _ := strconv.Atoi(cursor)
	if from < 0 || from > len(rows) {
		from = 0
	}
	to := min(from+q2spot.MaxRows, len(rows))
	r := q2spot.Reply{Rows: rows[from:to]}
	if to < len(rows) {
		r.Cursor, r.Message = strconv.Itoa(to), q2spot.Pages
	}
	return r
}

// loading is a screen whose data is still on its way: the firmware asks again every 2 s.
func loading(what string) q2spot.Reply {
	return q2spot.Reply{Message: q2spot.Refresh, Rows: []q2spot.Row{info("Loading…", what)}}
}

// fail is a screen that couldn't load: the firmware shows the message with Retry.
func (a *app) fail(what string, err error) q2spot.Reply {
	a.log.Printf("%s: %v", what, err)
	var th *ia.ErrThrottled
	var ne net.Error
	switch {
	case errors.As(err, &th):
		return q2spot.Fail(q2spot.Throttled, th.Error())
	case errors.As(err, &ne), errors.Is(err, context.DeadlineExceeded):
		return q2spot.Fail(q2spot.Offline, "Can't reach archive.org. Check Wi-Fi")
	}
	return q2spot.Fail(q2spot.Error, "Couldn't load "+what)
}

// fetch runs f through the loader, answering within answerIn: its value, or a ready reply (loading
// or failed) when there isn't one.
func fetch[T any](a *app, key, what string, f func(context.Context) (T, error)) (T, *q2spot.Reply) {
	v, err, ok := a.loads.get(key, answerIn*3/4, func(ctx context.Context) (any, error) { return f(ctx) })
	var zero T
	if !ok {
		r := loading(what)
		return zero, &r
	}
	if err != nil {
		r := a.fail(what, err)
		return zero, &r
	}
	return v.(T), nil
}

// device answers a request with every row fitted to its labels (rows.go).
func (a *app) device(req q2spot.Request) q2spot.Reply {
	r := a.answer(req)
	fitRows(&r)
	return r
}

func (a *app) answer(req q2spot.Request) q2spot.Reply {
	a.mu.Lock()
	a.lastReq = time.Now()
	a.mu.Unlock()
	ctx := req.Arg(0)
	switch req.Op {
	case "menu":
		return a.menu()
	case "mine":
		return a.myArtists(req.Arg(0))
	case "letters":
		return a.letters()
	case "letter":
		return a.letter(ctx, req.Arg(1))
	case "top":
		return a.top(req.Arg(0))
	case "search":
		return a.search(ctx, req.Arg(1))
	case "artist":
		return a.artist(ctx, req.Arg(1))
	case "fav":
		return a.fav(ctx)
	case "year":
		return a.year(ctx, req.Arg(1))
	case "show":
		return a.show(ctx, req.Arg(1))
	case "versions":
		return a.versions(ctx)
	case "pick":
		return a.pick(ctx)
	case "play":
		return a.play(ctx)
	case "shuffle":
		return a.shuffle(ctx)
	case "settings":
		return a.settingsPage()
	case "speedtest":
		go a.speedTest()
		return q2spot.Reply{Message: "Testing download speed: about 10 s"}
	case "cachesize", "format", "clear":
		return a.settingAction(req.Op)
	case "details":
		return a.details()
	case "stop":
		a.log.Printf("turned off from the Q2")
		go func() { time.Sleep(500 * time.Millisecond); a.shutdown(); os.Exit(0) }()
		return q2spot.Reply{Message: "Live Music Archive turned off"}
	}
	a.log.Printf("unknown op %q %q", req.Op, req.Args)
	return q2spot.Fail(q2spot.Unsupported, "Unknown request")
}

// menu is the app's first level. It asks to refresh only while a show streams, so its last rows
// follow the stream; rows that come and go are last, so the ones above keep their places.
func (a *app) menu() q2spot.Reply {
	n := len(a.mine.Entries())
	mineSub, from := plural(n, "artist"), "From My Artists"
	if n == 0 {
		mineSub, from = "Hold Play/Pause on any artist to add it", "From Top Artists"
	}
	s := a.settings()
	rows := []q2spot.Row{
		list("mine", "", "My Artists", mineSub),
		list("letters", "", "All Artists", "A to Z"),
		list("top", "", "Top Artists", "Most played on the archive"),
		{Kind: q2spot.KindSearch, Title: "Search Artists", Subtitle: "By name"},
		act("shuffle", "", "Shuffle a Show", from),
		list("settings", "", "Settings", "Cache: "+gb(a.cacheSize())+" of "+gb(int64(s.CacheMB)<<20)),
	}
	active, line, serr := a.eng.Status()
	off := "It also turns off after 15 minutes unused"
	if active {
		off = "It stays on while a show streams"
	}
	rows = append(rows, act("stop", "", "Turn Off Live Music Archive", off))
	if active {
		rows = append(rows, info(line, ""))
	}
	if msg := a.problemLine(serr); msg != "" {
		rows = append(rows, info(msg, ""), list("details", "", "Details", ""))
	}
	r := q2spot.Reply{Rows: rows}
	if active {
		r.Message = q2spot.Refresh
	}
	return r
}

// problemLine is the newest of the stream's error and the last failed play or shuffle, while it
// is under an hour old.
func (a *app) problemLine(serr *stream.Err) string {
	a.mu.Lock()
	msg, at := a.problem, a.problemAt
	a.mu.Unlock()
	if serr != nil && serr.At.After(at) {
		msg, at = serr.Error(), serr.At
	}
	if msg == "" || time.Since(at) > time.Hour {
		return ""
	}
	return msg
}

func (a *app) setProblem(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.log.Printf("problem: %s", msg)
	a.mu.Lock()
	a.problem, a.problemAt = msg, time.Now()
	a.mu.Unlock()
}

func (a *app) details() q2spot.Reply {
	_, _, serr := a.eng.Status()
	a.mu.Lock()
	msg, at := a.problem, a.problemAt
	a.mu.Unlock()
	var rows []q2spot.Row
	if serr != nil {
		rows = append(rows, info(serr.Error(), serr.At.Local().Format("15:04")), info(fmt.Sprint(serr.Err), ""))
	}
	if msg != "" {
		rows = append(rows, info(msg, at.Local().Format("15:04")))
	}
	rows = append(rows, info("More in .lma/log.txt on the card", ""))
	return q2spot.Reply{Rows: rows}
}

// artistRow opens an artist; Play/Pause held on it adds or removes it from My Artists.
func (a *app) artistRow(x ia.Artist) q2spot.Row {
	a.names.Store(x.ID, string(x.Name)) // what a hold on this row adds to My Artists
	sub := ""
	if a.mine.Has(x.ID) {
		sub = "In My Artists"
	}
	return q2spot.Row{Kind: q2spot.KindList, URI: "artist", Context: x.ID, Title: string(x.Name), Subtitle: sub,
		Position: "fav"}
}

func (a *app) myArtists(cursor string) q2spot.Reply {
	es := a.mine.Entries()
	if len(es) == 0 {
		return q2spot.Reply{Rows: []q2spot.Row{info("No artists yet", "Hold Play/Pause on any artist to add it")}}
	}
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	// archive.org's names, and which identifiers it doesn't know; offline, the file's names
	known, err, ok := a.loads.get("names:"+strings.Join(ids, ","), answerIn/2, func(ctx context.Context) (any, error) {
		return a.ia.Names(ctx, ids)
	})
	names, _ := known.(map[string]ia.Artist)
	var rows []q2spot.Row
	for _, e := range es {
		x, found := names[e.ID]
		switch {
		case found:
			rows = append(rows, a.artistRow(x))
		case ok && err == nil:
			rows = append(rows, info("Not found on the archive: "+e.ID, "Fix or remove it in .lma/my-artists.txt"))
		default:
			name := e.Name
			if name == "" {
				name = e.ID
			}
			rows = append(rows, a.artistRow(ia.Artist{ID: e.ID, Name: ia.Text(name)}))
		}
	}
	return page(rows, cursor)
}

func (a *app) letters() q2spot.Reply {
	rows := []q2spot.Row{list("letter", "#", "0-9", "")}
	for c := 'A'; c <= 'Z'; c++ {
		rows = append(rows, list("letter", string(c), string(c), ""))
	}
	return q2spot.Reply{Rows: rows}
}

func (a *app) artistList(as []ia.Artist, cursor, empty string) q2spot.Reply {
	if len(as) == 0 {
		return q2spot.Reply{Rows: []q2spot.Row{info(empty, "")}}
	}
	rows := make([]q2spot.Row, len(as))
	for i, x := range as {
		rows[i] = a.artistRow(x)
	}
	return page(rows, cursor)
}

func (a *app) letter(l, cursor string) q2spot.Reply {
	if len(l) != 1 || !(l[0] == '#' || l[0] >= 'A' && l[0] <= 'Z') {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	c := strings.ToLower(l)[0]
	as, r := fetch(a, "letter:"+l, "artists", func(ctx context.Context) ([]ia.Artist, error) { return a.ia.Letter(ctx, c) })
	if r != nil {
		return *r
	}
	return a.artistList(as, cursor, "No artists")
}

func (a *app) top(cursor string) q2spot.Reply {
	as, r := fetch(a, "top", "artists", a.ia.Top)
	if r != nil {
		return *r
	}
	return a.artistList(as, cursor, "No artists")
}

func (a *app) search(term, cursor string) q2spot.Reply {
	as, r := fetch(a, "search:"+strings.ToLower(term), "results", func(ctx context.Context) ([]ia.Artist, error) {
		return a.ia.SearchArtists(ctx, term)
	})
	if r != nil {
		return *r
	}
	return a.artistList(as, cursor, "Nothing found")
}

// name is an artist's name: as a list sent it, else archive.org's, else My Artists', else its
// identifier.
func (a *app) name(ctx context.Context, id string) string {
	if n, ok := a.names.Load(id); ok && n.(string) != id {
		return n.(string)
	}
	if m, err := a.ia.Names(ctx, []string{id}); err == nil {
		if x, ok := m[id]; ok {
			return string(x.Name)
		}
	}
	for _, e := range a.mine.Entries() {
		if e.ID == id && e.Name != "" {
			return e.Name
		}
	}
	return id
}

// fav is Play/Pause held on an artist, or the artist page's row: in or out of My Artists.
func (a *app) fav(id string) q2spot.Reply {
	if !ia.ValidID(id) {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	name, _ := a.names.Load(id) // the row held was in a list just sent, so its name is known
	n, _ := name.(string)
	if n == "" {
		n = id
	}
	added, err := a.mine.Toggle(id, n)
	if added && n == id { // archive.org's name for the file, behind the reply
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if real := a.name(ctx, id); real != id {
				a.mine.Name(id, real)
			}
		}()
	}
	switch {
	case err != nil:
		a.log.Printf("my artists: %v", err)
		return q2spot.Reply{Message: "Couldn't save My Artists"}
	case added:
		return q2spot.Reply{Message: "Added to My Artists"}
	}
	return q2spot.Reply{Message: "Removed from My Artists"}
}

func (a *app) artist(id, cursor string) q2spot.Reply {
	if !ia.ValidID(id) {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	years, r := fetch(a, "years:"+id, "years", func(ctx context.Context) ([]ia.Year, error) { return a.ia.Years(ctx, id) })
	if r != nil {
		return *r
	}
	favTitle := "Add to My Artists"
	if a.mine.Has(id) {
		favTitle = "Remove from My Artists"
	}
	rows := []q2spot.Row{act("shuffle", id, "Shuffle a Show", "From this artist"), act("fav", id, favTitle, "")}
	if len(years) == 0 {
		rows = append(rows, info("No shows on the archive", ""))
	}
	for _, y := range years {
		rows = append(rows, list("year", fmt.Sprintf("%s %d", id, y.Year), strconv.Itoa(y.Year), plural(y.Count, "recording")))
	}
	return page(rows, cursor)
}

// fields splits a row's context into n parts, checking the identifiers.
func fields(ctx string, n int) ([]string, bool) {
	f := strings.Fields(ctx)
	return f, len(f) == n && ia.ValidID(f[0])
}

func (a *app) recordings(id string, year int) ([]ia.Recording, *q2spot.Reply) {
	return fetch(a, fmt.Sprintf("rec:%s:%d", id, year), "shows", func(ctx context.Context) ([]ia.Recording, error) {
		return a.ia.Recordings(ctx, id, year)
	})
}

// chosen is the recording of show s to play: the one picked in More Versions, else the best.
func (a *app) chosen(id string, s ia.Show) ia.Recording {
	if pick := a.settings().Picks[id+" "+s.Date]; pick != "" {
		for _, v := range s.Versions {
			if v.ID == pick {
				return v
			}
		}
	}
	return s.Versions[0]
}

func describe(r ia.Recording) string {
	stars := ""
	if r.Rating > 0 {
		stars = fmt.Sprintf("%.1f★", float64(r.Rating))
	}
	return join(ia.ClassOf(r).Label(), stars)
}

func (a *app) year(ctx, cursor string) q2spot.Reply {
	f, ok := fields(ctx, 2)
	if !ok {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	y, err := strconv.Atoi(f[1])
	if err != nil {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	recs, r := a.recordings(f[0], y)
	if r != nil {
		return *r
	}
	shows := ia.Shows(recs)
	if len(shows) == 0 {
		return q2spot.Reply{Rows: []q2spot.Row{info("No shows yet", "")}}
	}
	rows := make([]q2spot.Row, len(shows))
	for i, s := range shows {
		rows[i] = showRow(f[0]+" "+s.Date, s, a.chosen(f[0], s))
	}
	return page(rows, cursor)
}

// findShow is date's show among an artist's recordings of its year.
func (a *app) findShow(id, date string) (ia.Show, *q2spot.Reply) {
	if len(date) != 10 {
		r := q2spot.Fail(q2spot.Bad, "Bad request")
		return ia.Show{}, &r
	}
	y, _ := strconv.Atoi(date[:4])
	recs, r := a.recordings(id, y)
	if r != nil {
		return ia.Show{}, r
	}
	for _, s := range ia.Shows(recs) {
		if s.Date == date {
			return s, nil
		}
	}
	nf := q2spot.Fail(q2spot.Unavailable, "This show is no longer on the archive")
	return ia.Show{}, &nf
}

func (a *app) songs(rec string) ([]ia.Song, []ia.File, *q2spot.Reply) {
	files, r := fetch(a, "files:"+rec, "songs", func(ctx context.Context) ([]ia.File, error) { return a.ia.Files(ctx, rec) })
	if r != nil {
		return nil, nil, r
	}
	return ia.Songs(files, a.settings().Prefer), files, nil
}

func (a *app) show(ctx, cursor string) q2spot.Reply {
	f, ok := fields(ctx, 2)
	if !ok {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	s, r := a.findShow(f[0], f[1])
	if r != nil {
		return *r
	}
	rec := a.chosen(f[0], s)
	songs, _, r := a.songs(rec.ID)
	if r != nil {
		return *r
	}
	if len(songs) > 0 && cursor == "" {
		a.prefetch(f[0], f[1], rec.ID) // its first song, so Play Show often finds it here
	}
	status, live := a.showStatus(rec.ID)
	rows := status
	base := f[0] + " " + f[1] + " " + rec.ID
	if len(songs) == 0 {
		rows = append(rows, info("No MP3 or Ogg files in this recording", "Choose another in More Versions"))
	} else {
		total := 0
		for _, sg := range songs {
			total += sg.Seconds
		}
		rows = append(rows, act("play", base+" 0", "Play Show", join(describe(rec), plural(len(songs), "song"), clock(total))))
	}
	if len(s.Versions) > 1 {
		rows = append(rows, list("versions", ctx, "More Versions", plural(len(s.Versions)-1, "other recording")))
	}
	for i, sg := range songs {
		sub := clock(sg.Seconds)
		if _, err := os.Stat(stream.SongPath(a.eng.Cache, rec.ID, sg)); err == nil {
			sub = join(sub, "On the Q2")
		}
		rows = append(rows, act("play", fmt.Sprintf("%s %d", base, i), fmt.Sprintf("%02d %s", sg.Track, sg.Title), sub))
	}
	// While it streams the page refreshes, so its bar moves; the firmware never pages a refreshing
	// list, so a show too long for one page gives up the refresh rather than its songs.
	if live && len(rows) <= q2spot.MaxRows && cursor == "" {
		return q2spot.Reply{Message: q2spot.Refresh, Rows: rows}
	}
	return page(rows, cursor)
}

func (a *app) versions(ctx string) q2spot.Reply {
	f, ok := fields(ctx, 2)
	if !ok {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	s, r := a.findShow(f[0], f[1])
	if r != nil {
		return *r
	}
	cur := a.chosen(f[0], s).ID
	var rows []q2spot.Row
	for _, v := range s.Versions {
		title := strings.TrimSpace(string(v.Source))
		if title == "" {
			title = v.ID
		}
		mark := ""
		if v.ID == cur {
			mark = "Playing this one"
		}
		dl := ""
		if v.Downloads > 0 {
			dl = plural(int(v.Downloads), "download")
		}
		rows = append(rows, act("pick", ctx+" "+v.ID, title, join(mark, describe(v), dl)))
	}
	return page(rows, "")
}

// pick chooses a recording for its date; Back shows its songs.
func (a *app) pick(ctx string) q2spot.Reply {
	f, ok := fields(ctx, 3)
	if !ok || !ia.ValidID(f[2]) {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	a.change(func(s *settings) { s.Picks[f[0]+" "+f[1]] = f[2] })
	return q2spot.Reply{Message: "This recording will play"}
}

// session builds a recording's stream from song from: its songs, names and art.
func (a *app) session(ctx context.Context, id, date, rec string, from int) (stream.Session, error) {
	files, err := a.ia.Files(ctx, rec)
	if err != nil {
		return stream.Session{}, err
	}
	songs := ia.Songs(files, a.settings().Prefer)
	if from < 0 || from >= len(songs) {
		return stream.Session{}, errors.New("no MP3 or Ogg files in this recording")
	}
	y, _ := strconv.Atoi(date[:4])
	album := date
	if recs, err := a.ia.Recordings(ctx, id, y); err == nil {
		for _, r := range recs {
			if p := ia.Place(r); r.ID == rec && p != "" {
				album = date + " " + p // "1977-05-08 Barton Hall, Cornell University"
			}
		}
	}
	return stream.Session{ID: rec, Artist: a.name(ctx, id), Album: album, Date: date, Songs: songs, From: from,
		ArtFile: ia.Art(files)}, nil
}

// play starts a recording from a song. It answers at once and starts behind the reply; the
// firmware opens Now Playing when the first song lands.
func (a *app) play(ctx string) q2spot.Reply {
	f, ok := fields(ctx, 4)
	if !ok || len(f[1]) != 10 || !ia.ValidID(f[2]) {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	n, err := strconv.Atoi(f[3])
	if err != nil {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	// A second press of what is starting or playing leaves it be: starting again would throw away
	// the download under way (seen on the Q2: a show pressed three times in 20 s, never playing).
	key := f[2] + " " + f[3]
	a.mu.Lock()
	starting := a.starting == key
	if !starting {
		a.starting = key
	}
	a.mu.Unlock()
	if id, from := a.eng.Playing(); starting || id == f[2] && from == n {
		return q2spot.Reply{Message: "Already started: it plays once its first song is on the Q2"}
	}
	msg := "Loading…"
	c, cancel := context.WithTimeout(context.Background(), answerIn/2)
	files, ferr := a.ia.Files(c, f[2]) // cached by the show page
	cancel()
	if songs := ia.Songs(files, a.settings().Prefer); ferr == nil && n >= 0 && n < len(songs) {
		sg := songs[n]
		if _, err := os.Stat(stream.SongPath(a.eng.Cache, f[2], sg)); err == nil {
			msg = "Playing " + sg.Title
		} else {
			rate := a.progress().rate
			if rate <= 0 {
				rate = 1.5e6 // measured on the Q2 over home Wi-Fi
			}
			msg = fmt.Sprintf("Loading %s: %s, about %d s", sg.Title, gb(sg.Size), int(float64(sg.Size)/rate)+2)
		}
	}
	a.mu.Lock()
	a.failedRec = ""
	a.mu.Unlock()
	a.stopPrefetch(f[2]) // another show's: this one's songs first
	go func() {
		defer func() {
			a.mu.Lock()
			if a.starting == key {
				a.starting = ""
			}
			a.mu.Unlock()
		}()
		c, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		s, err := a.session(c, f[0], f[1], f[2], n)
		if err == nil {
			err = a.eng.Start(s)
		}
		if err != nil {
			a.setProblem("Couldn't play %s: %v", f[1], err)
			a.mu.Lock()
			a.failedRec = f[2]
			a.mu.Unlock()
		}
	}()
	return q2spot.Reply{Message: msg}
}

// showStatus is the show page's live row while its recording streams: the song downloading (a
// progress bar where the firmware has them), else what the stream does, and any failure; live
// when the page should refresh.
func (a *app) showStatus(rec string) (rows []q2spot.Row, live bool) {
	a.mu.Lock()
	failed, problem := a.failedRec == rec, a.problem
	a.mu.Unlock()
	if failed {
		return []q2spot.Row{info(problem, "Press Play Show to try again")}, false
	}
	if a.eng.Current() != rec {
		return nil, false
	}
	p := a.progress()
	_, line, serr := a.eng.Status()
	switch {
	case p.active && p.id == rec:
		pml := 0
		if p.total > 0 {
			pml = int(p.done * 1000 / p.total)
		}
		title, sub := "Loading "+p.title, gb(p.done)+" of "+gb(p.total)
		if a.capsHas("progress") {
			rows = append(rows, q2spot.Row{Kind: q2spot.KindProgress, Title: title, Subtitle: sub, Position: strconv.Itoa(pml)})
		} else {
			rows = append(rows, info(title, sub))
		}
	case line != "":
		rows = append(rows, info(line, ""))
	}
	if serr != nil && time.Since(serr.At) < 10*time.Minute {
		rows = append(rows, info(serr.Error(), ""))
	}
	return rows, true
}

// shuffle plays a random show from the start: from one artist, else from My Artists, else from Top
// Artists. A year is drawn by its number of recordings, then a date, then its chosen recording.
func (a *app) shuffle(id string) q2spot.Reply {
	if id != "" && !ia.ValidID(id) {
		return q2spot.Fail(q2spot.Bad, "Bad request")
	}
	go func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := a.shuffleNow(c, id); err != nil {
			a.setProblem("Shuffle: %v", err)
		}
	}()
	return q2spot.Reply{Message: "Finding a show…"}
}

func (a *app) shuffleNow(ctx context.Context, id string) error {
	if id == "" {
		var ids []string
		for _, e := range a.mine.Entries() {
			if ia.ValidID(e.ID) {
				ids = append(ids, e.ID)
			}
		}
		if len(ids) == 0 {
			top, err := a.ia.Top(ctx)
			if err != nil {
				return err
			}
			for _, x := range top[:min(len(top), 100)] {
				ids = append(ids, x.ID)
			}
		}
		if len(ids) == 0 {
			return errors.New("no artists")
		}
		id = ids[rand.IntN(len(ids))]
	}
	years, err := a.ia.Years(ctx, id)
	if err != nil {
		return err
	}
	total := 0
	for _, y := range years {
		total += y.Count
	}
	if total == 0 {
		return errors.New("no shows for " + id)
	}
	k := rand.IntN(total)
	year := years[0].Year
	for _, y := range years {
		if k < y.Count {
			year = y.Year
			break
		}
		k -= y.Count
	}
	recs, err := a.ia.Recordings(ctx, id, year)
	if err != nil {
		return err
	}
	shows := ia.Shows(recs)
	if len(shows) == 0 {
		return errors.New("no shows")
	}
	// a few tries, for dates whose recordings have nothing to stream
	for try := 0; try < 4; try++ {
		s := shows[rand.IntN(len(shows))]
		sess, err := a.session(ctx, id, s.Date, a.chosen(id, s).ID, 0)
		if err != nil {
			continue
		}
		a.log.Printf("shuffle: %s %s", id, s.Date)
		return a.eng.Start(sess)
	}
	return errors.New("no playable show found")
}

func (a *app) settingsPage() q2spot.Reply {
	s := a.settings()
	prefer, why := "Prefer: Ogg", "Smaller files, so songs start sooner"
	if s.Prefer == "mp3" {
		prefer, why = "Prefer: MP3", "Larger files; for shows without Ogg it's MP3 anyway"
	}
	clear := "Keeps the show playing"
	a.mu.Lock()
	if time.Since(a.confirmAt) < 10*time.Second {
		clear = "Press again to clear it"
	}
	a.mu.Unlock()
	free := ""
	if n, _, err := diskSpace(a.card); err == nil {
		free = "Card: " + gb(int64(n)) + " free"
	}
	a.mu.Lock()
	speed := a.speedLine
	a.mu.Unlock()
	if speed == "" {
		speed = "One connection against three at once"
	}
	v := strings.TrimPrefix(version, "q2-v")
	return q2spot.Reply{Rows: []q2spot.Row{
		act("cachesize", "", "Cache Size: "+gb(int64(s.CacheMB)<<20), gb(a.cacheSize())+" used"),
		act("format", "", prefer, why),
		act("clear", "", "Clear Cache", clear),
		act("speedtest", "", "Test Download Speed", speed),
		info(free, ""),
		info("Version "+v, ""),
	}}
}

func (a *app) settingAction(op string) q2spot.Reply {
	switch op {
	case "cachesize":
		var mb int
		a.change(func(s *settings) {
			i := 0
			for k, v := range cacheSteps {
				if v == s.CacheMB {
					i = k
				}
			}
			s.CacheMB = cacheSteps[(i+1)%len(cacheSteps)]
			mb = s.CacheMB
		})
		go func() { stream.Evict(a.eng.Cache, int64(mb)<<20, a.playingID(), a.log.Printf); a.forgetSize() }()
		return q2spot.Reply{Message: "Cache Size: " + gb(int64(mb)<<20)}
	case "format":
		var p string
		a.change(func(s *settings) {
			if s.Prefer == "ogg" {
				s.Prefer = "mp3"
			} else {
				s.Prefer = "ogg"
			}
			p = strings.ToUpper(s.Prefer[:1]) + s.Prefer[1:]
			if p == "Mp3" {
				p = "MP3"
			}
		})
		return q2spot.Reply{Message: "Prefer: " + p}
	}
	a.mu.Lock()
	armed := time.Since(a.confirmAt) < 10*time.Second
	a.confirmAt = time.Now()
	if armed {
		a.confirmAt = time.Time{}
	}
	a.mu.Unlock()
	if !armed {
		return q2spot.Reply{Message: "Press again to clear " + gb(a.cacheSize())}
	}
	stream.Evict(a.eng.Cache, 0, a.playingID(), a.log.Printf)
	a.ia.Cache.Clear()
	a.forgetSize()
	return q2spot.Reply{Message: "Cache cleared"}
}

// playingID is the recording the stream plays, kept from eviction; "" when none.
func (a *app) playingID() string { return a.eng.Current() }
