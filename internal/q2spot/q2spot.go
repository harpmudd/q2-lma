// Package q2spot is Q2SPOT/1, the line protocol Q2 Pod's card-app page speaks to an app over a Unix
// stream socket (q2-pod docs/librespot.md#browsing-protocol-q2spot1, patch/addon.c): one request
// per connection; the client writes it, shuts its write side and reads the reply to EOF.
package q2spot

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits from the protocol doc and from addon.c's buffers. The last three are not in the doc: the
// firmware cuts or drops what passes them without an error.
const (
	Version    = "Q2SPOT/1"
	MaxRequest = 8 << 10
	MaxReply   = 64 << 10
	MaxRows    = 50
	MaxText    = 255 // title, subtitle
	MaxMessage = 120
	MaxCursor  = 1 << 10
	Timeout    = 15 * time.Second // the firmware's deadline for a reply

	MaxOp    = 15  // lv_t.op[16], pg.aop[16]: a longer op is cut
	MaxArgs  = 399 // lv_t.args[400]: "\t" + escaped context; past it a press does nothing
	MaxDepth = 6   // ADDON_DEPTH levels, the oldest past the first dropped
)

// Statuses the firmware knows. Any status but OK shows only the message and Retry, never rows.
const (
	OK          = "ok"
	Offline     = "offline"
	Unavailable = "unavailable"
	Throttled   = "throttled"
	Unsupported = "unsupported"
	Bad         = "bad"
	Error       = "error"
)

// Reply messages the firmware takes as instructions on an ok list.
const (
	Refresh = "refresh" // ask again every 2 s while on screen; never pages
	Pages   = "pages"   // with a cursor: fetch the next pages and append them (500 rows at most)
)

var ErrMalformed = errors.New("q2spot: malformed")

// Escape writes %, tab, LF and CR as %25, %09, %0A and %0D. NUL, which the firmware takes as a
// malformed reply, is dropped.
func Escape(s string) string {
	if !strings.ContainsAny(s, "%\t\n\r\x00") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case 0:
		case '%', '\t', '\n', '\r':
			b.WriteByte('%')
			b.WriteByte("0123456789ABCDEF"[c>>4])
			b.WriteByte("0123456789ABCDEF"[c&15])
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c|32 >= 'a' && c|32 <= 'f':
		return int(c|32-'a') + 10
	}
	return -1
}

// Unescape reverses Escape. A % without two hex digits, or an escaped NUL, is malformed.
func Unescape(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) || unhex(s[i+1]) < 0 || unhex(s[i+2]) < 0 {
			return "", ErrMalformed
		}
		v := byte(unhex(s[i+1])<<4 | unhex(s[i+2]))
		if v == 0 {
			return "", ErrMalformed
		}
		b.WriteByte(v)
		i += 2
	}
	return b.String(), nil
}

// Cut shortens s to at most n bytes without splitting a UTF-8 character.
func Cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// ContextFits reports whether a row's context can be sent back by a list or action press.
func ContextFits(ctx string) bool { return 1+len(Escape(ctx)) <= MaxArgs }

type Request struct {
	ID   string
	Op   string
	Args []string
}

// Arg is the i-th argument, "" when there are fewer.
func (r Request) Arg(i int) string {
	if i < len(r.Args) {
		return r.Args[i]
	}
	return ""
}

// ParseRequest reads "Q2SPOT/1\n<id>\t<op>\t<arg>…\n".
func ParseRequest(b []byte) (Request, error) {
	s := string(b)
	if len(b) > MaxRequest || !strings.HasPrefix(s, Version+"\n") || strings.IndexByte(s, 0) >= 0 {
		return Request{}, ErrMalformed
	}
	line := strings.TrimSuffix(s[len(Version)+1:], "\n")
	if strings.ContainsAny(line, "\n\r") {
		return Request{}, ErrMalformed
	}
	f := strings.Split(line, "\t")
	if len(f) < 2 {
		return Request{}, ErrMalformed
	}
	for i := range f {
		u, err := Unescape(f[i])
		if err != nil {
			return Request{}, err
		}
		f[i] = u
	}
	return Request{ID: f[0], Op: f[1], Args: f[2:]}, nil
}

// EncodeRequest frames a request, as the firmware sends it (for tests and q2ctl).
func EncodeRequest(id, op string, args ...string) []byte {
	var b strings.Builder
	b.WriteString(Version + "\n" + Escape(id) + "\t" + Escape(op))
	for _, a := range args {
		b.WriteString("\t" + Escape(a))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// Row kinds a card app may send (addon.c go()).
const (
	KindList     = "list"     // opens level URI with Context as its argument
	KindAction   = "action"   // sends URI with Context; the reply's message is toasted
	KindSearch   = "search"   // the T9 box; OK sends op "search" with the term
	KindRemote   = "remote"   // the QR page
	KindLocal    = "local"    // plays a card file, or opens a card folder (URI ending in /)
	KindInfo     = "info"     // inert
	KindProgress = "progress" // inert bar, Position in permille; only with the caps word progress
)

// Row is one list line. On every kind but progress, Position is the op a held Play/Pause sends.
type Row struct {
	Kind, URI, Title, Subtitle, Context, Position string
}

type Reply struct {
	Status, Message, Cursor string
	Rows                    []Row
}

// Fail is a reply with no rows.
func Fail(status, message string) Reply { return Reply{Status: status, Message: message} }

// Encode frames the reply for request id. Rows past MaxRows or MaxReply are left out; the count
// kept is returned so a paged list can continue its cursor after the last row sent.
func (r Reply) Encode(id string) ([]byte, int) {
	if r.Status == "" {
		r.Status = OK
	}
	var b strings.Builder
	b.WriteString(Version + "\n")
	b.WriteString(Escape(id) + "\t" + Escape(r.Status) + "\t" + Escape(Cut(r.Message, MaxMessage)) + "\t" +
		Escape(r.Cursor) + "\n")
	kept := 0
	for _, row := range r.Rows {
		if kept == MaxRows {
			break
		}
		line := Escape(row.Kind) + "\t" + Escape(row.URI) + "\t" + Escape(Cut(row.Title, MaxText)) + "\t" +
			Escape(Cut(row.Subtitle, MaxText)) + "\t" + Escape(row.Context) + "\t" + Escape(row.Position) + "\n"
		if b.Len()+len(line) > MaxReply {
			break
		}
		b.WriteString(line)
		kept++
	}
	return []byte(b.String()), kept
}

// ParseReply reads a reply, as the firmware's spot_reply does: a 4-field header, then 6-field rows.
func ParseReply(b []byte) (id string, r Reply, err error) {
	s := string(b)
	if len(b) > MaxReply || !strings.HasPrefix(s, Version+"\n") || strings.IndexByte(s, 0) >= 0 {
		return "", Reply{}, ErrMalformed
	}
	lines := strings.Split(strings.TrimSuffix(s[len(Version)+1:], "\n"), "\n")
	field := func(line string, n int) ([]string, error) {
		f := strings.Split(line, "\t")
		if len(f) != n {
			return nil, ErrMalformed
		}
		for i := range f {
			u, err := Unescape(f[i])
			if err != nil {
				return nil, err
			}
			f[i] = u
		}
		return f, nil
	}
	h, err := field(lines[0], 4)
	if err != nil {
		return "", Reply{}, err
	}
	r = Reply{Status: h[1], Message: h[2], Cursor: h[3]}
	if len(lines)-1 > MaxRows {
		return "", Reply{}, ErrMalformed
	}
	for _, l := range lines[1:] {
		f, err := field(l, 6)
		if err != nil {
			return "", Reply{}, err
		}
		r.Rows = append(r.Rows, Row{f[0], f[1], f[2], f[3], f[4], f[5]})
	}
	return h[0], r, nil
}

// Handler answers one request.
type Handler func(Request) Reply

// Serve answers requests on a Unix stream socket at path until the listener fails. A stale socket
// file is replaced.
func Serve(path string, h Handler) error {
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go answer(c, h)
	}
}

func answer(c net.Conn, h Handler) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(Timeout))
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 4096)
	for len(buf) <= MaxRequest {
		n, err := c.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	req, err := ParseRequest(buf)
	var out []byte
	if err != nil {
		out, _ = Fail(Bad, "Bad request").Encode(firstField(buf))
	} else {
		out, _ = h(req).Encode(req.ID)
	}
	c.Write(out)
}

// firstField is a malformed request's id when it has a numeric one, so the client can match it.
func firstField(b []byte) string {
	s := strings.TrimPrefix(string(b), Version+"\n")
	if i := strings.IndexAny(s, "\t\n"); i >= 0 {
		s = s[:i]
	}
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return "0"
	}
	return s
}

// Ask sends one request to the socket at path and returns its reply, as the firmware does: write,
// shut the write side, read to EOF, within Timeout.
func Ask(path, id, op string, args ...string) (Reply, error) {
	buf, err := AskRaw(path, id, op, args...)
	if err != nil {
		return Reply{}, err
	}
	got, r, err := ParseReply(buf)
	if err == nil && got != id {
		err = errors.New("q2spot: reply for request " + got + ", not " + id)
	}
	return r, err
}

// AskRaw is Ask's reply as bytes, unparsed.
func AskRaw(path, id, op string, args ...string) ([]byte, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(Timeout))
	if _, err := c.Write(EncodeRequest(id, op, args...)); err != nil {
		return nil, err
	}
	if uc, ok := c.(*net.UnixConn); ok {
		uc.CloseWrite()
	}
	var buf []byte
	tmp := make([]byte, 4096)
	for len(buf) <= MaxReply {
		n, err := c.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
