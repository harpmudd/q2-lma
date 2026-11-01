package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Settings' Test Download Speed: the same 4 MB of one archive.org file, over one connection and
// then split over three at once, the bytes thrown away (the card isn't measured). It says whether
// the Q2's downloads are held back by its Wi-Fi (both alike) or by archive.org's speed for one
// connection (three faster), which decides whether a song is worth fetching in parallel pieces.
const (
	speedID    = "gd77-05-08.sbd.hicks.4982.sbeok.shnf"
	speedFile  = "gd77-05-08eaton-d2t04.mp3" // 38 MB
	speedBytes = 4 << 20
)

func (a *app) speedTest() {
	a.mu.Lock()
	if a.speedRunning {
		a.mu.Unlock()
		return
	}
	a.speedRunning, a.speedLine = true, "Testing…"
	a.mu.Unlock()
	one, err1 := a.timedRanges(1)
	three, err3 := a.timedRanges(3)
	line := fmt.Sprintf("1 connection %.1f MB/s · 3 at once %.1f MB/s", one, three)
	if err := firstErr(err1, err3); err != nil {
		line = "Test failed: " + err.Error()
	}
	a.log.Printf("speed test: %s", line)
	a.mu.Lock()
	a.speedRunning, a.speedLine = false, line
	a.mu.Unlock()
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// timedRanges fetches speedBytes in n equal Range requests at once; MB/s over the whole.
func (a *app) timedRanges(n int) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	part := speedBytes / n
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := time.Now()
	for k := 0; k < n; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.ia.DownloadURL(speedID, speedFile), nil)
			if err != nil {
				errs[k] = err
				return
			}
			req.Header.Set("User-Agent", userAgent)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", k*part, (k+1)*part-1))
			resp, err := a.hc.Do(req)
			if err != nil {
				errs[k] = err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				errs[k] = fmt.Errorf("archive.org answered %s", resp.Status)
				return
			}
			_, errs[k] = io.Copy(io.Discard, resp.Body)
		}()
	}
	wg.Wait()
	return float64(part*n) / 1e6 / time.Since(start).Seconds(), firstErr(errs...)
}
