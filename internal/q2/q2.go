// Package q2 is what a card app shares with Q2 Pod's side (q2-pod patch/addon.c): files written
// whole by rename, the status file, the firmware's feature words, and a log that never keeps the
// card busy.
package q2

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// WriteFile writes data beside path and renames it into place, synced first, so a reader (the
// firmware, or the next run after a power cut) never sees half of it.
func WriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Caps is the firmware's card-app feature words, from /tmp/q2-addon.caps (rewritten at every
// Streaming scan). A missing file is older firmware: no words. Raw is the file as read, "" if none.
type Caps struct {
	Words   map[string]bool
	Raw     string
	Present bool
}

func ReadCaps(path string) Caps {
	c := Caps{Words: map[string]bool{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	c.Present, c.Raw = true, strings.TrimSpace(string(b))
	for _, w := range strings.Fields(c.Raw) {
		c.Words[w] = true
	}
	return c
}

func (c Caps) Has(word string) bool { return c.Words[word] }

// Log appends timestamped lines to a file, opening and closing it for each, so the card is never
// held open (USB storage needs to unmount it). Past Max bytes the file moves to <path>.1.
type Log struct {
	Path string
	Max  int64
	mu   sync.Mutex
}

func (l *Log) Printf(format string, a ...any) {
	line := time.Now().UTC().Format("2006-01-02 15:04:05 ") + fmt.Sprintf(format, a...)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if st, err := os.Stat(l.Path); err == nil && st.Size()+int64(len(line)) > l.Max {
		os.Rename(l.Path, l.Path+".1")
	}
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	f.WriteString(line)
	f.Close()
}

// Write lets the standard library's log package (and panics routed through it) use a Log.
func (l *Log) Write(p []byte) (int, error) {
	l.Printf("%s", p)
	return len(p), nil
}
