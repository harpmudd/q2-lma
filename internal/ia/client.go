package ia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client asks archive.org, one request at a time and with the app's User-Agent, as its API
// etiquette asks; a 429 or 503 is honoured (Retry-After, else a minute) for every later request.
type Client struct {
	HTTP  *http.Client
	Agent string
	Base  string // https://archive.org; tests point it at a fake
	Cache *Cache

	one   sync.Mutex
	mu    sync.Mutex
	until time.Time // no request before this: throttled
}

// ErrThrottled is archive.org asking the app to slow down; Wait says for how long.
type ErrThrottled struct{ Wait time.Duration }

func (e *ErrThrottled) Error() string {
	return fmt.Sprintf("archive.org is busy: try again in %d s", int(e.Wait.Seconds()+0.5))
}

// HTTPError is any other answer than 2xx.
type HTTPError struct{ Code int }

func (e *HTTPError) Error() string { return fmt.Sprintf("archive.org answered %d", e.Code) }

func (c *Client) throttled() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := time.Until(c.until); d > 0 {
		return &ErrThrottled{d}
	}
	return nil
}

func retryAfter(h string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil && time.Until(t) > 0 {
		return time.Until(t)
	}
	return time.Minute
}

// getJSON decodes the JSON at path (and query) into v.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, v any) error {
	c.one.Lock()
	defer c.one.Unlock()
	if err := c.throttled(); err != nil {
		return err
	}
	u := c.Base + path
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.Agent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		d := retryAfter(resp.Header.Get("Retry-After"))
		c.mu.Lock()
		c.until = time.Now().Add(d)
		c.mu.Unlock()
		return &ErrThrottled{d}
	}
	if resp.StatusCode/100 != 2 {
		return &HTTPError{resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// cached answers key from the cache while fresh, else from fetch, storing what it gets. When the
// fetch fails, a stale entry is used rather than an error.
func cached[T any](c *Client, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	var v T
	found, fresh := c.Cache.Get(key, ttl, &v)
	if fresh {
		return v, nil
	}
	got, err := fetch()
	if err != nil {
		if found {
			return v, nil
		}
		return got, err
	}
	c.Cache.Put(key, got)
	return got, nil
}

type searchReply[T any] struct {
	Response struct {
		NumFound int `json:"numFound"`
		Docs     []T `json:"docs"`
	} `json:"response"`
}

func (c *Client) search(ctx context.Context, q string, fields []string, sortBy string, rows int, out any) error {
	v := url.Values{"q": {q}, "fl[]": fields, "rows": {strconv.Itoa(rows)}, "output": {"json"}}
	if sortBy != "" {
		v["sort[]"] = []string{sortBy}
	}
	return c.getJSON(ctx, "/advancedsearch.php", v, out)
}

const (
	day         = 24 * time.Hour
	artistQuery = "mediatype:collection AND collection:etree"
)

var artistFields = []string{"identifier", "title", "downloads"}

func (c *Client) artists(ctx context.Context, key, q, sortBy string, rows int) ([]Artist, error) {
	return cached(c, key, day, func() ([]Artist, error) {
		var r searchReply[Artist]
		if err := c.search(ctx, artistQuery+" AND ("+q+")", artistFields, sortBy, rows, &r); err != nil {
			return nil, err
		}
		var out []Artist
		for _, a := range r.Response.Docs {
			if ValidID(a.ID) {
				if a.Name == "" {
					a.Name = Text(a.ID)
				}
				out = append(out, a)
			}
		}
		return out, nil
	})
}

// MaxList is the most rows a list on the Q2 holds (the firmware's full-list limit).
const MaxList = 500

func byName(as []Artist) []Artist {
	sort.SliceStable(as, func(i, j int) bool {
		return strings.ToLower(string(as[i].Name)) < strings.ToLower(string(as[j].Name))
	})
	return as
}

// Letter is the MaxList most downloaded artists whose name starts with l ('#': a digit), by name.
// A letter with more is cut there; search reaches the rest.
func (c *Client) Letter(ctx context.Context, l byte) ([]Artist, error) {
	q := "titleSorter:" + string(l) + "*"
	if l == '#' {
		var p []string
		for d := '0'; d <= '9'; d++ {
			p = append(p, "titleSorter:"+string(d)+"*")
		}
		q = strings.Join(p, " OR ")
	}
	as, err := c.artists(ctx, "letter:"+string(l), q, "downloads desc", MaxList)
	return byName(as), err
}

// Top is the MaxList most downloaded artists, most first.
func (c *Client) Top(ctx context.Context) ([]Artist, error) {
	return c.artists(ctx, "top", "identifier:*", "downloads desc", MaxList)
}

// SearchArtists finds artists whose name has words starting with each word of term.
func (c *Client) SearchArtists(ctx context.Context, term string) ([]Artist, error) {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(term), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	}) {
		words = append(words, "title:"+w+"*")
	}
	if len(words) == 0 {
		return nil, nil
	}
	return c.artists(ctx, "search:"+strings.Join(words, " "), strings.Join(words, " AND "), "downloads desc", MaxList)
}

// Names looks up artists by identifier (My Artists), at most 100 at a time; an identifier
// archive.org doesn't know is missing from the answer.
func (c *Client) Names(ctx context.Context, ids []string) (map[string]Artist, error) {
	out := map[string]Artist{}
	for len(ids) > 0 {
		n := min(len(ids), 100)
		var q []string
		for _, id := range ids[:n] {
			if ValidID(id) {
				q = append(q, id)
			}
		}
		ids = ids[n:]
		if len(q) == 0 {
			continue
		}
		sort.Strings(q)
		as, err := c.artists(ctx, "names:"+strings.Join(q, ","), "identifier:("+strings.Join(q, " OR ")+")", "", 100)
		if err != nil {
			return out, err
		}
		for _, a := range as {
			out[a.ID] = a
		}
	}
	return out, nil
}

// Year is how many recordings an artist has from one year.
type Year struct {
	Year  int `json:"y"`
	Count int `json:"n"`
}

// Years lists an artist's years, newest first. The scrape API pages 10,000 items at a time; the
// Grateful Dead's 18,000 take about 9 s, so callers run this in the background.
func (c *Client) Years(ctx context.Context, coll string) ([]Year, error) {
	if !ValidID(coll) {
		return nil, fmt.Errorf("not an artist: %q", coll)
	}
	return cached(c, "years:"+coll, 7*day, func() ([]Year, error) {
		counts := map[int]int{}
		cursor := ""
		for page := 0; page < 20; page++ {
			v := url.Values{"q": {"collection:" + coll}, "fields": {"year"}, "count": {"10000"}}
			if cursor != "" {
				v.Set("cursor", cursor)
			}
			var r struct {
				Items []struct {
					Year Num `json:"year"`
				} `json:"items"`
				Cursor string `json:"cursor"`
			}
			if err := c.getJSON(ctx, "/services/search/v1/scrape", v, &r); err != nil {
				return nil, err
			}
			for _, it := range r.Items {
				if y := int(it.Year); y > 1900 {
					counts[y]++
				}
			}
			if cursor = r.Cursor; cursor == "" {
				break
			}
		}
		var ys []Year
		for y, n := range counts {
			ys = append(ys, Year{y, n})
		}
		sort.Slice(ys, func(i, j int) bool { return ys[i].Year > ys[j].Year })
		return ys, nil
	})
}

var recordingFields = []string{"identifier", "date", "source", "avg_rating", "num_reviews", "downloads",
	"venue", "coverage", "title"}

// Recordings is every recording an artist has from one year, by date.
func (c *Client) Recordings(ctx context.Context, coll string, year int) ([]Recording, error) {
	if !ValidID(coll) {
		return nil, fmt.Errorf("not an artist: %q", coll)
	}
	ttl := 30 * day
	if year >= time.Now().Year()-1 {
		ttl = day // new shows still arrive
	}
	return cached(c, fmt.Sprintf("recordings:%s:%d", coll, year), ttl, func() ([]Recording, error) {
		var r searchReply[Recording]
		q := fmt.Sprintf("collection:%s AND year:%d", coll, year)
		if err := c.search(ctx, q, recordingFields, "date asc", 5000, &r); err != nil {
			return nil, err
		}
		var out []Recording
		for _, x := range r.Response.Docs {
			if ValidID(x.ID) && x.Day() != "" {
				out = append(out, x)
			}
		}
		return out, nil
	})
}

// Files lists a recording's files (/metadata/<id>/files: about 30 KB, where the whole of
// /metadata/<id> can pass 2 MB of reviews).
func (c *Client) Files(ctx context.Context, id string) ([]File, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("not a recording: %q", id)
	}
	return cached(c, "files:"+id, 30*day, func() ([]File, error) {
		var r struct {
			Result []File `json:"result"`
		}
		if err := c.getJSON(ctx, "/metadata/"+id+"/files", nil, &r); err != nil {
			return nil, err
		}
		if r.Result == nil {
			return nil, errors.New("no such recording on archive.org")
		}
		return r.Result, nil
	})
}

// DownloadURL is where a recording's file is fetched from (archive.org redirects it to a node).
func (c *Client) DownloadURL(id, file string) string {
	parts := strings.Split(file, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return c.Base + "/download/" + id + "/" + strings.Join(parts, "/")
}
