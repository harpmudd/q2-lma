// Package ia is the Internet Archive's Live Music Archive (the etree collection) as q2-lma uses
// it: artists (etree's member collections), their years and shows, a recording's files, and which
// recording of a date to play.
package ia

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Text is a metadata field that archive.org sends as a string, a list of strings (joined with
// "; ") or a number, depending on the item.
type Text string

func (t *Text) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = Text(s)
		return nil
	}
	var l []string
	if json.Unmarshal(b, &l) == nil {
		*t = Text(strings.Join(l, "; "))
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) == nil {
		*t = Text(n.String())
		return nil
	}
	*t = ""
	return nil
}

// Num is a numeric field sent as a number or a string; anything else is 0.
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	var f float64
	if json.Unmarshal(b, &f) == nil {
		*n = Num(f)
		return nil
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		f, _ = strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	*n = Num(f)
	return nil
}

// Artist is one of etree's member collections: a band.
type Artist struct {
	ID        string `json:"identifier"`
	Name      Text   `json:"title"`
	Downloads Num    `json:"downloads"`
}

// Recording is one item: a recording of one show.
type Recording struct {
	ID        string `json:"identifier"`
	Date      Text   `json:"date"`
	Source    Text   `json:"source"`
	Rating    Num    `json:"avg_rating"`
	Reviews   Num    `json:"num_reviews"`
	Downloads Num    `json:"downloads"`
	Venue     Text   `json:"venue"`
	Coverage  Text   `json:"coverage"`
	Title     Text   `json:"title"`
}

// Day is the recording's date as YYYY-MM-DD, "" when it has none.
func (r Recording) Day() string {
	d := string(r.Date)
	if len(d) >= 10 && d[4] == '-' && d[7] == '-' {
		return d[:10]
	}
	return ""
}

// File is one entry of /metadata/<id>/files.
type File struct {
	Name     string `json:"name"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	Original string `json:"original"`
	Title    Text   `json:"title"`
	Track    Text   `json:"track"`
	Length   Text   `json:"length"`
	Size     Num    `json:"size"`
}

// archive.org identifiers are letters, digits, '.', '_' and '-'; nothing else from the network or
// a request may name a collection, an item or a cache folder (no '/', no "..").
var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// ValidID reports whether s can be an identifier, and so a path element.
func ValidID(s string) bool { return idRE.MatchString(s) && !strings.Contains(s, "..") }
