package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"q2-lma/internal/ia"
	"q2-lma/internal/q2"
	"q2-lma/internal/stream"
	"q2-lma/internal/tag"
)

// get is a GET with the app's User-Agent; anything but 2xx is an error.
func (a *app) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, &ia.HTTPError{Code: resp.StatusCode} // the stream tells a passing 5xx from a 404
	}
	return resp, nil
}

// progress is the download running now, for the show page's bar and the toast's estimate.
type progress struct {
	id          string // the recording
	index       int
	title       string
	done, total int64
	active      bool
	rate        float64 // bytes a second, from the downloads so far
}

func (a *app) progress() progress {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prog
}

// coverWait is how long a song waits for its recording's own art before taking the logo: the art
// must never hold up the first song (it did in 0.2: fetched whole before the song began).
const coverWait = 2 * time.Second

// cover is the recording's cover as a JPEG: its own picture when it has one that reads, else the
// llama logo. Kept as .cover.jpg in its cache folder (hidden, so the firmware's folder listing
// skips it), so a replay doesn't fetch it again. The picture is fetched once, in the background;
// a song whose download starts before it is there gets the logo.
func (a *app) cover(s *stream.Session) []byte {
	f := path.Join(a.eng.Cache, s.ID, ".cover.jpg")
	if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
		return b
	}
	if s.ArtFile == "" {
		os.MkdirAll(path.Dir(f), 0o755)
		os.WriteFile(f, tag.Logo(), 0o644)
		return tag.Logo()
	}
	a.mu.Lock()
	done := a.covers[s.ID]
	if done == nil {
		done = make(chan struct{})
		a.covers[s.ID] = done
		id, art := s.ID, s.ArtFile
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			img := tag.Logo()
			if resp, err := a.get(ctx, a.ia.DownloadURL(id, art)); err == nil {
				raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
				resp.Body.Close()
				if c, cerr := tag.Cover(raw); err == nil && cerr == nil {
					img = c
				} else {
					a.log.Printf("cover %s/%s: %v %v", id, art, err, cerr)
				}
			} else {
				a.log.Printf("cover %s/%s: %v", id, art, err)
			}
			os.MkdirAll(path.Dir(f), 0o755)
			q2.WriteFile(f, img)
		}()
	}
	a.mu.Unlock()
	select {
	case <-done:
		if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
			return b
		}
	case <-time.After(coverWait):
	}
	return tag.Logo()
}

// fetchShared is the stream's and the prefetch's download of a song: one download a file, a second
// caller waiting for the first's. A prefetch (prefetch true) can be called off by the next show page
// opened, unless a play is waiting on it.
func (a *app) fetchShared(ctx context.Context, s *stream.Session, i int, dst string, prefetch bool) error {
	a.mu.Lock()
	if c := a.calls[dst]; c != nil {
		if !prefetch {
			c.pinned = true
		}
		a.mu.Unlock()
		select {
		case <-c.done:
			if c.err != nil && !prefetch && errors.Is(c.err, context.Canceled) {
				return a.fetchShared(ctx, s, i, dst, prefetch) // a prefetch called off before the pin
			}
			return c.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	run, cancel := ctx, context.CancelFunc(func() {})
	if prefetch {
		run, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
	}
	c := &call{done: make(chan struct{}), cancel: cancel, pinned: !prefetch}
	a.calls[dst] = c
	a.mu.Unlock()
	c.err = a.fetchSong(run, s, i, dst)
	cancel()
	a.mu.Lock()
	delete(a.calls, dst)
	a.mu.Unlock()
	close(c.done)
	return c.err
}

type call struct {
	done   chan struct{}
	err    error
	cancel context.CancelFunc
	pinned bool // a play waits on it: not called off
}

// prefetch fetches the first song of a recording when its show page opens, so Play Show often
// finds it on the card. One at a time: the one before is called off unless a play waits on it.
// Nothing is fetched while the recording streams already, or when the song is on the card.
func (a *app) prefetch(id, date, rec string) {
	if a.eng.Current() == rec {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s, err := a.session(ctx, id, date, rec, 0)
		cancel()
		if err != nil {
			return
		}
		dst := stream.SongPath(a.eng.Cache, rec, s.Songs[0])
		if _, err := os.Stat(dst); err == nil {
			a.cover(&s) // its art, if a replay lacks it
			return
		}
		a.mu.Lock()
		if old := a.calls[a.pre]; old != nil && a.pre != dst && !old.pinned {
			old.cancel()
		}
		busy := a.calls[dst] != nil
		a.pre = dst
		a.mu.Unlock()
		if busy {
			return
		}
		os.MkdirAll(path.Dir(dst), 0o755)
		if err := a.fetchShared(context.Background(), &s, 0, dst, true); err != nil && !errors.Is(err, context.Canceled) {
			a.log.Printf("prefetch %s: %v", rec, err)
		} else if err == nil {
			a.log.Printf("prefetched %s/%s", rec, s.Songs[0].File)
			if a.eng.Current() == "" { // else the stream's next download trims, keeping its own show
				stream.Evict(a.eng.Cache, a.eng.Cap(), rec, a.log.Printf)
			}
		}
	}()
}

// stopPrefetch calls off the prefetch running, unless it is for rec (or a play waits on it).
func (a *app) stopPrefetch(rec string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.calls[a.pre]; c != nil && !c.pinned && !strings.Contains(a.pre, "/"+rec+"/") {
		c.cancel()
	}
}

type counter struct {
	r    io.Reader
	n    int64
	tick func(int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.tick != nil {
		c.tick(c.n)
	}
	return n, err
}

// fetchSong downloads song i of s to dst, tagged on the way (title, artist, album, date, track,
// cover): written to dst.part, checked against the length the server sent, synced, renamed into
// place, so the firmware never sees half a song.
func (a *app) fetchSong(ctx context.Context, s *stream.Session, i int, dst string) error {
	sg := s.Songs[i]
	m := tag.Meta{Title: sg.Title, Artist: s.Artist, Album: s.Album, Date: s.Date, Track: sg.Track,
		Tracks: len(s.Songs), Cover: a.cover(s)}
	resp, err := a.get(ctx, a.ia.DownloadURL(s.ID, sg.File))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	part := dst + ".part"
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	if total <= 0 {
		total = sg.Size
	}
	start := time.Now()
	a.mu.Lock()
	a.prog = progress{id: s.ID, index: i, title: sg.Title, total: total, active: true, rate: a.prog.rate}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.prog.active = false
		a.mu.Unlock()
	}()
	in := &counter{r: resp.Body, tick: func(n int64) {
		a.mu.Lock()
		a.prog.done = n
		a.mu.Unlock()
	}}
	_, err = tag.Write(out, in, sg.Ext, m)
	if err == nil && resp.ContentLength >= 0 && in.n != resp.ContentLength {
		err = fmt.Errorf("got %d of %d bytes", in.n, resp.ContentLength)
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(part, dst)
	}
	if err != nil {
		os.Remove(part)
		return err
	}
	a.forgetSize()
	if secs := time.Since(start).Seconds(); secs > 0.5 && in.n > 1<<20 {
		a.mu.Lock()
		a.prog.rate = float64(in.n) / secs
		a.mu.Unlock()
	}
	a.log.Printf("fetched %s/%s (%d KB in %.1f s)", s.ID, sg.File, in.n>>10, time.Since(start).Seconds())
	return nil
}
