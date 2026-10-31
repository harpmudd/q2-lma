package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

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
		return nil, fmt.Errorf("archive.org answered %s", resp.Status)
	}
	return resp, nil
}

// cover is the recording's cover as a JPEG: its own picture when it has one that reads, else the
// llama logo. Kept as .cover.jpg in its cache folder (hidden, so the firmware's folder listing
// skips it), so a replay doesn't fetch it again.
func (a *app) cover(ctx context.Context, s *stream.Session) []byte {
	f := path.Join(a.eng.Cache, s.ID, ".cover.jpg")
	if b, err := os.ReadFile(f); err == nil && len(b) > 0 {
		return b
	}
	img := tag.Logo()
	if s.ArtFile != "" {
		if resp, err := a.get(ctx, a.ia.DownloadURL(s.ID, s.ArtFile)); err == nil {
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()
			if err == nil {
				if c, err := tag.Cover(raw); err == nil {
					img = c
				} else {
					a.log.Printf("cover %s/%s: %v", s.ID, s.ArtFile, err)
				}
			}
		}
	}
	os.WriteFile(f, img, 0o644)
	return img
}

type counter struct {
	r io.Reader
	n int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// fetchSong downloads song i of s to dst, tagged on the way (title, artist, album, date, track,
// cover): written to dst.part, checked against the length the server sent, synced, renamed into
// place, so the firmware never sees half a song.
func (a *app) fetchSong(ctx context.Context, s *stream.Session, i int, dst string) error {
	sg := s.Songs[i]
	m := tag.Meta{Title: sg.Title, Artist: s.Artist, Album: s.Album, Date: s.Date, Track: sg.Track,
		Tracks: len(s.Songs), Cover: a.cover(ctx, s)}
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
	in := &counter{r: resp.Body}
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
	a.log.Printf("fetched %s/%s (%d KB)", s.ID, sg.File, in.n>>10)
	return nil
}
