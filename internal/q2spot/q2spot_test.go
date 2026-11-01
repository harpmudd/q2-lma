package q2spot

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEscapeRoundTrip(t *testing.T) {
	for _, s := range []string{"", "plain", "a\tb\nc\rd%e", "Dancin' → Streets", "100%"} {
		u, err := Unescape(Escape(s))
		if err != nil || u != s {
			t.Errorf("%q: got %q, %v", s, u, err)
		}
	}
	if Escape("a\x00b") != "ab" {
		t.Error("NUL not dropped")
	}
	for _, bad := range []string{"%", "%4", "%zz", "%00"} {
		if _, err := Unescape(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestCut(t *testing.T) {
	if got := Cut("héllo", 2); got != "h" {
		t.Errorf("cut inside a character: %q", got)
	}
	if got := Cut("abc", 5); got != "abc" {
		t.Errorf("%q", got)
	}
}

func TestEncodeLimits(t *testing.T) {
	var rows []Row
	for i := 0; i < 60; i++ {
		rows = append(rows, Row{Kind: KindInfo, Title: strings.Repeat("x", 400)})
	}
	b, kept := Reply{Message: strings.Repeat("m", 200), Rows: rows}.Encode("7")
	if kept != MaxRows {
		t.Errorf("kept %d rows", kept)
	}
	_, r, err := ParseReply(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Message) != MaxMessage || len(r.Rows[0].Title) != MaxText {
		t.Errorf("message %d, title %d", len(r.Message), len(r.Rows[0].Title))
	}

	big := Row{Kind: KindInfo, Title: strings.Repeat("t", 255), Subtitle: strings.Repeat("s", 255),
		URI: strings.Repeat("u", 2000)}
	b, kept = Reply{Rows: []Row{big, big, big, big, big, big, big, big, big, big, big, big, big, big,
		big, big, big, big, big, big, big, big, big, big, big, big, big, big, big, big}}.Encode("1")
	if len(b) > MaxReply || kept == 30 {
		t.Errorf("%d bytes, kept %d", len(b), kept)
	}
}

func TestContextFits(t *testing.T) {
	if !ContextFits(strings.Repeat("a", 398)) || ContextFits(strings.Repeat("a", 399)) {
		t.Error("plain bound")
	}
	if ContextFits(strings.Repeat("\t", 133)) { // 1 + 133*3 = 400
		t.Error("escaped bound")
	}
}

func TestServeAndAsk(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")
	go Serve(sock, func(r Request) Reply {
		return Reply{Message: r.Op, Rows: []Row{{Kind: KindInfo, Title: r.Arg(0)}}}
	})
	var (
		r   Reply
		err error
	)
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if r, err = Ask(sock, "42", "menu", "a\tb"); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != OK || r.Message != "menu" || len(r.Rows) != 1 || r.Rows[0].Title != "a\tb" {
		t.Errorf("%+v", r)
	}
}
