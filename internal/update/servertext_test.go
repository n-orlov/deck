package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// echoServer is a loopback server that answers every request with the raw
// HTTP bytes reply(requestURI) returns, so a test can send a status line, a
// reason phrase or a header Go's own server would never write.
func echoServer(t *testing.T, reply func(uri string) string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				req, err := http.ReadRequest(bufio.NewReader(c))
				if err != nil {
					return
				}
				_, _ = c.Write([]byte(reply(req.RequestURI)))
			}()
		}
	}()
	return "http://" + ln.Addr().String()
}

// serverEchoSecrets are configured credentials shorter than minKnownSecret
// (or holding a delimiter byte), with the value a server echoes back. (A
// space cannot be echoed: it never reaches the server in a request line.) Every
// byte of each value is one of K, Q, W, Z or a delimiter, and no diagnostic
// of a failed update holds an upper-case K, Q, W or Z.
var serverEchoSecrets = []struct{ name, cfg, value string }{
	{"query angle", "?access=Q>7", "Q>7"},
	{"query quote", "?t=K\"Z", "K\"Z"},
	{"query one byte", "?t=Z", "Z"},
	{"query second pair", "?a=1&t=K>Z&b=2", "K>Z"},
	{"query pre-encoded", "?t=Q%3EW", "Q>W"},
	{"userinfo two bytes", "m:QW@", "QW"},
}

// serverEchoes are the places a release server can quote what it was sent:
// each builds the raw reply to every request (latest lookup and downloads)
// for one echoed value, and names the diagnostic that must survive.
var serverEchoes = []struct {
	name    string
	pinned  bool   // run with Version set (download) rather than a latest lookup
	noSpace bool   // the echo only quotes a value up to its first space
	want    string // non-secret diagnostic that must survive
	reply   func(v, uri string) string
}{
	{"reason phrase on a download", true, false, "503 access denied for credential ***", func(v, _ string) string {
		return "HTTP/1.1 503 access denied for credential " + v + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	}},
	{"reason phrase on the latest lookup", false, false, "403 no redirect for ***", func(v, _ string) string {
		return "HTTP/1.1 403 no redirect for " + v + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	}},
	{"percent-encoded reason phrase", true, false, "502 bad token ***", func(v, _ string) string {
		return "HTTP/1.1 502 bad token " + url.QueryEscape(v) + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	}},
	{"redirect path", false, false, "redirect /releases/tag/*** does not name a stable", func(v, _ string) string {
		return "HTTP/1.1 302 Found\r\nLocation: /releases/tag/" + url.PathEscape(v) + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	}},
	{"malformed status line", true, true, "malformed HTTP", func(v, _ string) string {
		return "HTTP/1.1 " + v + " nope\r\nConnection: close\r\n\r\n"
	}},
}

// TestRunAndCheckScrubServerEchoedSecrets: a release server that echoes the
// configured credential back -- in a status reason phrase, percent-encoded,
// in a redirect path, or in a malformed status line a transport error quotes
// -- never gets that credential, however short, into Run's or Check's error.
// The error still names the host and the server's non-secret diagnostic, and
// the binary and its directory are untouched.
func TestRunAndCheckScrubServerEchoedSecrets(t *testing.T) {
	dir, exe := installOld(t)
	t.Setenv("PATH", t.TempDir())
	for _, s := range serverEchoSecrets {
		for _, e := range serverEchoes {
			if e.noSpace && strings.Contains(s.value, " ") {
				continue
			}
			t.Run(s.name+"/"+e.name, func(t *testing.T) {
				base := breakingURL(echoServer(t, func(uri string) string { return e.reply(s.value, uri) }), s.cfg)
				o := Options{BaseURL: base, Exe: exe, Current: "v0.0.1"}
				if e.pinned {
					o.Version = tagV
				}
				_, err := Run(context.Background(), o)
				assertScrubbed(t, "Run", err, e.want, e.pinned)
				if !e.pinned {
					_, err = Check(context.Background(), o)
					assertScrubbed(t, "Check", err, e.want, false)
				}
			})
		}
	}
	if b, _ := os.ReadFile(exe); string(b) != oldBody {
		t.Errorf("binary = %q after failed updates", b)
	}
	if got := listing(t, dir); len(got) != 1 {
		t.Errorf("directory = %v after failed updates", got)
	}
}

func assertScrubbed(t *testing.T, what string, err error, want string, host bool) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s succeeded against an echoing server", what)
	}
	msg := err.Error()
	if strings.ContainsAny(strings.ReplaceAll(msg, "vX.Y.Z", ""), "KQWZ") {
		t.Errorf("%s error carries the echoed credential: %q", what, msg)
	}
	if !strings.Contains(msg, want) {
		t.Errorf("%s error %q lost the diagnostic %q", what, msg, want)
	}
	if host && !strings.Contains(msg, "127.0.0.1") {
		t.Errorf("%s error %q lost the host", what, msg)
	}
}

// TestChecksumFieldEchoIsScrubbed: a checksums.txt whose hash field echoes
// the credential is reported as a mismatch without the credential.
func TestChecksumFieldEchoIsScrubbed(t *testing.T) {
	dir, exe := installOld(t)
	t.Setenv("PATH", t.TempDir())
	rel := newRelease(t)
	archive := rel.archive
	reply := func(uri string) string {
		body := "QW  " + rel.asset + "\n"
		if strings.Contains(uri, rel.asset) {
			body = string(archive)
		}
		return "HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\nConnection: close\r\n\r\n" + body
	}
	base := echoServer(t, reply) + "/m?t=QW"
	_, err := Run(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v0.0.1", Version: tagV})
	assertScrubbed(t, "Run", err, "checksum mismatch for "+rel.asset+": checksums.txt says ***", false)
	if b, _ := os.ReadFile(exe); string(b) != oldBody || len(listing(t, dir)) != 1 {
		t.Errorf("binary %q / directory %v changed", b, listing(t, dir))
	}
}

// TestScrubServerTextKeepsTheConfiguredHost: a one-byte secret that also
// occurs inside the configured host and port is masked where the server put
// it and kept inside the host, so the dial address stays readable.
func TestScrubServerTextKeepsTheConfiguredHost(t *testing.T) {
	cfg := "http://127.0.0.1:4477?t=7"
	in := `Get "http://127.0.0.1:4477?t=7/releases/latest": malformed HTTP status code "7"; dial 127.0.0.1:4477 said 7`
	got := scrubServerText(in, cfg)
	want := `Get "http://127.0.0.1:4477?t=***/releases/latest": malformed HTTP status code "***"; dial 127.0.0.1:4477 said ***`
	if got != want {
		t.Errorf("scrubServerText =\n %q\nwant\n %q", got, want)
	}
}

// TestRedactErrScrubsOnlyServerText: deck's own wording around server text is
// never rewritten for a short secret; the server text inside it is, and the
// cause stays reachable.
func TestRedactErrScrubsOnlyServerText(t *testing.T) {
	cfg := "https://mirror.example/r?t=x"
	cause := fromServer(os.ErrDeadlineExceeded)
	err := redactErr(fmt.Errorf("extract deck: %w (server said %w)", cause, serverString("bad x")), cfg)
	if got, want := err.Error(), "extract deck: i/o timeout (server said bad ***)"; got != want {
		t.Errorf("redactErr = %q, want %q", got, want)
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Error("the cause is no longer reachable")
	}
}
