package main

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// updEchoServer is a loopback release server that answers every request with
// the raw HTTP bytes reply returns, so it can send a reason phrase or a
// status line net/http's own server would never write.
func updEchoServer(t *testing.T, reply string) string {
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
				if _, err := http.ReadRequest(bufio.NewReader(c)); err == nil {
					_, _ = c.Write([]byte(reply))
				}
			}()
		}
	}()
	return "http://" + ln.Addr().String()
}

// TestUpdateVerbScrubsServerEchoedCredentials: a mirror whose reply echoes
// the short credential deck sent it (in an HTTP reason phrase, percent-
// encoded, in the latest redirect's path, in a malformed status line, or in a
// malformed Location header on any status, which a parse error quotes)
// never gets that credential onto stdout or stderr of the real binary, for
// `deck update --version`, `deck update` and `deck update --check`. The
// failure still names the status code and the server's non-secret words, and
// the installed binary and its directory are untouched.
func TestUpdateVerbScrubsServerEchoedCredentials(t *testing.T) {
	bins := updBinaries{}
	v100 := bins.build(t, "v1.0.0", "-X main.version=v1.0.0")
	emptyPath := t.TempDir()
	reply := func(status string, hdr ...string) string {
		return "HTTP/1.1 " + status + "\r\n" + strings.Join(append(hdr, "Content-Length: 0", "Connection: close"), "\r\n") + "\r\n\r\n"
	}
	type row struct {
		name, query, reply, want string
		args                     []string
	}
	var rows []row
	for _, cred := range []struct{ name, query, value string }{
		{"angle", "?access=Q>7", "Q>7"},
		{"one byte", "?t=Z", "Z"},
		{"quote", "?t=K\"Q", "K\"Q"},
		{"second pair", "?a=1&t=K>Z&b=2", "K>Z"},
		{"ticket", "?ticket=R>", "R>"},
		{"three bytes", "?t=Q>Z", "Q>Z"},
	} {
		v := cred.value
		rows = append(rows,
			row{cred.name + " reason pinned", cred.query, reply("503 access denied for credential " + v), "503 access denied for credential ***", []string{"update", "--version", "v8.4.1"}},
			row{cred.name + " reason latest", cred.query, reply("401 bad token " + v), "401 bad token ***", []string{"update"}},
			row{cred.name + " encoded reason check", cred.query, reply("401 bad token " + url.QueryEscape(v)), "401 bad token ***", []string{"update", "--check"}},
			row{cred.name + " redirect path", cred.query, reply("302 Found", "Location: /releases/tag/"+url.PathEscape(v)), "redirect /releases/tag/*** does not name", []string{"update"}},
			row{cred.name + " redirect path check", cred.query, reply("302 Found", "Location: /releases/tag/"+url.PathEscape(v)), "redirect /releases/tag/*** does not name", []string{"update", "--check"}},
			row{cred.name + " malformed status", cred.query, reply(v + " nope"), "malformed HTTP", []string{"update", "--version", "v8.4.1"}},
			row{cred.name + " bad location 503", cred.query, reply("503 Service Unavailable", "Location: %"+v), "503 Service Unavailable without a redirect: parse", []string{"update"}},
			row{cred.name + " bad location 503 check", cred.query, reply("503 Service Unavailable", "Location: %"+v), "503 Service Unavailable without a redirect: parse", []string{"update", "--check"}},
			row{cred.name + " bad location 200 check", cred.query, reply("200 Fine", "Location: %"+v), "200 Fine without a redirect: parse", []string{"update", "--check"}},
			row{cred.name + " bad redirect pinned", cred.query, reply("302 Found", "Location: /x/%"+v), "failed to parse Location header", []string{"update", "--version", "v8.4.1"}},
		)
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "deck")
			copyFile(t, v100, exe)
			before := updSum(t, exe)
			base := updEchoServer(t, r.reply)
			stdout, stderr, code := updRun(t, exe, t.TempDir(), emptyPath, []string{"DECK_UPDATE_URL=" + base + r.query}, r.args...)
			if code == 0 || !strings.HasPrefix(stderr, "deck update: ") || !strings.Contains(stderr, r.want) {
				t.Fatalf("exit %d stderr %q, want a failure naming %q", code, stderr, r.want)
			}
			if out := strings.NewReplacer("vX.Y.Z", "", "URL", "").Replace(stdout + stderr); strings.ContainsAny(out, "KQRZ") {
				t.Fatalf("the echoed credential reached the output: stdout %q stderr %q", stdout, stderr)
			}
			if strings.Contains(r.name, "pinned") && !strings.Contains(stderr, "127.0.0.1") {
				t.Fatalf("stderr %q lost the host", stderr)
			}
			if updSum(t, exe) != before || len(updNames(t, dir)) != 1 {
				t.Errorf("binary or directory changed: %v", updNames(t, dir))
			}
		})
	}
}
