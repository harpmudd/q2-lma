// Package tlsroots is HTTPS for the Q2: the firmware has no CA bundle an app can use, so this
// embeds Mozilla's as curl publishes it (cacert.pem, MPL-2.0, https://curl.se/docs/caextract.html).
//
// The Q2's clock is often unset (1970 or 2000), and every certificate would read "not yet valid".
// Until it looks set, certificates are checked at the system time corrected by the Date header of
// a verified HTTPS reply (Sync), and never at a time before Floor, a date built into the binary: a
// network that forges the time can't make an expired certificate pass.
package tlsroots

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed cacert.pem
var bundle []byte

// Floor is the earliest time a certificate is checked at: about this code's date.
var Floor = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

var roots = sync.OnceValue(func() *x509.CertPool {
	p := x509.NewCertPool()
	p.AppendCertsFromPEM(bundle)
	return p
})

// Pool is the embedded roots.
func Pool() *x509.CertPool { return roots() }

var clock struct {
	sync.Mutex
	offset time.Duration
	synced bool
}

// Now is the time certificates are checked at: the system clock when it is past Floor, else the
// clock corrected by Sync, and never before Floor.
func Now() time.Time {
	now := time.Now()
	if now.After(Floor) {
		return now
	}
	clock.Lock()
	off := clock.offset
	clock.Unlock()
	if t := now.Add(off); t.After(Floor) {
		return t
	}
	return Floor
}

// Synced reports whether Now came from a server rather than the system clock or Floor.
func Synced() bool {
	clock.Lock()
	defer clock.Unlock()
	return clock.synced
}

// Sync reads the Date header of a HEAD to url (https), whose chain and host name are verified
// against the roots at the later of Floor and the leaf's own NotBefore (the date is what is being
// asked), and keeps it as an offset from the system clock. It returns the server's time.
func Sync(ctx context.Context, url string) (time.Time, error) {
	c := &http.Client{Timeout: 15 * time.Second, Transport: dateTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	resp.Body.Close()
	d, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return time.Time{}, err
	}
	clock.Lock()
	clock.offset, clock.synced = time.Until(d), true
	clock.Unlock()
	return d, nil
}

func dateTransport() *http.Transport {
	t := Transport()
	t.DisableKeepAlives = true
	t.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Go's own check runs at one fixed time; this verifies everything but the dates instead,
		// the documented pattern for custom verification.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("tlsroots: no certificate")
			}
			leaf := cs.PeerCertificates[0]
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			at := leaf.NotBefore.Add(time.Second)
			if at.Before(Floor) {
				at = Floor
			}
			_, err := leaf.Verify(x509.VerifyOptions{Roots: Pool(), Intermediates: inter,
				DNSName: cs.ServerName, CurrentTime: at})
			return err
		},
	}
	return t
}

// Transport verifies servers against the roots at Now, dials IPv4 only (the Q2's kernel has no
// IPv6) and bounds each step, not the body, so long downloads aren't cut.
func Transport() *http.Transport {
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp4", addr)
		},
		TLSClientConfig:       &tls.Config{RootCAs: Pool(), Time: Now, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConnsPerHost:   2,
		ForceAttemptHTTP2:     true,
	}
}
