package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"q2-lma/internal/q2"
	"q2-lma/internal/q2spot"
)

// Play test: three short tracks of the test show, MP3, Ogg, MP3, fetched in order into
// .lma/stream/ and played through Stream mode: an .m3u when the firmware lists "playlist", else the
// folder itself (older firmware plays only FLAC and MP3 that way, so the Ogg would be skipped).
var playTracks = []struct{ file, name string }{
	{"gd77-05-08eaton-d1t09.mp3", "01 - Mama Tried.mp3"},          // 4.2 MB, 3:05
	{"gd77-05-08eaton-d2t03.ogg", "02 - Take A Step Back.ogg"},    // 1.2 MB, 1:50
	{"gd77-05-08eaton-d3t05.mp3", "03 - Saint Stephen (end).mp3"}, // 2.7 MB, 1:53
}

const watchFor = 12 * time.Minute // .playing is followed this long after the press

type playRun struct {
	mu       sync.Mutex
	started  time.Time
	mode     string
	fetch    string // what the download side does, "" once done
	state    string // what playback does, or the outcome
	paths    []string
	ready    []time.Duration // when each file landed, since the press; 0 not yet
	seen     []time.Duration // when .playing first named it; 0 not yet
	notes    []string
	fetching bool
	watching bool
}

func (p *playRun) live() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fetching || p.watching
}

func (p *playRun) status() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fetch != "" && p.state != "" {
		return p.fetch + "; " + p.state
	}
	return p.fetch + p.state
}

func (p *playRun) note(format string, a ...any) {
	p.notes = append(p.notes, fmt.Sprintf("%6.1f s  ", time.Since(p.started).Seconds())+fmt.Sprintf(format, a...))
}

func (p *playRun) report() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "Play test (%s mode), pressed %s\n", p.mode, p.started.UTC().Format(time.RFC3339))
	for i, t := range playTracks {
		fmt.Fprintf(&b, "  %-30s file ready %s, playing %s\n", t.name, secs(p.ready[i]), secs(p.seen[i]))
	}
	b.WriteString("  events:\n")
	for _, n := range p.notes {
		b.WriteString("  " + n + "\n")
	}
	fmt.Fprintf(&b, "  state: %s\n", strings.TrimPrefix(p.fetch+"; "+p.state, "; "))
	return b.String()
}

func secs(d time.Duration) string {
	if d == 0 {
		return "never"
	}
	return fmt.Sprintf("at %.1f s", d.Seconds())
}

func title(name string) string { return strings.TrimSuffix(name[5:], path.Ext(name)) }

func (a *app) startPlay() q2spot.Reply {
	a.mu.Lock()
	if a.play != nil && a.play.live() {
		a.mu.Unlock()
		return q2spot.Reply{Message: "Play test already running"}
	}
	p := &playRun{started: time.Now(), fetch: "Starting", fetching: true, watching: true,
		ready: make([]time.Duration, len(playTracks)), seen: make([]time.Duration, len(playTracks))}
	a.play = p
	a.mu.Unlock()
	go a.runPlay(p)
	return q2spot.Reply{Message: "Fetching 3 test tracks"}
}

func (a *app) runPlay(p *playRun) {
	set := func(f func()) { p.mu.Lock(); f(); p.mu.Unlock() }
	fail := func(what string, err error) {
		a.log.Printf("play: %s: %v", what, err)
		set(func() {
			p.fetch, p.state = "", "Failed: "+what+": "+err.Error()
			p.note("%s", p.state)
			p.fetching, p.watching = false, false
		})
		a.setHold(false)
		a.writeReport()
	}
	caps := q2.ReadCaps(path.Join(a.tmp, "q2-addon.caps"))
	sdir := path.Join(a.dir, "stream")
	os.RemoveAll(sdir)
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		fail("stream folder", err)
		return
	}
	stale, _ := os.ReadFile(a.tmpFile(".playing")) // the last session's, ignored until it changes
	seq := time.Now().UnixNano()
	paths := make([]string, len(playTracks))
	for i, t := range playTracks {
		paths[i] = path.Join(sdir, t.name)
	}
	target, mode := sdir, "folder"
	if caps.Has("playlist") {
		target, mode = path.Join(sdir, fmt.Sprintf("queue-%d.m3u", seq)), "m3u"
		if err := q2.WriteFile(target, []byte("#EXTM3U\n"+strings.Join(paths, "\n")+"\n")); err != nil {
			fail("queue file", err)
			return
		}
	}
	if err := q2.WriteFile(a.tmpFile(".play"), []byte(fmt.Sprintf("%d %s", seq, target))); err != nil {
		fail(".play", err)
		return
	}
	set(func() { p.mode, p.paths = mode, paths; p.note("wrote .play: %d %s", seq, target) })
	a.log.Printf("play: %s mode, .play %d %s", mode, seq, target)
	go a.watchPlaying(p, strings.TrimSpace(string(stale)))

	a.setHold(true) // files move: standby waits
	for i, t := range playTracks {
		set(func() { p.fetch = fmt.Sprintf("Fetching %d of %d: %s", i+1, len(playTracks), title(t.name)) })
		var (
			n    int64
			host string
			took time.Duration
			err  error
		)
		for try := 1; try <= 2; try++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			n, host, took, err = a.download(ctx, dlURL(testID, t.file), paths[i], "")
			cancel()
			if err == nil {
				break
			}
			a.log.Printf("play: %s try %d: %v", t.name, try, err)
			time.Sleep(3 * time.Second)
		}
		if err != nil {
			set(func() { p.watching = false })
			fail("fetching "+t.name, err)
			return
		}
		set(func() {
			p.ready[i] = time.Since(p.started)
			p.note("%s landed: %s from %s", t.name, rate(int(n), took), host)
		})
		a.log.Printf("play: %s %d bytes in %s from %s", t.name, n, took.Round(time.Millisecond), host)
	}
	a.setHold(false)
	set(func() {
		p.fetching, p.fetch = false, ""
		p.note("all fetched")
		if p.state == "" {
			p.state = "All fetched, waiting for playback"
		}
	})
}

// watchPlaying follows the firmware's .playing, the path it plays now, noting when each test track
// first starts. It stops when the last one has played, another queue plays, or after watchFor.
func (a *app) watchPlaying(p *playRun, stale string) {
	last := stale
	defer func() {
		p.mu.Lock()
		p.watching = false
		p.mu.Unlock()
		a.writeReport()
	}()
	for end := time.Now().Add(watchFor); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		b, _ := os.ReadFile(a.tmpFile(".playing"))
		now := strings.TrimSpace(string(b))
		if now == last || now == "" {
			continue
		}
		last = now
		i := -1
		for k, q := range p.paths {
			if q == now {
				i = k
			}
		}
		p.mu.Lock()
		if i < 0 {
			p.note(".playing names another file: %s", now)
			p.state = "Left: something else plays"
			p.mu.Unlock()
			a.log.Printf("play: left for %s", now)
			return
		}
		if p.seen[i] == 0 {
			p.seen[i] = time.Since(p.started)
		}
		p.note("playing %s", playTracks[i].name)
		p.state = fmt.Sprintf("Track %d playing: %s (first started at %.1f s)", i+1, title(playTracks[i].name),
			p.seen[0].Seconds())
		done := i == len(playTracks)-1
		p.mu.Unlock()
		a.log.Printf("play: playing %s", now)
		if done {
			p.mu.Lock()
			p.state = fmt.Sprintf("Done: all 3 reached; first started at %.1f s", p.seen[0].Seconds())
			p.mu.Unlock()
			return
		}
	}
	p.mu.Lock()
	p.note("stopped watching after %s", watchFor)
	switch {
	case p.state == "" || p.state == "All fetched, waiting for playback":
		p.state = fmt.Sprintf("Nothing played within %s", watchFor)
	case !strings.HasPrefix(p.state, "Failed"):
		p.state += " (stopped watching)"
	}
	p.mu.Unlock()
}
