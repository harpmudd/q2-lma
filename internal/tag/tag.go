// Package tag writes what the Q2 shows of a streamed song into its file as it downloads: title,
// artist, album, date, track and cover. Q2 Pod's Now Playing reads art embedded in the file (stock
// toolsGetAlbumCover → TagLib), never an image beside it, and Stream mode's hidden folders are
// never in the Library, so the file is the only place for it.
package tag

import (
	"bufio"
	"errors"
	"io"
)

// Meta is a song as its tags name it. Cover is a baseline JPEG, or nil.
type Meta struct {
	Title, Artist, Album, Date string
	Track, Tracks              int
	Cover                      []byte
}

// ErrFormat is a file this package can't retag; Copy then writes it unchanged.
var ErrFormat = errors.New("tag: unrecognised file")

// Write copies src to w with m's tags, as ext (".mp3" or ".ogg") wants them. A file it can't read
// as that format is copied unchanged, as playing it untagged beats not playing it.
func Write(w io.Writer, src io.Reader, ext string, m Meta) (int64, error) {
	br := bufio.NewReaderSize(src, 64<<10)
	switch ext {
	case ".mp3":
		return mp3(w, br, m)
	case ".ogg":
		return ogg(w, br, m)
	}
	return io.Copy(w, br)
}
