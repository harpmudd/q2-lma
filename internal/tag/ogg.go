package tag

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image/jpeg"
	"io"
	"strconv"
	"strings"
)

// ogg rewrites an Ogg Vorbis file's comment header with m's tags and its cover as a
// METADATA_BLOCK_PICTURE (the Xiph convention TagLib reads), keeping every other comment but the
// ones it replaces. The comment and setup headers are laid out on new pages, and every later
// page gets its sequence number moved by the page count's change and its checksum again. Anything
// but a plain single-stream Vorbis file is copied unchanged.
func ogg(w io.Writer, src *bufio.Reader, m Meta) (int64, error) {
	var raw bytes.Buffer // the header pages as read, written back unchanged on any surprise
	unchanged := func() (int64, error) {
		k, err := w.Write(raw.Bytes())
		if err != nil {
			return int64(k), err
		}
		n, err := io.Copy(w, src)
		return int64(k) + n, err
	}
	first, err := readPage(src, &raw)
	if err != nil || first.flags&2 == 0 || len(first.packets) != 1 || !bytes.HasPrefix(first.packets[0], []byte("\x01vorbis")) {
		return unchanged()
	}
	// The comment and setup headers follow; the setup ends its page, and audio starts on a new one.
	var packets [][]byte
	var partial []byte
	headerPages := 0
	for len(packets) < 2 {
		p, err := readPage(src, &raw)
		if err != nil || p.serial != first.serial || p.granule != 0 {
			return unchanged()
		}
		headerPages++
		for i, pk := range p.packets {
			if i == 0 && p.flags&1 != 0 {
				pk = append(partial, pk...)
				partial = nil
			}
			if i == len(p.packets)-1 && !p.lastComplete {
				partial = append([]byte(nil), pk...)
				continue
			}
			packets = append(packets, pk)
		}
		if headerPages > 64 {
			return unchanged()
		}
	}
	if len(packets) != 2 || partial != nil || !bytes.HasPrefix(packets[0], []byte("\x03vorbis")) ||
		!bytes.HasPrefix(packets[1], []byte("\x05vorbis")) {
		return unchanged()
	}
	comment, ok := retag(packets[0], m)
	if !ok {
		return unchanged()
	}

	var total int64
	put := func(b []byte) error {
		k, err := w.Write(b)
		total += int64(k)
		return err
	}
	if err := put(first.bytes); err != nil {
		return total, err
	}
	pages := paginate([][]byte{comment, packets[1]}, first.serial, 1)
	for _, pg := range pages {
		if err := put(pg); err != nil {
			return total, err
		}
	}
	shift := int64(len(pages)) - int64(headerPages)
	for {
		var none bytes.Buffer
		p, err := readPage(src, &none)
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		b := p.bytes
		binary.LittleEndian.PutUint32(b[18:], uint32(int64(p.seq)+shift))
		binary.LittleEndian.PutUint32(b[22:], 0)
		binary.LittleEndian.PutUint32(b[22:], crc(b))
		if err := put(b); err != nil {
			return total, err
		}
	}
}

type page struct {
	flags        byte
	granule      int64
	serial, seq  uint32
	packets      [][]byte // the packet pieces on this page
	lastComplete bool     // the last piece ends its packet here
	bytes        []byte
}

// readPage reads one Ogg page. Every byte it reads goes to raw as read, a bad page's included, so
// a caller giving up can write back exactly what it consumed.
func readPage(r *bufio.Reader, raw *bytes.Buffer) (page, error) {
	read := func(b []byte) error {
		k, err := io.ReadFull(r, b)
		raw.Write(b[:k])
		if err == io.ErrUnexpectedEOF {
			return ErrFormat
		}
		return err
	}
	h := make([]byte, 27)
	if err := read(h); err != nil {
		return page{}, err
	}
	if string(h[:4]) != "OggS" || h[4] != 0 {
		return page{}, ErrFormat
	}
	lacing := make([]byte, h[26])
	if err := read(lacing); err != nil {
		return page{}, ErrFormat
	}
	n := 0
	for _, l := range lacing {
		n += int(l)
	}
	body := make([]byte, n)
	if err := read(body); err != nil {
		return page{}, ErrFormat
	}
	p := page{flags: h[5], granule: int64(binary.LittleEndian.Uint64(h[6:])),
		serial: binary.LittleEndian.Uint32(h[14:]), seq: binary.LittleEndian.Uint32(h[18:])}
	p.bytes = append(append(append([]byte(nil), h...), lacing...), body...)
	at, start := 0, 0
	for i, l := range lacing {
		at += int(l)
		if l < 255 {
			p.packets = append(p.packets, body[start:at])
			start = at
		}
		if i == len(lacing)-1 {
			p.lastComplete = l < 255
			if l == 255 {
				p.packets = append(p.packets, body[start:at])
			}
		}
	}
	return p, nil
}

// paginate lays packets out on header pages (granule 0) from sequence number seq: up to 255
// lacing values a page, a packet that doesn't fit continuing on the next, and the last packet
// ending its page, as Vorbis wants before the audio.
func paginate(packets [][]byte, serial, seq uint32) [][]byte {
	var lacing []byte
	var data []byte
	for _, p := range packets {
		n := len(p)
		for n >= 255 {
			lacing = append(lacing, 255)
			n -= 255
		}
		lacing = append(lacing, byte(n))
		data = append(data, p...)
	}
	var pages [][]byte
	continued := false
	for len(lacing) > 0 {
		k := min(len(lacing), 255)
		size := 0
		for _, l := range lacing[:k] {
			size += int(l)
		}
		h := make([]byte, 27, 27+k+size)
		copy(h, "OggS")
		if continued {
			h[5] = 1
		}
		binary.LittleEndian.PutUint32(h[14:], serial)
		binary.LittleEndian.PutUint32(h[18:], seq)
		h[26] = byte(k)
		pg := append(append(h, lacing[:k]...), data[:size]...)
		binary.LittleEndian.PutUint32(pg[22:], crc(pg))
		pages = append(pages, pg)
		continued = lacing[k-1] == 255
		lacing, data, seq = lacing[k:], data[size:], seq+1
	}
	return pages
}

var crcTable = func() (t [256]uint32) {
	for i := range t {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if r&0x80000000 != 0 {
				r = r<<1 ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return
}()

// crc is an Ogg page's checksum, its own field read as zero.
func crc(p []byte) uint32 {
	var c uint32
	for i, b := range p {
		if i >= 22 && i < 26 {
			b = 0
		}
		c = c<<8 ^ crcTable[byte(c>>24)^b]
	}
	return c
}

var replaced = map[string]bool{"TITLE": true, "ARTIST": true, "ALBUM": true, "ALBUMARTIST": true, "DATE": true,
	"TRACKNUMBER": true, "TRACKTOTAL": true, "METADATA_BLOCK_PICTURE": true, "COVERART": true}

// retag rebuilds a Vorbis comment header: its vendor and the comments m doesn't replace, then m's.
func retag(p []byte, m Meta) ([]byte, bool) {
	r := bytes.NewReader(p[7:])
	var vlen uint32
	if binary.Read(r, binary.LittleEndian, &vlen) != nil || int(vlen) > r.Len() {
		return nil, false
	}
	vendor := make([]byte, vlen)
	r.Read(vendor)
	var count uint32
	if binary.Read(r, binary.LittleEndian, &count) != nil {
		return nil, false
	}
	var keep []string
	for i := uint32(0); i < count; i++ {
		var l uint32
		if binary.Read(r, binary.LittleEndian, &l) != nil || int(l) > r.Len() {
			return nil, false
		}
		c := make([]byte, l)
		r.Read(c)
		k, _, _ := strings.Cut(string(c), "=")
		if !replaced[strings.ToUpper(k)] {
			keep = append(keep, string(c))
		}
	}
	add := func(k, v string) {
		if v != "" {
			keep = append(keep, k+"="+v)
		}
	}
	add("TITLE", m.Title)
	add("ARTIST", m.Artist)
	add("ALBUMARTIST", m.Artist)
	add("ALBUM", m.Album)
	add("DATE", m.Date)
	if m.Track > 0 {
		add("TRACKNUMBER", strconv.Itoa(m.Track))
	}
	if m.Tracks > 0 {
		add("TRACKTOTAL", strconv.Itoa(m.Tracks))
	}
	if len(m.Cover) > 0 {
		add("METADATA_BLOCK_PICTURE", base64.StdEncoding.EncodeToString(picture(m.Cover)))
	}
	var out bytes.Buffer
	out.WriteString("\x03vorbis")
	binary.Write(&out, binary.LittleEndian, vlen)
	out.Write(vendor)
	binary.Write(&out, binary.LittleEndian, uint32(len(keep)))
	for _, c := range keep {
		binary.Write(&out, binary.LittleEndian, uint32(len(c)))
		out.WriteString(c)
	}
	out.WriteByte(1) // framing bit
	return out.Bytes(), true
}

// picture is a FLAC picture block for a front cover (big-endian fields), as the Xiph convention
// stores in METADATA_BLOCK_PICTURE.
func picture(jpg []byte) []byte {
	w, h := 0, 0
	if c, err := jpeg.DecodeConfig(bytes.NewReader(jpg)); err == nil {
		w, h = c.Width, c.Height
	}
	var b bytes.Buffer
	for _, v := range []any{uint32(3), uint32(len("image/jpeg")), []byte("image/jpeg"), uint32(0),
		uint32(w), uint32(h), uint32(24), uint32(0), uint32(len(jpg)), jpg} {
		binary.Write(&b, binary.BigEndian, v)
	}
	return b.Bytes()
}
