package ia

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"q2-lma/internal/q2"
)

// Cache keeps decoded answers on the card, one JSON file per request, so browsing is quick and
// works on a weak connection. A stale entry is still used when the network fails. The folder is
// held under Max bytes, oldest first.
type Cache struct {
	Dir string
	Max int64
	mu  sync.Mutex
}

func (c *Cache) path(key string) string {
	h := sha1.Sum([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(h[:10])+".json")
}

// Get decodes key's entry into v: found when there is one, fresh when it is younger than ttl.
func (c *Cache) Get(key string, ttl time.Duration, v any) (found, fresh bool) {
	if c == nil {
		return false, false
	}
	p := c.path(key)
	st, err := os.Stat(p)
	if err != nil {
		return false, false
	}
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, v) != nil {
		return false, false
	}
	return true, time.Since(st.ModTime()) < ttl
}

// Put stores v under key, then trims the folder.
func (c *Cache) Put(key string, v any) {
	if c == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if os.MkdirAll(c.Dir, 0o755) != nil || q2.WriteFile(c.path(key), b) != nil {
		return
	}
	c.trim()
}

func (c *Cache) trim() {
	ents, _ := os.ReadDir(c.Dir)
	type f struct {
		p string
		t time.Time
		n int64
	}
	var fs []f
	var total int64
	for _, e := range ents {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			fs = append(fs, f{filepath.Join(c.Dir, e.Name()), info.ModTime(), info.Size()})
			total += info.Size()
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t.Before(fs[j].t) })
	for _, x := range fs {
		if total <= c.Max {
			break
		}
		if os.Remove(x.p) == nil {
			total -= x.n
		}
	}
}

// Clear removes every entry.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	os.RemoveAll(c.Dir)
}
