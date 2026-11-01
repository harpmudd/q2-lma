package ia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func files(t *testing.T, id string) []File {
	var r struct{ Result []File }
	fixture(t, "files-"+id+".json", &r)
	return r.Result
}

func TestClass(t *testing.T) {
	for _, c := range []struct {
		id, source string
		want       Class
	}{
		{"gd77-05-08.sbd.hicks.4982.sbeok.shnf", "Matrix (see notes)", Soundboard},
		{"gd1977-05-08.mtx.dan.29511.flac16", "See info file", Matrix},
		{"gd77-05-08.maizner.hicks.5002.sbeok.shnf", "Audience - Sony ECM-990", Audience},
		{"um2008-10-31..flac16", "Schoeps mk41 (din)>Kc5>cmc6>Sonosax", Audience},
		{"wsp2000-04-18", "", Unknown},
	} {
		if got := ClassOf(Recording{ID: c.id, Source: Text(c.source)}); got != c.want {
			t.Errorf("%s: %v, want %v", c.id, got, c.want)
		}
	}
}

func TestShowsPickCornell(t *testing.T) {
	var r searchReply[Recording]
	fixture(t, "shows-GratefulDead-1977.json", &r)
	shows := Shows(r.Response.Docs)
	if len(shows) < 50 {
		t.Fatalf("%d dates", len(shows))
	}
	for i := 1; i < len(shows); i++ {
		if shows[i-1].Date >= shows[i].Date {
			t.Fatalf("dates out of order at %d", i)
		}
	}
	for _, s := range shows {
		if s.Date == "1977-05-08" {
			if got := s.Versions[0].ID; got != "gd77-05-08.sbd.hicks.4982.sbeok.shnf" {
				t.Errorf("Cornell best: %s", got)
			}
			return
		}
	}
	t.Error("no 1977-05-08")
}

func TestRatingDamped(t *testing.T) {
	few := Recording{Rating: 5, Reviews: 5}
	many := Recording{Rating: 4.8, Reviews: 300}
	if Rating(few) >= Rating(many) {
		t.Errorf("few %.2f >= many %.2f", Rating(few), Rating(many))
	}
	if Rating(Recording{}) != 4 {
		t.Error("unrated")
	}
}

func TestSongsPairsOggWithMP3Titles(t *testing.T) {
	songs := Songs(files(t, "gd77-05-08.sbd.hicks.4982.sbeok.shnf"), "ogg")
	if len(songs) != 20 {
		t.Fatalf("%d songs", len(songs))
	}
	first, last := songs[0], songs[len(songs)-1]
	if first.Title != "Minglewood Blues" || first.Ext != ".ogg" || !strings.HasSuffix(first.File, "d1t01.ogg") ||
		first.Track != 1 || first.Seconds != 381 {
		t.Errorf("first: %+v", first)
	}
	if last.Track != 20 || !strings.Contains(last.File, "d3t07") || last.Title != "Saturday Night" {
		t.Errorf("last: %+v", last)
	}
	if first.AltExt != ".mp3" || !strings.HasSuffix(first.AltFile, "d1t01.mp3") || first.AltSize == 0 {
		t.Errorf("first's other format: %+v", first)
	}
	mp3 := Songs(files(t, "gd77-05-08.sbd.hicks.4982.sbeok.shnf"), "mp3")
	if mp3[0].Ext != ".mp3" || mp3[0].Title != "Minglewood Blues" {
		t.Errorf("mp3: %+v", mp3[0])
	}
}

func TestSongsMP3Only(t *testing.T) {
	songs := Songs(files(t, "wsp2026-06-26"), "ogg") // no Ogg: MP3
	if len(songs) != 22 || songs[0].Ext != ".mp3" || songs[0].Title != "One-Arm Steve >" || songs[0].AltFile != "" {
		t.Fatalf("%d songs, first %+v", len(songs), songs[0])
	}
	if Art(files(t, "wsp2026-06-26")) != "" {
		t.Error("a screenshot or waveform taken as art")
	}
}

func TestNatLess(t *testing.T) {
	if !natLess("d1t9.flac", "d1t10.flac") || natLess("d2t01", "d1t09") {
		t.Error("natural order")
	}
}

func TestValidID(t *testing.T) {
	for _, ok := range []string{"GratefulDead", "gd77-05-08.sbd.hicks.4982.sbeok.shnf", "moe"} {
		if !ValidID(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "../x", "a/b", "a..b", ".hidden", "a b"} {
		if ValidID(bad) {
			t.Errorf("%q taken", bad)
		}
	}
}

func TestClientCachesAndThrottles(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		if strings.Contains(r.URL.RawQuery, "busy") {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		http.ServeFile(w, r, filepath.Join("testdata", "letter-g.json"))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Agent: "test-agent", Base: srv.URL, Cache: &Cache{Dir: t.TempDir(), Max: 1 << 20}}
	ctx := context.Background()
	as, err := c.Letter(ctx, 'g')
	if err != nil || len(as) < 100 {
		t.Fatalf("%d artists, %v", len(as), err)
	}
	for i := 1; i < len(as); i++ {
		if strings.ToLower(string(as[i-1].Name)) > strings.ToLower(string(as[i].Name)) {
			t.Fatalf("not by name at %d", i)
		}
	}
	if _, err := c.Letter(ctx, 'g'); err != nil || calls != 1 {
		t.Errorf("second ask: %d calls, %v", calls, err)
	}
	_, err = c.SearchArtists(ctx, "busy")
	if _, ok := err.(*ErrThrottled); !ok {
		t.Fatalf("429: %v", err)
	}
	before := calls
	if _, err := c.SearchArtists(ctx, "other"); err == nil || calls != before {
		t.Errorf("asked while throttled: %v", err)
	}
	c.until = time.Time{}
}
