package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"q2-lma/internal/q2"
	"q2-lma/internal/tlsroots"
)

// The test show: Grateful Dead, Barton Hall, Cornell, 1977-05-08. It is in archive.org's
// stream_only collection, whose derivatives (VBR MP3, Ogg Vorbis) still download, checked
// 2026-10-31.
const (
	testID      = "gd77-05-08.sbd.hicks.4982.sbeok.shnf"
	searchQuery = "collection:GratefulDead AND year:1977"
	rangeFile   = "gd77-05-08eaton-d2t04.mp3" // 38 MB; the speed test reads its first 4 MiB
	rangeBytes  = 4 << 20
)

func dlURL(id, file string) string {
	return "https://archive.org/download/" + url.PathEscape(id) + "/" + url.PathEscape(file)
}

type step struct {
	name string
	fn   func(ctx context.Context) (string, error)
}

func (a *app) steps() []step {
	return []step{
		{"App", func(context.Context) (string, error) {
			return fmt.Sprintf("%s, %s %s/%s", version, runtime.Version(), runtime.GOOS, runtime.GOARCH), nil
		}},
		{"Kernel", func(context.Context) (string, error) { return firstLine("/proc/version") }},
		{"CPU", func(context.Context) (string, error) { return cpuInfo() }},
		{"Memory", func(context.Context) (string, error) { return memInfo() }},
		{"Card space", a.cardSpace},
		{"Card write", a.cardWrite},
		{"Firmware features", a.features},
		{"Clock", func(context.Context) (string, error) {
			s := a.start.UTC().Format("2006-01-02 15:04:05 UTC")
			if a.start.Year() < 2025 {
				return s + " (unset)", nil
			}
			return s, nil
		}},
		{"DNS", a.dns},
		{"Server time", a.serverTime},
		{"Search", a.search},
		{"Show files", a.showFiles},
		{"Download speed", a.speed},
		{"Memory used", func(context.Context) (string, error) {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			rss, _ := procField("/proc/self/status", "VmRSS")
			return fmt.Sprintf("RSS %s, Go heap %.1f MB of %.1f MB", rss, mb(m.HeapAlloc), mb(m.Sys)), nil
		}},
	}
}

func (a *app) runChecks() {
	steps := a.steps()
	a.mu.Lock()
	a.checking, a.total = true, len(steps)
	a.mu.Unlock()
	for _, s := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		t0 := time.Now()
		v, err := s.fn(ctx)
		cancel()
		r := result{name: s.name, value: v, ok: err == nil}
		if err != nil {
			r.value = "FAIL " + strings.TrimPrefix(v+": ", ": ") + err.Error()
		}
		a.log.Printf("check %s (%s): %s", s.name, time.Since(t0).Round(time.Millisecond), r.value)
		a.mu.Lock()
		a.results = append(a.results, r)
		a.mu.Unlock()
	}
	a.mu.Lock()
	a.checking = false
	a.mu.Unlock()
	a.writeReport()
}

func mb(n uint64) float64 { return float64(n) / (1 << 20) }

func firstLine(file string) (string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	s, _, _ := strings.Cut(string(b), "\n")
	return s, nil
}

// procField is the value of "key:" in a /proc file.
func procField(file, key string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("no %s in %s", key, file)
}

func cpuInfo() (string, error) {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	var sys, model string
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		k, v, _ := strings.Cut(l, ":")
		switch strings.TrimSpace(k) {
		case "system type":
			sys = strings.TrimSpace(v)
		case "cpu model":
			model = strings.TrimSpace(v)
		case "processor":
			n++
		}
	}
	return fmt.Sprintf("%s, %s, %d cores", sys, model, n), nil
}

func memInfo() (string, error) {
	total, err := procField("/proc/meminfo", "MemTotal")
	if err != nil {
		return "", err
	}
	avail, _ := procField("/proc/meminfo", "MemAvailable")
	return "total " + total + ", available " + avail, nil
}

func (a *app) cardSpace(context.Context) (string, error) {
	free, total, err := diskSpace(a.card)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.1f GB free of %.1f GB", float64(free)/1e9, float64(total)/1e9), nil
}

// cardWrite writes and syncs 4 MiB in the app's folder, as a streamed song is written.
func (a *app) cardWrite(context.Context) (string, error) {
	f := path.Join(a.dir, "probe.bin")
	defer os.Remove(f)
	buf := make([]byte, 256<<10)
	t0 := time.Now()
	out, err := os.Create(f)
	if err != nil {
		return "", err
	}
	for i := 0; i < 16 && err == nil; i++ {
		_, err = out.Write(buf)
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return rate(16*len(buf), time.Since(t0)), nil
}

func rate(n int, d time.Duration) string {
	return fmt.Sprintf("%.1f MB in %.1f s = %.2f MB/s", float64(n)/1e6, d.Seconds(), float64(n)/1e6/d.Seconds())
}

func (a *app) features(context.Context) (string, error) {
	c := q2.ReadCaps(path.Join(a.tmp, "q2-addon.caps"))
	a.mu.Lock()
	a.caps = c
	a.mu.Unlock()
	switch {
	case !c.Present:
		return "", fmt.Errorf("no /tmp/q2-addon.caps: firmware older than 1.5.9")
	case c.Raw == "":
		return "(none listed)", nil
	}
	return c.Raw, nil
}

func (a *app) dns(ctx context.Context) (string, error) {
	t0 := time.Now()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", "archive.org")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("archive.org: %d addresses (%s) in %d ms", len(ips), ips[0], time.Since(t0).Milliseconds()), nil
}

func (a *app) serverTime(ctx context.Context) (string, error) {
	d, err := tlsroots.Sync(ctx, "https://archive.org/")
	if err != nil {
		return "", err
	}
	off := time.Until(d).Round(time.Second)
	return fmt.Sprintf("%s, system clock off by %s", d.UTC().Format("2006-01-02 15:04:05 UTC"), off), nil
}

// get is a verified GET with the app's User-Agent; anything but 2xx is an error.
func (a *app) get(ctx context.Context, u string, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp, nil
}

func (a *app) search(ctx context.Context) (string, error) {
	q := url.Values{"q": {searchQuery}, "fl[]": {"identifier", "date", "avg_rating", "downloads", "source"},
		"sort[]": {"downloads desc"}, "rows": {"50"}, "output": {"json"}}
	t0 := time.Now()
	resp, err := a.get(ctx, "https://archive.org/advancedsearch.php?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		Response struct {
			NumFound int `json:"numFound"`
			Docs     []struct {
				Identifier string `json:"identifier"`
			} `json:"docs"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d items for 1977, %d rows in %.1f s", v.Response.NumFound, len(v.Response.Docs),
		time.Since(t0).Seconds()), nil
}

func (a *app) showFiles(ctx context.Context) (string, error) {
	t0 := time.Now()
	resp, err := a.get(ctx, "https://archive.org/metadata/"+url.PathEscape(testID)+"/files", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var v struct {
		Result []struct {
			Name, Format string
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	mp3, ogg := 0, 0
	for _, f := range v.Result {
		switch f.Format {
		case "VBR MP3":
			mp3++
		case "Ogg Vorbis":
			ogg++
		}
	}
	return fmt.Sprintf("%d MP3, %d Ogg; %d KB in %.1f s", mp3, ogg, len(body)>>10, time.Since(t0).Seconds()), nil
}

func (a *app) speed(ctx context.Context) (string, error) {
	f := path.Join(a.dir, "probe.bin")
	defer os.Remove(f)
	n, host, took, err := a.download(ctx, dlURL(testID, rangeFile), f, fmt.Sprintf("bytes=0-%d", rangeBytes-1))
	if err != nil {
		return "", err
	}
	return rate(int(n), took) + " from " + host, nil
}

// download fetches u (a Range when rng isn't "") to dst: written to dst.part, synced, renamed
// into place, so the firmware never sees half a song. It returns the bytes, the host that served
// them (archive.org redirects to a storage node) and the time taken.
func (a *app) download(ctx context.Context, u, dst, rng string) (int64, string, time.Duration, error) {
	t0 := time.Now()
	hdr := map[string]string{}
	if rng != "" {
		hdr["Range"] = rng
	}
	resp, err := a.get(ctx, u, hdr)
	if err != nil {
		return 0, "", 0, err
	}
	defer resp.Body.Close()
	part := dst + ".part"
	out, err := os.Create(part)
	if err != nil {
		return 0, "", 0, err
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, 256<<20))
	if err == nil && resp.ContentLength >= 0 && n != resp.ContentLength {
		err = fmt.Errorf("got %d of %d bytes", n, resp.ContentLength)
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
		return n, "", 0, err
	}
	return n, resp.Request.URL.Host, time.Since(t0), nil
}

// writeReport saves every result, and the latest Play test, to probe.txt on the card.
func (a *app) writeReport() {
	var b strings.Builder
	fmt.Fprintf(&b, "q2-lma probe %s\nstarted %s (system clock), written %s (corrected)\n\n", version,
		a.start.UTC().Format(time.RFC3339), tlsroots.Now().UTC().Format(time.RFC3339))
	a.mu.Lock()
	for _, r := range a.results {
		fmt.Fprintf(&b, "%-18s %s\n", r.name+":", r.value)
	}
	p := a.play
	a.mu.Unlock()
	if p != nil {
		b.WriteString("\n" + p.report())
	}
	if err := q2.WriteFile(path.Join(a.dir, "probe.txt"), []byte(b.String())); err != nil {
		a.log.Printf("probe.txt: %v", err)
	}
}
