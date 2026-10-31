// q2-lma, probe build: a throwaway Q2 Pod card app that measures what the Live Music Archive app's
// design depends on (docs/TESTING.md, round 1). It runs its checks once at start, shows each as a
// row on the Q2 and writes them to .lma/probe.txt; Play test streams three short tracks through
// Stream mode. Started by the card's run (contrib/shanlingq2/launcher.sh), from /tmp.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"q2-lma/internal/q2"
	"q2-lma/internal/q2spot"
	"q2-lma/internal/tlsroots"
)

const (
	version   = "q2-probe-0.1"
	userAgent = "q2-lma/" + version + " (Shanling Q2 card app)"
	idleAfter = 15 * time.Minute // exit 0 after this long with no request and nothing running
)

type result struct {
	name, value string
	ok          bool
}

type app struct {
	dir, card, tmp, sock string
	log                  *q2.Log
	hc                   *http.Client
	start                time.Time

	mu       sync.Mutex
	caps     q2.Caps
	results  []result
	total    int
	checking bool
	play     *playRun // the latest Play test; nil before the first
	lastReq  time.Time
}

func (a *app) tmpFile(name string) string { return path.Join(a.tmp, "q2-lma"+name) }

func main() {
	dir := flag.String("dir", "/mnt/mmc/.lma", "the app's folder on the card")
	card := flag.String("card", "/mnt/mmc", "the card's mount point")
	tmp := flag.String("tmp", "/tmp", "where the firmware's files and the socket live")
	flag.Parse()
	// About 67 MB is free beside the firmware, with no swap: the GC works harder near this rather
	// than the kernel's OOM killer picking the app, or the firmware.
	debug.SetMemoryLimit(48 << 20)
	os.MkdirAll(*dir, 0o755)
	a := &app{dir: *dir, card: *card, tmp: *tmp, start: time.Now(), lastReq: time.Now(),
		log: &q2.Log{Path: path.Join(*dir, "log.txt"), Max: 1 << 20},
		hc:  &http.Client{Transport: tlsroots.Transport()}}
	a.sock = a.tmpFile(".browse.sock")
	a.caps = q2.ReadCaps(path.Join(*tmp, "q2-addon.caps"))
	log.SetFlags(0)
	log.SetOutput(a.log)
	a.log.Printf("start %s pid %d, caps %q", version, os.Getpid(), a.caps.Raw)
	a.foldCrash()
	a.setHold(false)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-sig
		a.log.Printf("signal %v, exiting", s)
		a.shutdown()
		os.Exit(0)
	}()
	go a.runChecks()
	go a.idleWatch()
	err := q2spot.Serve(a.sock, a.device)
	a.log.Printf("serve: %v", err)
	a.shutdown()
	os.Exit(1)
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

// setHold writes the status file: hold=1 keeps standby and auto power-off away while files move.
func (a *app) setHold(hold bool) {
	s := "version=" + version + "\n"
	if hold {
		s += "hold=1\n"
	}
	if err := q2.WriteFile(a.tmpFile(".state"), []byte(s)); err != nil {
		a.log.Printf("state: %v", err)
	}
}

// shutdown leaves no socket, status or play request behind. Streamed songs stay: the firmware may
// still be playing them, and the next Play test clears them.
func (a *app) shutdown() {
	os.Remove(a.sock)
	os.Remove(a.tmpFile(".state"))
	os.Remove(a.tmpFile(".play"))
}

func (a *app) idleWatch() {
	for range time.Tick(30 * time.Second) {
		a.mu.Lock()
		idle := time.Since(a.lastReq) >= idleAfter && !a.checking && (a.play == nil || !a.play.live())
		a.mu.Unlock()
		if idle {
			a.log.Printf("idle, exiting")
			a.shutdown()
			os.Exit(0)
		}
	}
}

func (a *app) device(req q2spot.Request) q2spot.Reply {
	a.mu.Lock()
	a.lastReq = time.Now()
	a.mu.Unlock()
	switch req.Op {
	case "menu":
		return a.menu()
	case "play":
		return a.startPlay()
	case "stop": // an ok reply closes the page (addon.c got()); exit once it has gone out
		a.log.Printf("stopped from the Q2")
		go func() {
			time.Sleep(500 * time.Millisecond)
			a.shutdown()
			os.Exit(0)
		}()
		return q2spot.Reply{Message: "Probe stopped"}
	}
	a.log.Printf("unknown op %q %q", req.Op, req.Args)
	return q2spot.Fail(q2spot.Unsupported, "Unknown request: "+req.Op)
}

func info(title, sub string) q2spot.Row {
	return q2spot.Row{Kind: q2spot.KindInfo, Title: title, Subtitle: sub}
}

// menu is the only list: progress or a summary, Play test, every result, Quit. It asks the
// firmware to refresh it every 2 s while checks or a Play test run.
func (a *app) menu() q2spot.Reply {
	a.mu.Lock()
	defer a.mu.Unlock()
	var rows []q2spot.Row
	live := a.checking
	if a.checking {
		done, sub := len(a.results), fmt.Sprintf("%d of %d", len(a.results), a.total)
		if a.caps.Has("progress") {
			rows = append(rows, q2spot.Row{Kind: q2spot.KindProgress, Title: "Running checks", Subtitle: sub,
				Position: strconv.Itoa(done * 1000 / a.total)})
		} else {
			rows = append(rows, info("Running checks", sub))
		}
	} else {
		failed := 0
		for _, r := range a.results {
			if !r.ok {
				failed++
			}
		}
		rows = append(rows, info(fmt.Sprintf("Checks done: %d failed", failed), "Saved to .lma/probe.txt"))
	}
	sub := "MP3, Ogg, MP3: Cornell 5/8/77"
	if p := a.play; p != nil {
		sub = p.status()
		live = live || p.live()
	}
	rows = append(rows, q2spot.Row{Kind: q2spot.KindAction, URI: "play", Title: "Play test", Subtitle: sub})
	for _, r := range a.results {
		rows = append(rows, info(r.name, r.value))
	}
	rows = append(rows, q2spot.Row{Kind: q2spot.KindAction, URI: "stop", Title: "Quit probe"})
	msg := ""
	if live {
		msg = q2spot.Refresh
	}
	return q2spot.Reply{Message: msg, Rows: rows}
}
