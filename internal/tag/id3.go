package tag

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
)

// mp3 writes an ID3v2.4 tag for m (UTF-8 text, as q2-podcasts writes for the Q2), then src with its
// leading ID3v2 tags dropped. The audio, its LAME/Xing frame (which gapless playback reads) and any
// ID3v1 tag at the end pass through untouched.
func mp3(w io.Writer, src *bufio.Reader, m Meta) (int64, error) {
	for {
		h, err := src.Peek(10)
		if err != nil || string(h[:3]) != "ID3" || h[3] == 0xff || h[4] == 0xff {
			break
		}
		n := int64(syncsafe(h[6:10])) + 10
		if h[5]&0x10 != 0 { // a footer
			n += 10
		}
		if _, err := src.Discard(int(n)); err != nil {
			return 0, err
		}
	}
	tag := id3(m)
	k, err := w.Write(tag)
	if err != nil {
		return int64(k), err
	}
	n, err := io.Copy(w, src)
	return int64(k) + n, err
}

func syncsafe(b []byte) uint32 {
	return uint32(b[0]&0x7f)<<21 | uint32(b[1]&0x7f)<<14 | uint32(b[2]&0x7f)<<7 | uint32(b[3]&0x7f)
}

func putSyncsafe(b []byte, n int) {
	b[0], b[1], b[2], b[3] = byte(n>>21&0x7f), byte(n>>14&0x7f), byte(n>>7&0x7f), byte(n&0x7f)
}

func id3(m Meta) []byte {
	var body bytes.Buffer
	frame := func(id string, data []byte) {
		h := make([]byte, 10)
		copy(h, id)
		putSyncsafe(h[4:8], len(data))
		body.Write(h)
		body.Write(data)
	}
	text := func(id, s string) {
		if s != "" {
			frame(id, append([]byte{3}, s...)) // 3: UTF-8
		}
	}
	text("TIT2", m.Title)
	text("TPE1", m.Artist)
	text("TPE2", m.Artist)
	text("TALB", m.Album)
	text("TDRC", m.Date)
	if m.Track > 0 {
		t := strconv.Itoa(m.Track)
		if m.Tracks > 0 {
			t += "/" + strconv.Itoa(m.Tracks)
		}
		text("TRCK", t)
	}
	if len(m.Cover) > 0 {
		apic := append([]byte{3}, "image/jpeg\x00"...)
		apic = append(apic, 3, 0) // front cover, empty description
		frame("APIC", append(apic, m.Cover...))
	}
	out := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	putSyncsafe(out[6:10], body.Len())
	return append(out, body.Bytes()...)
}
