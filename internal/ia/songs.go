package ia

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// Song is one track of a recording as the app streams it: the file chosen for it and what the
// recording says about it.
type Song struct {
	File    string // the recording's file name, as /download/<id>/<file> takes it
	Ext     string // ".ogg" or ".mp3"
	Title   string
	Track   int
	Seconds int
	Size    int64
}

// Formats a song may stream in, by preference name.
var streamable = map[string]string{"Ogg Vorbis": ".ogg", "VBR MP3": ".mp3", "64Kbps MP3": ".mp3", "MP3": ".mp3"}

func rank(format, prefer string) int {
	switch {
	case format == "Ogg Vorbis" && prefer == "ogg", format == "VBR MP3" && prefer != "ogg":
		return 0
	case format == "VBR MP3", format == "Ogg Vorbis":
		return 1
	case format == "MP3":
		return 2
	}
	return 3 // 64Kbps MP3
}

// Songs is a recording's tracks in play order, each in the format prefer names ("ogg" or "mp3")
// when the recording has it, else the other. archive.org derives an MP3 and an Ogg from each
// original (a .flac or .shn); only the MP3 carries the title and track number, so derivatives are
// paired through their original. Order is by track number when every song has its own, else by the
// original's name (d1t01, d1t02, …).
func Songs(files []File, prefer string) []Song {
	type group struct {
		key            string
		best           File
		title, track   string
		length         string
		bestRank, have int
	}
	groups := map[string]*group{}
	var keys []string
	for _, f := range files {
		ext, ok := streamable[f.Format]
		if !ok || ext == "" {
			continue
		}
		key := f.Original
		if key == "" || f.Source == "original" {
			key = f.Name
		}
		g := groups[key]
		if g == nil {
			g = &group{key: key, bestRank: 99}
			groups[key] = g
			keys = append(keys, key)
		}
		if r := rank(f.Format, prefer); r < g.bestRank {
			g.best, g.bestRank = f, r
		}
		if f.Title != "" && g.title == "" {
			g.title = string(f.Title)
		}
		if f.Track != "" && g.track == "" {
			g.track = string(f.Track)
		}
		if g.length == "" || strings.Contains(string(f.Length), ":") {
			g.length = string(f.Length)
		}
	}
	songs := make([]Song, 0, len(keys))
	tracks := map[int]bool{}
	numbered := true
	for _, k := range keys {
		g := groups[k]
		s := Song{File: g.best.Name, Ext: streamable[g.best.Format], Title: g.title, Size: int64(g.best.Size),
			Seconds: seconds(g.length)}
		if s.Title == "" {
			s.Title = strings.TrimSuffix(path.Base(k), path.Ext(k))
		}
		n, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(g.track, "/", 2)[0]))
		if err != nil || n <= 0 || tracks[n] {
			numbered = false
		}
		tracks[n], s.Track = true, n
		songs = append(songs, s)
	}
	byKey := map[string]string{}
	for _, k := range keys {
		byKey[groups[k].best.Name] = k
	}
	sort.SliceStable(songs, func(i, j int) bool {
		if numbered {
			return songs[i].Track < songs[j].Track
		}
		return natLess(byKey[songs[i].File], byKey[songs[j].File])
	})
	for i := range songs {
		songs[i].Track = i + 1
	}
	return songs
}

// seconds reads a length as "mm:ss", "h:mm:ss" or seconds ("381.1").
func seconds(s string) int {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ":") {
		t := 0
		for _, p := range strings.Split(s, ":") {
			n, _ := strconv.Atoi(p)
			t = t*60 + n
		}
		return t
	}
	f, _ := strconv.ParseFloat(s, 64)
	return int(f + 0.5)
}

// natLess orders names with their digit runs as numbers: d1t9 before d1t10.
func natLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da > 0 && db > 0 {
			na, _ := strconv.Atoi(a[:da])
			nb, _ := strconv.Atoi(b[:db])
			if na != nb {
				return na < nb
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digits(s string) int {
	n := 0
	for n < len(s) && n < 9 && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// Art is the recording's own picture, if the uploader added one: an original JPEG or PNG under
// 8 MB, not a screenshot. The per-track PNGs are derived waveforms and __ia_thumb.jpg is
// archive.org's tile, so neither counts. "" when there is none.
func Art(files []File) string {
	for _, f := range files {
		name := strings.ToLower(f.Name)
		if f.Source == "original" && (f.Format == "JPEG" || f.Format == "PNG") && f.Size < 8<<20 &&
			!strings.Contains(name, "screenshot") && !strings.HasPrefix(path.Base(name), "__ia_") {
			return f.Name
		}
	}
	return ""
}
