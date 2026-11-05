// q2-lma is a Q2 Pod card app (contrib/shanlingq2/README): the Internet Archive's Live Music
// Archive on the Shanling Q2. It browses artists, years and shows, picks the best recording of a
// date and streams it through Stream mode (internal/stream). It runs from the card's .lma folder,
// copied to /tmp first so the card is never kept busy, and draws the Q2's page over Q2SPOT/1.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"q2-lma/internal/ia"
	"q2-lma/internal/mine"
	"q2-lma/internal/q2"
	"q2-lma/internal/q2spot"
	"q2-lma/internal/stream"
	"q2-lma/internal/tlsroots"
)

const (
	version   = "q2-v0.5.1"
	userAgent = "q2-lma/0.5 (Shanling Q2 card app; +https://github.com/HarpMudd/q2-lma)"
	idleAfter = 15 * time.Minute // exit 0 after this long with no request, stream or load
	answerIn  = 8 * time.Second  // a reply within this; the firmware gives up at 15 s
)

type app struct {
	dir, card, tmp, sock string
	log                  *q2.Log
	hc                   *http.Client
	ia                   *ia.Client
	mine                 *mine.List
	eng                  *stream.Engine
	loads                loader

	mu        sync.Mutex
	set       settings
	lastReq   time.Time
	confirmAt time.Time // Clear Cache's first press
	problem   string    // the last play or shuffle that failed, for the menu
	problemAt time.Time
	sizeAt    time.Time // the cache's size, measured at most every 30 s
	size      int64
	names     sync.Map                 // artist identifier → name, from the lists sent
	prog      progress                 // the download running now
	failedRec string                   // the recording the last failed play was for
	starting  string                   // "<recording> <song>" while a play is being set up
	calls     map[string]*call         // downloads running, by file (fetchShared)
	pre       string                   // the file the latest prefetch fetches
	covers    map[string]chan struct{} // recordings whose art is being or was fetched
}

// settings is .lma/settings.json. It isn't in the release zip, so updates keep it.
type settings struct {
	CacheMB int               `json:"cacheMB"`
	Prefer  string            `json:"prefer"` // "ogg" or "mp3"
	Picks   map[string]string `json:"picks"`  // "<artist> <date>": the recording chosen in More Versions
}

var cacheSteps = []int{512, 1024, 2048, 4096, 8192}

func (a *app) tmpFile(name string) string { return path.Join(a.tmp, "q2-lma"+name) }
func (a *app) capsHas(w string) bool      { return q2.ReadCaps(path.Join(a.tmp, "q2-addon.caps")).Has(w) }

func main() {
	dir := flag.String("dir", "/mnt/mmc/.lma", "the app's folder on the card")
	card := flag.String("card", "/mnt/mmc", "the card's mount point")
	tmp := flag.String("tmp", "/tmp", "where the firmware's files and the socket live")
	base := flag.String("archive", "https://archive.org", "archive.org, or a stand-in for tests")
	config := flag.String("config", "/mnt/data/config.ini", "Q2 Pod's settings, for its theme")
	flag.Parse()
	// About 78 MB is free beside the firmware, with no swap: the GC works harder near this rather
	// than the kernel's OOM killer picking the app, or the firmware.
	debug.SetMemoryLimit(48 << 20)
	a := newApp(*dir, *card, *tmp, *base, &http.Client{Transport: tlsroots.Transport()})
	log.SetFlags(0)
	log.SetOutput(a.log)
	a.log.Printf("start %s pid %d, caps %q", version, os.Getpid(), q2.ReadCaps(path.Join(*tmp, "q2-addon.caps")).Raw)
	a.foldCrash()
	a.themeIcons(*config)
	a.setHold(false)
	if time.Now().Before(tlsroots.Floor) { // the clock unset: certificates checked at archive.org's time
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := tlsroots.Sync(ctx, *base+"/"); err != nil {
				a.log.Printf("clock: %v", err)
			}
		}()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sig
		a.log.Printf("signal %v, exiting", s)
		a.shutdown()
		os.Exit(0)
	}()
	go a.idleWatch()
	err := q2spot.Serve(a.sock, a.device)
	a.log.Printf("serve: %v", err)
	a.shutdown()
	os.Exit(1)
}

// newApp is the app with its folders, archive.org at base (a stand-in in tests) through hc.
func newApp(dir, card, tmp, base string, hc *http.Client) *app {
	os.MkdirAll(dir, 0o755)
	a := &app{dir: dir, card: card, tmp: tmp, lastReq: time.Now(), hc: hc,
		calls: map[string]*call{}, covers: map[string]chan struct{}{},
		log: &q2.Log{Path: path.Join(dir, "log.txt"), Max: 1 << 20}}
	a.sock = a.tmpFile(".browse.sock")
	a.ia = &ia.Client{HTTP: hc, Agent: userAgent, Base: base,
		Cache: &ia.Cache{Dir: path.Join(dir, "meta"), Max: 20 << 20}}
	a.mine = mine.Load(path.Join(dir, "my-artists.txt"))
	a.loadSettings()
	a.eng = &stream.Engine{
		Cache: path.Join(dir, "cache"), Queue: path.Join(dir, "queue"),
		PlayFile: a.tmpFile(".play"), PlayingFile: a.tmpFile(".playing"),
		Playlist: func() bool { return a.capsHas("playlist") },
		Fetch: func(ctx context.Context, s *stream.Session, i int, dst string) error {
			return a.fetchShared(ctx, s, i, dst, false)
		},
		Hold: a.setHold,
		Cap:  func() int64 { return int64(a.settings().CacheMB) << 20 },
		Free: func() int64 { return a.free() },
		Log:  a.log.Printf,
		Poll: time.Second, Gap: 2100 * time.Millisecond, // FAT keeps mtimes in 2 s
		Backoff:   []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second},
		Transient: []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second},
		Idle:      10 * time.Minute,
	}
	return a
}

func (a *app) loadSettings() {
	s := settings{CacheMB: 2048, Prefer: "ogg"}
	if b, err := os.ReadFile(path.Join(a.dir, "settings.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	ok := false
	for _, v := range cacheSteps {
		ok = ok || v == s.CacheMB
	}
	if !ok {
		s.CacheMB = 2048
	}
	if s.Prefer != "mp3" {
		s.Prefer = "ogg"
	}
	if s.Picks == nil {
		s.Picks = map[string]string{}
	}
	a.set = s
}

func (a *app) settings() settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set
}

// change edits the settings and saves them.
func (a *app) change(f func(*settings)) {
	a.mu.Lock()
	f(&a.set)
	b, _ := json.MarshalIndent(a.set, "", "  ")
	a.mu.Unlock()
	if err := q2.WriteFile(path.Join(a.dir, "settings.json"), b); err != nil {
		a.log.Printf("settings: %v", err)
	}
}

// foldCrash copies what the launcher kept of a failed run (its stderr, a panic's trace, its exit
// code) into log.txt, so a crash leaves a trace on the card.
func (a *app) foldCrash() {
	f := a.tmpFile(".crash")
	b, err := os.ReadFile(f)
	if err != nil {
		return
	}
	os.Remove(f)
	s := strings.TrimSpace(string(b))
	if len(s) > 4<<10 {
		s = s[:4<<10] + "\n…"
	}
	a.log.Printf("the last run failed:\n%s", s)
}

// setHold writes the status file: hold=1 keeps standby and auto power-off away, only while a
// download runs (never for a paused session: the songs ahead are already on the card).
func (a *app) setHold(hold bool) {
	s := "version=" + version + "\n"
	if hold {
		s += "hold=1\n"
	}
	if err := q2.WriteFile(a.tmpFile(".state"), []byte(s)); err != nil {
		a.log.Printf("state: %v", err)
	}
}

func (a *app) free() int64 {
	free, _, err := diskSpace(a.card)
	if err != nil {
		return 1 << 40 // not on the Q2 (a host run): don't stop for it
	}
	return int64(free)
}

// cacheSize is the audio cache's bytes, measured again at most every 30 s.
func (a *app) cacheSize() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.sizeAt) > 30*time.Second {
		a.size, a.sizeAt = stream.Size(a.eng.Cache), time.Now()
	}
	return a.size
}

func (a *app) forgetSize() { a.mu.Lock(); a.sizeAt = time.Time{}; a.mu.Unlock() }

// shutdown stops fetching and leaves no socket, status or play request behind. The songs stay.
func (a *app) shutdown() {
	a.eng.Stop()
	os.Remove(a.sock)
	os.Remove(a.tmpFile(".state"))
	os.Remove(a.tmpFile(".play"))
}

func (a *app) idleWatch() {
	for range time.Tick(30 * time.Second) {
		a.mu.Lock()
		quiet := time.Since(a.lastReq) >= idleAfter
		a.mu.Unlock()
		if quiet && !a.eng.Active() && !a.loads.running() {
			a.log.Printf("idle, exiting")
			a.shutdown()
			os.Exit(0)
		}
	}
}
