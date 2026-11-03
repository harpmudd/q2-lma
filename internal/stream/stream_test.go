package stream

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"q2-lma/internal/ia"
)

// firmware stands in for Q2 Pod's stream_poll in .m3u mode: it reads .play, then the queue file,
// playing each entry once it exists (writing .playing), waiting at a missing one, and reading the
// file again while it waits, as the firmware does when its mtime changes.
type firmware struct {
	play, playing string
	songFor       time.Duration
	mu            sync.Mutex
	played        []string
}

func (f *firmware) run(t *testing.T, stop <-chan struct{}) {
	var seq string
	next := 0
	for {
		select {
		case <-stop:
			return
		case <-time.After(5 * time.Millisecond):
		}
		b, err := os.ReadFile(f.play)
		if err != nil {
			continue
		}
		s, target, _ := strings.Cut(string(b), " ")
		if s != seq {
			seq, next = s, 0
		}
		q, err := os.ReadFile(target)
		if err != nil {
			continue
		}
		var entries []string
		for _, l := range strings.Split(string(q), "\n") {
			if l != "" && !strings.HasPrefix(l, "#") {
				entries = append(entries, l)
			}
		}
		if next >= len(entries) {
			continue
		}
		e := entries[next]
		if _, err := os.Stat(e); err != nil {
			continue // waits for it
		}
		os.WriteFile(f.playing, []byte(e+"\n"), 0o644)
		f.mu.Lock()
		f.played = append(f.played, path.Base(e))
		f.mu.Unlock()
		time.Sleep(f.songFor)
		next++
	}
}

func songs(n int) []ia.Song {
	var s []ia.Song
	for i := 1; i <= n; i++ {
		s = append(s, ia.Song{File: fmt.Sprintf("t%02d.ogg", i), Ext: ".ogg", Title: fmt.Sprintf("Song %d", i),
			Track: i, Seconds: 1, Size: 100})
	}
	return s
}

func engine(t *testing.T, fetch Fetcher) (*Engine, *firmware) {
	dir := filepath.ToSlash(t.TempDir())
	var holds atomic.Int32
	e := &Engine{
		Cache: dir + "/cache", Queue: dir + "/queue", PlayFile: dir + "/q2-lma.play", PlayingFile: dir + "/q2-lma.playing",
		Playlist: func() bool { return true }, Fetch: fetch,
		Hold: func(h bool) {
			if h {
				holds.Add(1)
			}
		},
		Cap: func() int64 { return 1 << 30 }, Free: func() int64 { return 100 << 30 },
		Log: t.Logf, Poll: 5 * time.Millisecond, Gap: 0,
		Backoff: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		Transient: []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond,
			5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond},
		Idle: time.Second,
	}
	return e, &firmware{play: e.PlayFile, playing: e.PlayingFile, songFor: 15 * time.Millisecond}
}

func write(dst string) error {
	if err := os.WriteFile(dst+".part", []byte("audio"), 0o644); err != nil {
		return err
	}
	return os.Rename(dst+".part", dst)
}

func TestPlaysInOrderAndSkipsAFailedSong(t *testing.T) {
	var fetched sync.Map
	e, fw := engine(t, func(ctx context.Context, s *Session, i int, dst string) error {
		if s.Songs[i].Title == "Song 3" {
			return errors.New("HTTP 404")
		}
		fetched.Store(s.Songs[i].Title, true)
		time.Sleep(5 * time.Millisecond)
		return write(dst)
	})
	stop := make(chan struct{})
	defer close(stop)
	go fw.run(t, stop)
	if err := e.Start(Session{ID: "gd77-05-08.test", Artist: "Grateful Dead", Songs: songs(6)}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.Active() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	fw.mu.Lock()
	got := strings.Join(fw.played, ",")
	fw.mu.Unlock()
	want := "01 - Song 1.ogg,02 - Song 2.ogg,04 - Song 4.ogg,05 - Song 5.ogg,06 - Song 6.ogg"
	if got != want {
		t.Errorf("played %s\nwant   %s", got, want)
	}
	if _, _, err := e.Status(); err == nil || !err.Skip || err.Title != "Song 3" {
		t.Errorf("status error %+v", err)
	}
}

func TestNetworkErrorsDontUseTries(t *testing.T) {
	var calls atomic.Int32
	e, fw := engine(t, func(ctx context.Context, s *Session, i int, dst string) error {
		if i == 0 && calls.Add(1) <= 6 { // more failures than tries, all of the network
			return context.DeadlineExceeded
		}
		return write(dst)
	})
	stop := make(chan struct{})
	defer close(stop)
	go fw.run(t, stop)
	e.Start(Session{ID: "net.test", Songs: songs(2)})
	for deadline := time.Now().Add(5 * time.Second); e.Active() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if len(fw.played) != 2 || fw.played[0] != "01 - Song 1.ogg" {
		t.Errorf("played %v after %d calls", fw.played, calls.Load())
	}
}

func TestServerErrorsWaitThenTheOtherFormat(t *testing.T) {
	// song 1 answers 500 five times then plays; song 2's Ogg never comes, its MP3 does
	var calls sync.Map
	e, fw := engine(t, func(ctx context.Context, s *Session, i int, dst string) error {
		sg := s.Songs[i]
		n, _ := calls.LoadOrStore(sg.File, new(atomic.Int32))
		k := n.(*atomic.Int32).Add(1)
		switch {
		case sg.File == "t01.ogg" && k <= 5:
			return &ia.HTTPError{Code: 500}
		case sg.File == "t02.ogg":
			return &ia.HTTPError{Code: 500}
		}
		return write(dst)
	})
	stop := make(chan struct{})
	defer close(stop)
	go fw.run(t, stop)
	ss := songs(3)
	ss[1].AltFile, ss[1].AltExt = "t02.mp3", ".mp3"
	e.Start(Session{ID: "flaky.test", Songs: ss})
	for deadline := time.Now().Add(5 * time.Second); e.Active() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	fw.mu.Lock()
	got := strings.Join(fw.played, ",")
	fw.mu.Unlock()
	if want := "01 - Song 1.ogg,02 - Song 2.mp3,03 - Song 3.ogg"; got != want {
		t.Errorf("played %s\nwant   %s", got, want)
	}
	if _, _, err := e.Status(); err != nil {
		t.Errorf("an error left: %v", err)
	}
}

func TestStalePlayingIsIgnored(t *testing.T) {
	// the last session's .playing names this session's first song (a replay from the cache)
	e, _ := engine(t, func(ctx context.Context, s *Session, i int, dst string) error { return write(dst) })
	ss := Session{ID: "replay.test", Songs: songs(8), From: 0}
	first := SongPath(e.Cache, ss.ID, ss.Songs[0])
	os.MkdirAll(path.Dir(e.PlayingFile), 0o755)
	os.WriteFile(e.PlayingFile, []byte(first+"\n"), 0o644)
	e.Start(ss)
	time.Sleep(100 * time.Millisecond)
	// with .playing unchanged, only the first song and Ahead more are fetched
	for i, sg := range ss.Songs {
		_, err := os.Stat(SongPath(e.Cache, ss.ID, sg))
		if (i <= Ahead) != (err == nil) {
			t.Errorf("song %d fetched: %v", i, err == nil)
		}
	}
	e.Stop()
}

func TestEvictKeepsTheSession(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, age time.Duration) {
		d := filepath.Join(dir, name)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "a.ogg"), make([]byte, 1000), 0o644)
		at := time.Now().Add(-age)
		os.Chtimes(d, at, at)
	}
	mk("old", 3*time.Hour)
	mk("playing", 2*time.Hour)
	mk("new", time.Hour)
	Evict(dir, 1500, "playing", nil)
	for name, want := range map[string]bool{"old": false, "playing": true, "new": false} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Errorf("%s kept: %v", name, err == nil)
		}
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"Scarlet Begonias-> Fire On The Mountain": "Scarlet Begonias-_ Fire On The Mountain", // FAT: no '>'
		"Take A Step Back/Tuning":                 "Take A Step Back_Tuning",
		"...":                                     "Track",
		"What?":                                   "What_",
	} {
		if got := safeName(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
	if len(safeName(strings.Repeat("é", 60))) > 80 {
		t.Error("too long")
	}
}

func TestLastSongCountsUntilItHasPlayed(t *testing.T) {
	e, _ := engine(t, func(ctx context.Context, s *Session, i int, dst string) error { return write(dst) })
	ss := songs(1)
	ss[0].Seconds = 1
	e.Start(Session{ID: "last.test", Songs: ss})
	time.Sleep(300 * time.Millisecond)
	n, ok := e.NowPlaying()
	if !ok || n.Title != "Song 1" || !n.Ready || n.Total != 1 {
		t.Errorf("while it plays: %+v %v", n, ok)
	}
	for end := time.Now().Add(3 * time.Second); e.Active() && time.Now().Before(end); {
		time.Sleep(20 * time.Millisecond)
	}
	if e.Active() {
		t.Error("still active after the last song's length")
	}
	if _, ok := e.NowPlaying(); ok {
		t.Error("a song shown after the session")
	}
}
