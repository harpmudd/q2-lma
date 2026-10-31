package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"q2-lma/internal/q2spot"
)

// fakeArchive answers as archive.org does, from internal/ia's recorded responses; every song is
// the start of a real Ogg (internal/tag's fixture).
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
		for _, row := range r.Rows {
			if len(row.URI) > q2spot.MaxOp {
				t.Errorf("%s: op %q too long for the firmware", op, row.URI)
			}
			if !q2spot.ContextFits(row.Context) {
				t.Errorf("%s: context %q too long", op, row.Context)
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
	cornell := find(t, all(t, a, "year", y77.Context), "05-08")
	if !strings.HasPrefix(cornell.Subtitle, "SBD · 4.8★") || !strings.Contains(cornell.Subtitle, "23 versions") {
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
	if r := ask(t, a, "play", play.Context); r.Message != "Starting…" {
		t.Errorf("play %q", r.Message)
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
