package tag

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image/jpeg"
	"os"
	"strings"
	"testing"
)

var meta = Meta{Title: "Take A Step Back/Tuning", Artist: "Grateful Dead", Album: "1977-05-08 Barton Hall",
	Date: "1977-05-08", Track: 12, Tracks: 20}

func TestLogoIsAJPEG(t *testing.T) {
	c, err := jpeg.DecodeConfig(bytes.NewReader(Logo()))
	if err != nil || c.Width != 300 || c.Height != 300 {
		t.Fatalf("%v %+v", err, c)
	}
}

func TestMP3(t *testing.T) {
	in, err := os.ReadFile("testdata/head.mp3")
	if err != nil {
		t.Fatal(err)
	}
	m := meta
	m.Cover = Logo()
	var out bytes.Buffer
	if _, err := Write(&out, bytes.NewReader(in), ".mp3", m); err != nil {
		t.Fatal(err)
	}
	b := out.Bytes()
	if string(b[:3]) != "ID3" || b[3] != 4 {
		t.Fatalf("no v2.4 tag: % x", b[:10])
	}
	n := int(syncsafe(b[6:10])) + 10
	old := int(syncsafe(in[6:10])) + 10
	if !bytes.Equal(b[n:], in[old:]) {
		t.Error("audio changed")
	}
	tagBytes := b[10:n]
	for _, want := range []string{"TIT2", "Take A Step Back/Tuning", "TRCK", "12/20", "APIC", "image/jpeg"} {
		if !bytes.Contains(tagBytes, []byte(want)) {
			t.Errorf("tag lacks %q", want)
		}
	}
	if bytes.Count(b, []byte("ID3")) != 1 && bytes.Index(b[n:], []byte("ID3\x03")) == 0 {
		t.Error("old tag kept")
	}
}

func pages(t *testing.T, b []byte) []page {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(b))
	var ps []page
	for {
		var none bytes.Buffer
		p, err := readPage(r, &none)
		if err != nil {
			break
		}
		got := binary.LittleEndian.Uint32(p.bytes[22:])
		if got != crc(p.bytes) {
			t.Fatalf("page %d: bad checksum", len(ps))
		}
		ps = append(ps, p)
	}
	return ps
}

func TestOgg(t *testing.T) {
	in, err := os.ReadFile("testdata/head.ogg")
	if err != nil {
		t.Fatal(err)
	}
	before := pages(t, in)
	m := meta
	m.Cover = Logo()
	var out bytes.Buffer
	if _, err := Write(&out, bytes.NewReader(in), ".ogg", m); err != nil {
		t.Fatal(err)
	}
	after := pages(t, out.Bytes())
	if len(after) < len(before) {
		t.Fatalf("%d pages, was %d", len(after), len(before))
	}
	for i, p := range after {
		if p.seq != uint32(i) {
			t.Fatalf("page %d has sequence %d", i, p.seq)
		}
	}
	// the audio pages' bodies are the same, in the same order
	last := func(ps []page) []byte { return ps[len(ps)-1].bytes[27+int(ps[len(ps)-1].bytes[26]):] }
	if !bytes.Equal(last(before), last(after)) {
		t.Error("audio changed")
	}
	// the comment header, reassembled from the new pages
	var comment []byte
	for _, p := range after[1:] {
		for _, pk := range p.packets {
			comment = append(comment, pk...)
		}
		if bytes.Contains(comment, []byte("\x05vorbis")) {
			break
		}
	}
	for _, want := range []string{"TITLE=Take A Step Back/Tuning", "ALBUM=1977-05-08 Barton Hall",
		"TRACKNUMBER=12", "METADATA_BLOCK_PICTURE=", "comment=http://archive.org/details/"} {
		if !bytes.Contains(comment, []byte(want)) {
			t.Errorf("comments lack %q", want)
		}
	}
	if bytes.Contains(comment, []byte("title=Take")) {
		t.Error("old title kept")
	}
	i := bytes.Index(comment, []byte("METADATA_BLOCK_PICTURE="))
	enc := comment[i+len("METADATA_BLOCK_PICTURE="):]
	enc = enc[:bytes.IndexFunc(enc, func(r rune) bool {
		return !strings.ContainsRune(
			"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=", r)
	})]
	pic, err := base64.StdEncoding.DecodeString(string(enc))
	if err != nil || binary.BigEndian.Uint32(pic) != 3 {
		t.Fatalf("picture block: %v", err)
	}
}

func TestOggUnchangedWhenNotVorbis(t *testing.T) {
	in := []byte("not an ogg file at all")
	var out bytes.Buffer
	if _, err := Write(&out, bytes.NewReader(in), ".ogg", meta); err != nil || !bytes.Equal(out.Bytes(), in) {
		t.Errorf("%v %q", err, out.Bytes())
	}
}
