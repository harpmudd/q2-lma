// Package mine is My Artists: each user's own list, kept on the card as a text file they can also
// edit on a computer (.lma/my-artists.txt). One archive.org collection identifier a line, an
// optional name after a tab or spaces, '#' for comments. Adding or removing on the Q2 rewrites the
// file whole, keeping its comments and order.
package mine

import (
	"os"
	"strings"
	"sync"

	"q2-lma/internal/ia"
	"q2-lma/internal/q2"
)

const header = "# My Artists for q2-lma (Live Music Archive): one archive.org collection a line,\n" +
	"# its name after a tab. The collection is the end of the artist's address on archive.org:\n" +
	"# https://archive.org/details/GratefulDead -> GratefulDead. Or hold Play/Pause on an artist on the Q2.\n"

// Entry is one artist line. A line that isn't a valid identifier is kept as text and shown as
// not found, so a typo is visible on the Q2.
type Entry struct {
	ID, Name string
}

type List struct {
	Path  string
	mu    sync.Mutex
	lines []string // the file as read: comments, blanks and entries in order
}

// Load reads the file; a missing one is an empty list.
func Load(path string) *List {
	l := &List{Path: path}
	if b, err := os.ReadFile(path); err == nil {
		s := strings.ReplaceAll(strings.TrimPrefix(string(b), "\xef\xbb\xbf"), "\r\n", "\n") // a BOM
		l.lines = strings.Split(strings.TrimRight(s, "\n"), "\n")
	}
	return l
}

func parse(line string) (Entry, bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return Entry{}, false
	}
	id, name, _ := strings.Cut(s, "\t")
	if f := strings.Fields(id); len(f) > 0 && name == "" {
		id, name = f[0], strings.TrimSpace(strings.TrimPrefix(s, f[0]))
	}
	return Entry{strings.TrimSpace(id), strings.TrimSpace(name)}, true
}

// Entries is the list in file order.
func (l *List) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for _, line := range l.lines {
		if e, ok := parse(line); ok {
			out = append(out, e)
		}
	}
	return out
}

// Has reports whether id is in the list.
func (l *List) Has(id string) bool {
	for _, e := range l.Entries() {
		if e.ID == id {
			return true
		}
	}
	return false
}

// Toggle adds id (named name) at the end, or removes it, and saves; added reports which.
func (l *List) Toggle(id, name string) (added bool, err error) {
	if !ia.ValidID(id) {
		return false, os.ErrInvalid
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.lines[:0:0]
	for _, line := range l.lines {
		if e, ok := parse(line); ok && e.ID == id {
			continue
		}
		kept = append(kept, line)
	}
	added = len(kept) == len(l.lines)
	if added {
		if len(kept) == 0 {
			kept = append(kept, strings.Split(strings.TrimRight(header, "\n"), "\n")...)
		}
		kept = append(kept, id+"\t"+strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(name))
	}
	if err := q2.WriteFile(l.Path, []byte(strings.Join(kept, "\n")+"\n")); err != nil {
		return false, err
	}
	l.lines = kept
	return added, nil
}
