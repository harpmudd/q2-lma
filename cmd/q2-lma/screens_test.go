package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"q2-lma/internal/fit"
	"q2-lma/internal/ia"
	"q2-lma/internal/q2spot"
)

// fakeArchive answers as archive.org does, from internal/ia's recorded responses; every song is
// the start of a real Ogg (internal/tag's fixture).
// downloads counts the fake archive's song downloads, by path; gate, when set, holds them.
var (
	downloads sync.Map
	gate      atomic.Pointer[chan struct{}]
)

func downloaded(file string) int32 {
	n, ok := downloads.Load("/download/gd77-05-08.sbd.hicks.4982.sbeok.shnf/" + file)
	if !ok {
		return 0
	}
	return n.(*atomic.Int32).Load()
}

func fakeArchive(t *testing.T) *httptest.Server {
	ia := filepath.Join("..", "..", "internal", "ia", "testdata")
	ogg := filepath.Join("..", "..", "internal", "tag", "testdata", "head.ogg")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		switch {
		case r.URL.Path == "/advancedsearch.php" && strings.Contains(q, "collection:GratefulDead AND year:1977"):
			http.ServeFile(w, r, filepath.Join(ia, "shows-GratefulDead-1977.json"))
		case r.URL.Path == "/advancedsearch.php" && strings.Contains(q, "identifier:(GratefulDead"):
			fmt.Fprint(w, `{"response":{"numFound":1,"docs":[{"identifier":"GratefulDead","title":"Grateful Dead"}]}}`)
		case r.URL.Path == "/advancedsearch.php":
			http.ServeFile(w, r, filepath.Join(ia, "letter-g.json"))
		case r.URL.Path == "/services/search/v1/scrape":
			fmt.Fprint(w, `{"items":[{"year":1977},{"year":1977},{"year":"1972"}],"count":3}`)
		case strings.HasPrefix(r.URL.Path, "/metadata/") && strings.HasSuffix(r.URL.Path, "/files"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/metadata/"), "/files")
			http.ServeFile(w, r, filepath.Join(ia, "files-"+id+".json"))
		case strings.HasPrefix(r.URL.Path, "/download/") && strings.HasSuffix(r.URL.Path, ".ogg"):
			if g := gate.Load(); g != nil {
				<-*g
			}
			n, _ := downloads.LoadOrStore(r.URL.Path, new(atomic.Int32))
			n.(*atomic.Int32).Add(1)
			http.ServeFile(w, r, ogg)
		default:
			t.Logf("fake archive: no answer for %s", r.URL)
			http.NotFound(w, r)
		}
	}))
}

func testApp(t *testing.T) (*app, string) {
	srv := fakeArchive(t)
	t.Cleanup(srv.Close)
	root := t.TempDir()
	card, tmp := filepath.Join(root, "card"), filepath.Join(root, "tmp")
	os.MkdirAll(tmp, 0o755)
	os.WriteFile(filepath.Join(tmp, "q2-addon.caps"), []byte("playlist progress pages library\n"), 0o644)
	a := newApp(filepath.ToSlash(filepath.Join(card, ".lma")), card, filepath.ToSlash(tmp), srv.URL, srv.Client())
	a.eng.Poll, a.eng.Gap = 20*time.Millisecond, 0
	t.Cleanup(a.eng.Stop)
	return a, tmp
}

// ask sends a request as the firmware would and checks the reply against Q2SPOT/1 and addon.c's
// limits; a "Loading…" reply is asked again, as the firmware's refresh does.
func ask(t *testing.T, a *app, op string, args ...string) q2spot.Reply {
	t.Helper()
	for try := 0; ; try++ {
		r := a.device(q2spot.Request{ID: "1", Op: op, Args: args})
		b, kept := r.Encode("1")
		if kept != len(r.Rows) || len(b) > q2spot.MaxReply {
			t.Fatalf("%s: %d of %d rows fit", op, kept, len(r.Rows))
		}
		// a toast scrolls what doesn't fit, so an action's is short; a list's message is an instruction
		if r.Status == q2spot.OK || r.Status == "" {
			if r.Message != q2spot.Refresh && r.Message != q2spot.Pages && !fit.Fits(r.Message, titlePx, rowWide-spare) {
				t.Errorf("%s: toast %q would scroll", op, r.Message)
			}
		}
		for _, row := range r.Rows {
			if len(row.URI) > q2spot.MaxOp {
				t.Errorf("%s: op %q too long for the firmware", op, row.URI)
			}
			if !q2spot.ContextFits(row.Context) {
				t.Errorf("%s: context %q too long", op, row.Context)
			}
			w := rowWide
			if chevron(row) {
				w = rowChevron
			}
			if row.Kind != q2spot.KindProgress && (!fit.Fits(row.Title, titlePx, w) || !fit.Fits(row.Subtitle, subtitlePx, w)) {
				t.Errorf("%s: row %q / %q would scroll", op, row.Title, row.Subtitle)
			}
		}
		if r.Message == q2spot.Refresh && len(r.Rows) == 1 && r.Rows[0].Title == "Loading…" && try < 50 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return r
	}
}

// all is a list with its following pages appended, as the firmware loads a "pages" list.
func all(t *testing.T, a *app, op string, args ...string) q2spot.Reply {
	t.Helper()
	r := ask(t, a, op, args...)
	for p := r; p.Cursor != ""; {
		if p.Message != q2spot.Pages {
			t.Fatalf("%s: a cursor without \"pages\"", op)
		}
		p = ask(t, a, op, append(append([]string{}, args...), p.Cursor)...)
		r.Rows = append(r.Rows, p.Rows...)
	}
	return r
}

func find(t *testing.T, r q2spot.Reply, title string) q2spot.Row {
	t.Helper()
	for _, row := range r.Rows {
		if strings.HasPrefix(row.Title, title) {
			return row
		}
	}
	t.Fatalf("no %q in %+v", title, r.Rows)
	return q2spot.Row{}
}

func TestBrowseToPlay(t *testing.T) {
	a, tmp := testApp(t)
	menu := ask(t, a, "menu")
	find(t, menu, "My Artists")
	find(t, menu, "Turn Off Live Music Archive")

	if r := ask(t, a, "mine"); r.Rows[0].Title != "No artists yet" {
		t.Errorf("empty My Artists: %+v", r.Rows)
	}
	letters := ask(t, a, "letters")
	if len(letters.Rows) != 27 {
		t.Errorf("%d letters", len(letters.Rows))
	}
	g := ask(t, a, "letter", "G")
	if g.Message != q2spot.Pages || g.Cursor != "50" || len(g.Rows) != 50 {
		t.Errorf("letter G: message %q cursor %q rows %d", g.Message, g.Cursor, len(g.Rows))
	}
	dead := find(t, all(t, a, "letter", "G"), "Grateful Dead")
	if dead.Position != "fav" || dead.Context != "GratefulDead" {
		t.Errorf("artist row %+v", dead)
	}

	// hold Play/Pause on it: into My Artists, and out again
	if r := ask(t, a, "fav", "GratefulDead"); r.Message != "Added to My Artists" {
		t.Errorf("fav: %q", r.Message)
	}
	if r := ask(t, a, "mine"); r.Rows[0].Title != "Grateful Dead" {
		t.Errorf("My Artists: %+v", r.Rows)
	}
	mineFile, _ := os.ReadFile(filepath.Join(a.dir, "my-artists.txt"))
	if !strings.HasSuffix(string(mineFile), "GratefulDead\tGrateful Dead\n") {
		t.Errorf("my-artists.txt:\n%s", mineFile)
	}

	artist := ask(t, a, "artist", "GratefulDead")
	y77 := find(t, artist, "1977")
	if y77.Subtitle != "2 recordings" || y77.Context != "GratefulDead 1977" {
		t.Errorf("year row %+v", y77)
	}
	cornell := find(t, all(t, a, "year", y77.Context), "May 8 · Barton Hall")
	if cornell.Subtitle != "Ithaca, NY · SBD 4.8★ · 23 versions" {
		t.Errorf("Cornell row %+v", cornell)
	}
	show := ask(t, a, "show", cornell.Context)
	play := find(t, show, "Play Show")
	if !strings.Contains(play.Context, "gd77-05-08.sbd.hicks.4982.sbeok.shnf 0") {
		t.Errorf("Play Show %+v", play)
	}
	find(t, show, "More Versions")
	find(t, show, "01 Minglewood Blues")

	// another version picked: the show then plays it
	versions := ask(t, a, "versions", cornell.Context)
	if !strings.HasPrefix(versions.Rows[0].Subtitle, "Playing this one") {
		t.Errorf("versions %+v", versions.Rows[0])
	}
	other := versions.Rows[1]
	if r := ask(t, a, "pick", other.Context); r.Message != "This recording will play" {
		t.Errorf("pick %q", r.Message)
	}
	picked := strings.Fields(other.Context)[2]
	if got := a.settings().Picks["GratefulDead 1977-05-08"]; got != picked {
		t.Errorf("pick saved as %q, want %q", got, picked)
	}
	if v := ask(t, a, "versions", cornell.Context); !strings.HasPrefix(v.Rows[1].Subtitle, "Playing this one") {
		t.Errorf("after the pick: %+v", v.Rows[1])
	}
	a.change(func(s *settings) { delete(s.Picks, "GratefulDead 1977-05-08") })

	// Play Show: the queue file and .play, then the first song lands tagged in the cache
	if r := ask(t, a, "play", play.Context); !strings.HasPrefix(r.Message, "Loading: about") && r.Message != "Playing" {
		t.Errorf("play %q", r.Message)
	}
	// the show page has a live first row while its recording streams
	for end := time.Now().Add(3 * time.Second); a.eng.Current() == "" && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	if r := ask(t, a, "show", cornell.Context); r.Message != q2spot.Refresh || r.Rows[0].URI == "play" {
		t.Errorf("show while streaming: message %q, first row %+v", r.Message, r.Rows[0])
	}
	var first string
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		m, _ := filepath.Glob(filepath.Join(a.eng.Cache, "gd77-05-08.sbd.hicks.4982.sbeok.shnf", "01 - *.ogg"))
		if len(m) == 1 {
			first = m[0]
			break
		}
	}
	if first == "" {
		t.Fatal("first song never landed")
	}
	// read it as the firmware would; on Windows a fresh file can be locked for a moment (a scanner)
	var b []byte
	var err error
	for try := 0; try < 50; try++ {
		if b, err = os.ReadFile(first); err == nil && len(b) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TITLE=Minglewood Blues", "ARTIST=Grateful Dead", "ALBUM=1977-05-08 Barton Hall",
		"METADATA_BLOCK_PICTURE="} {
		if !strings.Contains(string(b), want) {
			i := strings.Index(string(b), "ALBUM=")
			t.Errorf("first song lacks %q (album tag: %.60q)", want, string(b[max(i, 0):]))
		}
	}
	if t.Failed() {
		orig, _ := os.ReadFile(filepath.Join("..", "..", "internal", "tag", "testdata", "head.ogg"))
		t.Logf("file %d bytes, fixture %d, starts %q", len(b), len(orig), b[:min(len(b), 40)])
		log, _ := os.ReadFile(filepath.Join(a.dir, "log.txt"))
		t.Logf("log:\n%s", log)
	}
	playFile, _ := os.ReadFile(filepath.Join(tmp, "q2-lma.play"))
	if !strings.HasSuffix(string(playFile), ".m3u") {
		t.Errorf(".play %q", playFile)
	}
	if r := ask(t, a, "menu"); r.Message != q2spot.Refresh {
		t.Errorf("menu while streaming: message %q", r.Message)
	}
}

func TestBadContextsRefused(t *testing.T) {
	a, _ := testApp(t)
	for _, req := range [][]string{{"artist", "../../x"}, {"year", "GratefulDead"}, {"show", "GratefulDead 1977"},
		{"play", "GratefulDead 1977-05-08 ../x 0"}, {"pick", "GratefulDead 1977-05-08 a/b"}, {"letter", "GG"},
		{"fav", ".."}} {
		r := a.device(q2spot.Request{ID: "1", Op: req[0], Args: req[1:]})
		if r.Status != q2spot.Bad {
			t.Errorf("%v: status %q", req, r.Status)
		}
	}
}

func TestSettings(t *testing.T) {
	a, _ := testApp(t)
	if r := ask(t, a, "cachesize"); r.Message != "Cache Size: 4.0 GB" {
		t.Errorf("%q", r.Message)
	}
	if r := ask(t, a, "format"); r.Message != "Prefer: MP3" {
		t.Errorf("%q", r.Message)
	}
	b, _ := os.ReadFile(filepath.Join(a.dir, "settings.json"))
	if !strings.Contains(string(b), `"cacheMB": 4096`) || !strings.Contains(string(b), `"prefer": "mp3"`) {
		t.Errorf("settings.json %s", b)
	}
	if r := ask(t, a, "clear"); !strings.HasPrefix(r.Message, "Press again") {
		t.Errorf("first clear %q", r.Message)
	}
	if r := ask(t, a, "clear"); r.Message != "Cache cleared" {
		t.Errorf("second clear %q", r.Message)
	}
}

// Opening a show fetches its first song; Play Show then finds it, with no second download.
func TestPrefetchOnShowPage(t *testing.T) {
	downloads.Clear()
	a, _ := testApp(t)
	ctx := "GratefulDead 1977-05-08"
	show := ask(t, a, "show", ctx)
	first := "gd77-05-08eaton-d1t01.ogg"
	var song string
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if m, _ := filepath.Glob(filepath.Join(a.eng.Cache, "gd77-05-08.sbd.hicks.4982.sbeok.shnf", "01 - *.ogg")); len(m) == 1 {
			song = m[0]
			break
		}
	}
	if song == "" {
		t.Fatal("opening the show fetched nothing")
	}
	if got := downloaded("gd77-05-08eaton-d1t02.ogg"); got != 0 {
		t.Errorf("song 2 fetched before a play: %d", got)
	}
	if r := ask(t, a, "play", find(t, show, "Play Show").Context); r.Message != "Playing" {
		t.Errorf("play after the prefetch: %q", r.Message)
	}
	for end := time.Now().Add(5 * time.Second); downloaded("gd77-05-08eaton-d1t02.ogg") == 0 && time.Now().Before(end); {
		time.Sleep(20 * time.Millisecond)
	}
	if got := downloaded(first); got != 1 {
		t.Errorf("song 1 downloaded %d times", got)
	}
}

func TestThemeIcons(t *testing.T) {
	a, _ := testApp(t)
	cfg := filepath.Join(t.TempDir(), "config.ini")
	for _, c := range []struct{ ini, want string }{
		{"[PLAYSET]\nARTISTTYPE=1\n[IPOD]\nACCENT=4\nTHEME=1\n", "classic"},
		{"[IPOD]\r\nTHEME = 0\r\n", "minimal"},
		{"[IPOD]\nACCENT=2\n", "minimal"},
		{"garbage", ""},
	} {
		os.WriteFile(cfg, []byte(c.ini), 0o644)
		if got := theme(cfg); got != c.want {
			t.Errorf("%q: %q, want %q", c.ini, got, c.want)
		}
	}
	os.WriteFile(cfg, []byte("[IPOD]\nTHEME=1\n"), 0o644)
	a.themeIcons(cfg)
	want, _ := icons.ReadFile("icons/classic-40.png")
	if got, _ := os.ReadFile(filepath.Join(a.dir, "icon-40.png")); string(got) != string(want) {
		t.Error("Classic's icon not written")
	}
	before, _ := os.Stat(filepath.Join(a.dir, "icon-40.png"))
	a.themeIcons(cfg) // unchanged: not written again
	if after, _ := os.Stat(filepath.Join(a.dir, "icon-40.png")); !after.ModTime().Equal(before.ModTime()) {
		t.Error("rewritten though unchanged")
	}
	os.Remove(cfg)
	a.themeIcons(cfg) // unreadable: left as they are
	if got, _ := os.ReadFile(filepath.Join(a.dir, "icon-40.png")); string(got) != string(want) {
		t.Error("icons changed without a theme")
	}
}

func TestShowRowWording(t *testing.T) {
	s := ia.Show{Date: "2000-04-05", Versions: make([]ia.Recording, 2)}
	r := showRow("x", s, ia.Recording{ID: "wsp2000-04-05.aud", Venue: "Mitchell Center", Coverage: "Mobile, AL."})
	if r.Title != "Apr 5 · Mitchell Center" || r.Subtitle != "Mobile, AL · AUD · 2 versions" {
		t.Errorf("%q / %q", r.Title, r.Subtitle)
	}
	r = showRow("x", ia.Show{Date: "1977-02-17", Versions: make([]ia.Recording, 1)},
		ia.Recording{ID: "gd77.sbd", Venue: "Arista Studios", Coverage: "Unknown, Unknown", Rating: 4})
	if r.Subtitle != "SBD 4.0★" {
		t.Errorf("%q", r.Subtitle)
	}
}

// While the first song downloads a second press changes nothing; once it is on the card a press
// plays again. The menu and the show page open with what streams.
func TestPlayGuardAndNowRow(t *testing.T) {
	downloads.Clear()
	hold := make(chan struct{})
	gate.Store(&hold)
	defer gate.Store(nil)
	a, _ := testApp(t)
	rec := "gd77-05-08.sbd.hicks.4982.sbeok.shnf"
	play := "GratefulDead 1977-05-08 " + rec + " 0"
	if r := ask(t, a, "play", play); !strings.HasPrefix(r.Message, "Loading: about") {
		t.Fatalf("first press %q", r.Message)
	}
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if _, ok := a.eng.NowPlaying(); ok {
			break
		}
	}
	if r := ask(t, a, "play", play); r.Message != "Already loading" {
		t.Errorf("second press %q", r.Message)
	}
	menu := ask(t, a, "menu")
	if menu.Rows[0].Kind != q2spot.KindInfo || menu.Rows[0].Title != "Minglewood Blues" || !strings.HasPrefix(menu.Rows[0].Subtitle, "Loading") {
		t.Errorf("menu's first row %+v", menu.Rows[0])
	}
	if menu.Rows[1].Title != "Grateful Dead" || menu.Rows[1].Subtitle != "May 8, 1977 · Barton Hall" {
		t.Errorf("menu's show row %+v", menu.Rows[1])
	}
	if menu.Rows[2].Title != "My Artists" || menu.Message != q2spot.Refresh {
		t.Errorf("menu %q %+v", menu.Message, menu.Rows[2])
	}
	close(hold)
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if n, ok := a.eng.NowPlaying(); ok && n.Ready {
			break
		}
	}
	if got := ask(t, a, "menu").Rows[0]; got.Subtitle != "Playing 1 of 20" {
		t.Errorf("playing %+v", got)
	}
	if got := ask(t, a, "show", "GratefulDead 1977-05-08").Rows[0]; got.Title != "Minglewood Blues" {
		t.Errorf("show's first row %+v", got)
	}
	if r := ask(t, a, "play", play); r.Message != "Playing" {
		t.Errorf("press once it is on the card %q", r.Message)
	}
	// that press starts behind its reply: let it finish, so nothing writes as the card is removed
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		a.mu.Lock()
		idle := a.starting == ""
		a.mu.Unlock()
		if idle {
			break
		}
	}
	a.eng.Stop()
}

func TestWrappedRowsReadWhole(t *testing.T) {
	r := wrapped("Scarlet Begonias-> Fire On The Mountain", "Playing 13 of 20")
	if r.Title+" "+strings.TrimSuffix(r.Subtitle, " · Playing 13 of 20") != "Scarlet Begonias-> Fire On The Mountain" {
		t.Errorf("%q / %q", r.Title, r.Subtitle)
	}
}
