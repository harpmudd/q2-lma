// Package stream plays a recording through Q2 Pod's Stream mode (q2-pod patch/addon.c
// stream_poll): songs are fetched whole into the card cache, a few ahead of the one playing, and
// the firmware plays each once it is there. With the firmware's "playlist" feature, .play names an
// .m3u of the songs in order, which the firmware waits on entry by entry; without it, .play names
// the recording's cache folder, played by name. The firmware writes the path playing to .playing.
package stream

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"q2-lma/internal/ia"
	"q2-lma/internal/q2"
)

const (
	Ahead   = 2 // songs fetched past the one playing
	minFree = 1 << 30
)

// Session is a recording to play, from song From.
type Session struct {
	ID      string // the recording's identifier, its cache folder's name
	Artist  string
	Album   string // "1977-05-08 Barton Hall, Cornell University"
	Date    string
	Songs   []ia.Song
	From    int
	ArtFile string // the recording's own picture, "" for the logo
}

// Fetcher downloads song i of s to dst (through dst+".part", synced, renamed into place).
type Fetcher func(ctx context.Context, s *Session, i int, dst string) error

// Engine runs one session at a time.
type Engine struct {
	Cache       string      // .lma/cache
	Queue       string      // .lma/queue
	PlayFile    string      // /tmp/q2-lma.play
	PlayingFile string      // /tmp/q2-lma.playing
	Playlist    func() bool // the firmware's "playlist" feature
	Fetch       Fetcher
	Hold        func(bool)   // standby held off while a download runs
	Cap         func() int64 // the cache's size limit
	Free        func() int64 // bytes free on the card
	Log         func(format string, a ...any)
	Poll        time.Duration   // how often .playing is read; 1 s
	Gap         time.Duration   // the least time between two queue writes: FAT keeps mtimes in 2 s
	Backoff     []time.Duration // the waits between a song's tries
	Transient   []time.Duration // the waits after passing failures (transient), which use no try
	Idle        time.Duration   // a session is over when nothing new has played for this long past a song's end

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	cur    *run
	err    atomic.Pointer[Err]
}

// Err is a song that couldn't be had, or the reason fetching stopped.
type Err struct {
	Title string
	At    time.Time
	Err   error
	Skip  bool // dropped from the queue, and the list played on
}

func (e *Err) Error() string {
	switch {
	case errors.Is(e.Err, ErrCardFull):
		return "Card full: streaming stopped"
	case e.Skip:
		return fmt.Sprintf("Skipped %q: it couldn't be fetched", e.Title)
	}
	return fmt.Sprintf("Couldn't stream %q", e.Title)
}
func (e *Err) Unwrap() error { return e.Err }

// ErrCardFull is the card below minFree.
var ErrCardFull = errors.New("less than 1 GB free on the card")

func (e *Engine) logf(f string, a ...any) {
	if e.Log != nil {
		e.Log(f, a...)
	}
}

// Status is what the menu shows: whether a session runs, a line about it, and its error.
func (e *Engine) Status() (active bool, line string, err *Err) {
	e.mu.Lock()
	r := e.cur
	e.mu.Unlock()
	err = e.err.Load()
	if r == nil {
		return false, "", err
	}
	select {
	case <-r.done:
		return false, "", err
	default:
	}
	return true, r.line(), err
}

// Busy reports whether a download runs now.
func (e *Engine) Busy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cur != nil && e.cur.fetching.Load()
}

// Current is the recording the session plays, "" when none runs.
func (e *Engine) Current() string { id, _ := e.Playing(); return id }

// Playing is the running session's recording and the song it started from; "" when none runs.
func (e *Engine) Playing() (id string, from int) {
	e.mu.Lock()
	r := e.cur
	e.mu.Unlock()
	if r == nil {
		return "", 0
	}
	select {
	case <-r.done:
		return "", 0
	default:
		return r.s.ID, r.s.From
	}
}

// Active reports whether a session runs.
func (e *Engine) Active() bool { a, _, _ := e.Status(); return a }

// Stop ends the session (its songs stay in the cache).
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
}

func (e *Engine) stopLocked() {
	if e.cancel != nil {
		e.cancel()
		<-e.done
		e.cancel = nil
	}
}

// SongPath is where song i of a recording lives in the cache.
func SongPath(cache, id string, s ia.Song) string {
	return path.Join(cache, id, fmt.Sprintf("%02d - %s%s", s.Track, safeName(s.Title), s.Ext))
}

// safeName is a title as a FAT file name: no reserved characters, no leading or trailing dots or
// spaces, at most 80 bytes cut on a character boundary.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, ". ")
	if len(s) > 80 {
		n := 80
		for n > 0 && s[n]&0xc0 == 0x80 {
			n--
		}
		s = strings.TrimRight(s[:n], ". ")
	}
	if s == "" {
		s = "Track"
	}
	return s
}

// Start replaces whatever streams with s. It returns at once; the session runs in the background.
func (e *Engine) Start(s Session) error {
	if !ia.ValidID(s.ID) || s.From < 0 || s.From >= len(s.Songs) {
		return errors.New("nothing to play")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
	e.err.Store(nil)
	dir := path.Join(e.Cache, s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	touch(dir) // most recently played: evicted last
	stale, _ := os.ReadFile(e.PlayingFile)
	r := &run{e: e, s: s, seq: time.Now().UnixNano(), stale: strings.TrimSpace(string(stale)), last: s.From,
		done: make(chan struct{}), tries: make([]int, len(s.Songs)), trans: make([]int, len(s.Songs)),
		next: make([]time.Time, len(s.Songs))}
	for _, sg := range s.Songs {
		r.paths = append(r.paths, SongPath(e.Cache, s.ID, sg))
	}
	cleanPart(dir)
	if e.Playlist() {
		if err := os.MkdirAll(e.Queue, 0o755); err != nil {
			return err
		}
		clearQueues(e.Queue)
		r.queue = path.Join(e.Queue, fmt.Sprintf("queue-%d.m3u", r.seq))
		if err := r.writeQueue(); err != nil {
			return err
		}
		r.announce(r.queue)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel, e.done, e.cur = cancel, r.done, r
	go func() { defer close(r.done); r.loop(ctx) }()
	e.logf("stream %s from %d (%d songs), queue %q", s.ID, s.From, len(s.Songs), r.queue)
	return nil
}

func touch(p string) { now := time.Now(); os.Chtimes(p, now, now) }

func cleanPart(dir string) {
	ents, _ := os.ReadDir(dir)
	for _, x := range ents {
		if strings.HasSuffix(x.Name(), ".part") || strings.HasSuffix(x.Name(), ".tmp") {
			os.Remove(path.Join(dir, x.Name()))
		}
	}
}

func clearQueues(dir string) {
	ents, _ := os.ReadDir(dir)
	for _, x := range ents {
		os.Remove(path.Join(dir, x.Name()))
	}
}

type run struct {
	e         *Engine
	s         Session
	paths     []string
	seq       int64
	queue     string // "" in folder mode
	stale     string // .playing when the session began: ignored until it changes
	last      int    // the song .playing named last (a song can appear twice: the nearest)
	announced bool
	wrote     time.Time
	tries     []int
	trans     []int // passing failures waited out, per song
	next      []time.Time
	fetching  atomic.Bool
	state     atomic.Value // string
	done      chan struct{}
}

func (r *run) line() string {
	s, _ := r.state.Load().(string)
	return s
}

func (r *run) set(f string, a ...any) { r.state.Store(fmt.Sprintf(f, a...)) }

// writeQueue writes the queue from the first song played on, aside and renamed into place. The
// firmware has queued the lines before the one it waits for, so only that one and those after it
// ever change.
func (r *run) writeQueue() error {
	if w := r.e.Gap - time.Since(r.wrote); w > 0 && !r.wrote.IsZero() {
		time.Sleep(w)
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for i := r.s.From; i < len(r.paths); i++ {
		fmt.Fprintf(&b, "#EXTINF:%d,%s - %s\n%s\n", r.s.Songs[i].Seconds, r.s.Artist, r.s.Songs[i].Title, r.paths[i])
	}
	var err error
	for try := 0; try < 5; try++ { // a reader holding it open fails the rename on some systems
		if err = q2.WriteFile(r.queue, []byte(b.String())); err == nil {
			r.wrote = time.Now()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

func (r *run) announce(target string) {
	if err := q2.WriteFile(r.e.PlayFile, []byte(fmt.Sprintf("%d %s", r.seq, target))); err != nil {
		r.e.logf("stream: .play: %v", err)
		return
	}
	r.announced = true
}

// current is the index .playing names, looked for from the last one found, then back; until it
// changes from the session's start, the song pressed.
func (r *run) current() (int, string) {
	b, _ := os.ReadFile(r.e.PlayingFile)
	p := strings.TrimSpace(string(b))
	if p == "" || p == r.stale {
		return r.last, p
	}
	r.stale = "" // once it has changed, a later return to the same path is real
	for i := r.last; i < len(r.paths); i++ {
		if r.paths[i] == p {
			r.last = i
			return i, p
		}
	}
	for i := r.last - 1; i >= r.s.From; i-- {
		if r.paths[i] == p {
			r.last = i
			return i, p
		}
	}
	return r.last, p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// transient is a failure of the connection or of archive.org's servers rather than of the song: a
// dropped connection, a timeout, a 429, or a 5xx, which archive.org's storage nodes answer for a
// moment and then serve the file (seen on the Q2: 500s that a retry seconds later got past).
func transient(err error) bool {
	var ne net.Error
	var th *ia.ErrThrottled
	var he *ia.HTTPError
	if errors.As(err, &he) {
		return he.Code >= 500 || he.Code == 429
	}
	return errors.As(err, &ne) || errors.As(err, &th) || errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "connection reset") || strings.Contains(err.Error(), "unexpected EOF")
}

func (r *run) loop(ctx context.Context) {
	e := r.e
	defer e.Hold(false)
	seen, since := "", time.Now()
	for ctx.Err() == nil {
		cur, now := r.current()
		if now != seen {
			seen, since = now, time.Now()
		}
		want := -1
		for i := cur; i < len(r.paths) && i <= cur+Ahead; i++ {
			if !exists(r.paths[i]) {
				want = i
				break
			}
		}
		fetched := false
		if want >= 0 && !time.Now().Before(r.next[want]) {
			fetched = r.fetchOne(ctx, want)
			if ctx.Err() != nil {
				return
			}
			if x := e.err.Load(); x != nil && errors.Is(x.Err, ErrCardFull) {
				return
			}
		}
		if !r.announced && exists(r.paths[r.s.From]) {
			r.announce(path.Join(e.Cache, r.s.ID)) // folder mode: played once a song is there
		}
		if want < 0 {
			r.set("Playing %d of %d: %s", cur+1, len(r.paths), r.s.Songs[cur].Title)
			long := e.Idle + time.Duration(r.s.Songs[cur].Seconds)*time.Second
			if cur == len(r.paths)-1 || time.Since(since) >= long {
				e.logf("stream %s over at %d", r.s.ID, cur)
				return
			}
		}
		if !fetched {
			select {
			case <-ctx.Done():
			case <-time.After(e.Poll):
			}
		}
	}
}

// fetchOne fetches song i, holding standby off meanwhile. A song that fails all its tries is
// dropped from the queue file (the firmware then plays on past it) unless the firmware may already
// have passed its line.
func (r *run) fetchOne(ctx context.Context, i int) bool {
	e, sg := r.e, r.s.Songs[i]
	if e.Free() < minFree+sg.Size {
		e.err.Store(&Err{Title: sg.Title, At: time.Now(), Err: ErrCardFull})
		e.logf("stream: card full")
		return false
	}
	r.set("Fetching %d of %d: %s", i+1, len(r.paths), sg.Title)
	r.fetching.Store(true)
	e.Hold(true)
	fctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	err := e.Fetch(fctx, &r.s, i, r.paths[i])
	cancel()
	r.fetching.Store(false)
	e.Hold(false)
	if err == nil {
		r.tries[i] = 0
		Evict(e.Cache, e.Cap(), r.s.ID, e.Log)
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	// A passing failure waits and tries again without using up a try, up to len(Transient) times.
	if transient(err) && r.trans[i] < len(e.Transient) {
		r.trans[i]++
		e.logf("stream %q: waiting %s: %v", sg.Title, e.Transient[r.trans[i]-1], err)
		r.set("Waiting for archive.org: %s", sg.Title)
		r.next[i] = time.Now().Add(e.Transient[r.trans[i]-1])
		return false
	}
	r.tries[i]++
	e.logf("stream %q try %d: %v", sg.Title, r.tries[i], err)
	if r.tries[i] < len(e.Backoff) {
		r.next[i] = time.Now().Add(e.Backoff[r.tries[i]-1])
		return false
	}
	// Out of tries: the same song in the other format, once. The firmware waits on this entry
	// (it is the current song or after it), so its line may change.
	if sg.AltFile != "" && (r.queue == "" || i >= r.last) {
		alt := sg
		alt.File, alt.Ext, alt.Size, alt.AltFile = sg.AltFile, sg.AltExt, sg.AltSize, ""
		r.s.Songs[i], r.paths[i], r.tries[i], r.trans[i] = alt, SongPath(e.Cache, r.s.ID, alt), 0, 0
		e.logf("stream %q: trying %s instead", sg.Title, alt.File)
		if r.queue != "" {
			if werr := r.writeQueue(); werr != nil {
				e.logf("stream: queue: %v", werr)
			}
		}
		return false
	}
	if r.queue == "" || i < r.last {
		e.err.Store(&Err{Title: sg.Title, At: time.Now(), Err: err})
		r.next[i] = time.Now().Add(time.Hour)
		return false
	}
	r.s.Songs = slices.Delete(r.s.Songs, i, i+1)
	r.paths = slices.Delete(r.paths, i, i+1)
	r.tries = slices.Delete(r.tries, i, i+1)
	r.trans = slices.Delete(r.trans, i, i+1)
	r.next = slices.Delete(r.next, i, i+1)
	if werr := r.writeQueue(); werr != nil {
		e.logf("stream: queue: %v", werr)
	}
	e.err.Store(&Err{Title: sg.Title, At: time.Now(), Err: err, Skip: true})
	return true
}

// Evict removes whole recordings from the cache, least recently played first, until it holds at
// most limit bytes; keep (the session's own) is never removed. A folder's mtime is when it was last
// played.
func Evict(cache string, limit int64, keep string, log func(string, ...any)) {
	ents, _ := os.ReadDir(cache)
	type rec struct {
		name string
		at   time.Time
		size int64
	}
	var recs []rec
	var total int64
	for _, x := range ents {
		if !x.IsDir() {
			continue
		}
		info, err := x.Info()
		if err != nil {
			continue
		}
		var n int64
		files, _ := os.ReadDir(path.Join(cache, x.Name()))
		for _, f := range files {
			if fi, err := f.Info(); err == nil {
				n += fi.Size()
			}
		}
		recs = append(recs, rec{x.Name(), info.ModTime(), n})
		total += n
	}
	slices.SortFunc(recs, func(a, b rec) int { return a.at.Compare(b.at) })
	for _, x := range recs {
		if total <= limit {
			return
		}
		if x.name == keep {
			continue
		}
		if os.RemoveAll(path.Join(cache, x.name)) == nil {
			total -= x.size
			if log != nil {
				log("cache: evicted %s (%d MB)", x.name, x.size>>20)
			}
		}
	}
}

// Size is the cache's total bytes.
func Size(cache string) int64 {
	var n int64
	ents, _ := os.ReadDir(cache)
	for _, x := range ents {
		files, _ := os.ReadDir(path.Join(cache, x.Name()))
		for _, f := range files {
			if fi, err := f.Info(); err == nil {
				n += fi.Size()
			}
		}
	}
	return n
}
