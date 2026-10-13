package tui

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	upd "github.com/n-orlov/deck/internal/update"
)

// ctrlUEchoMirror is a loopback mirror answering the latest lookup with
// latest and every download with download, both raw HTTP bytes, and returns
// a model whose ctrl+u installs from it with query appended to its URL.
func ctrlUEchoMirror(t *testing.T, query, latest, download string) (Model, string, string) {
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
				if strings.Contains(req.RequestURI, "/releases/latest") {
					_, _ = c.Write([]byte(latest))
					return
				}
				_, _ = c.Write([]byte(download))
			}()
		}
	}()
	t.Setenv("DECK_UPDATE_URL", "http://"+ln.Addr().String()+query)
	t.Setenv("PATH", t.TempDir())
	exe := filepath.Join(t.TempDir(), "deck")
	if err := os.WriteFile(exe, []byte(ctrlUOldBody), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := upd.Options{Exe: exe, Current: ctrlUCurrent, OnlyIfNewer: true}
	m := footerNoteFrameModel(240, 24, false)
	m = m.WithUpdateInstaller(ctrlUCurrent, func(ctx context.Context) (upd.Result, error) { return upd.Run(ctx, opts) })
	m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	return m, exe, ctrlUSum([]byte(ctrlUOldBody))
}

// TestCtrlUFailureNeverShowsServerEchoedCredential: a mirror whose reply
// echoes the short credential deck sent it -- in a download's HTTP reason
// phrase, in the latest redirect's path, or in a malformed status line --
// leaves no trace of it in the footer note or the frame after a failed
// ctrl+u. The note still names the status code or cause and the host where
// the cause quotes the URL, and the binary is untouched.
func TestCtrlUFailureNeverShowsServerEchoedCredential(t *testing.T) {
	raw := func(status string, hdr ...string) string {
		return "HTTP/1.1 " + status + "\r\n" + strings.Join(append(hdr, "Content-Length: 0", "Connection: close"), "\r\n") + "\r\n\r\n"
	}
	found := raw("302 Found", "Location: /releases/tag/"+indicatorTag)
	for _, cred := range []struct{ name, query, value string }{
		{"angle", "?access=Q>7", "Q>7"},
		{"one byte", "?t=Z", "Z"},
		{"quote", "?t=K\"Q", "K\"Q"},
		{"second pair", "?a=1&t=K>Z&b=2", "K>Z"},
	} {
		v := cred.value
		for name, row := range map[string]struct {
			latest, download, want string
			host                   bool
		}{
			"reason phrase":    {found, raw("503 access denied for credential " + v), "503 access denied for credential ***", true},
			"encoded reason":   {found, raw("503 bad token " + url.QueryEscape(v)), "503 bad token ***", true},
			"redirect path":    {raw("302 Found", "Location: /releases/tag/"+url.PathEscape(v)), "", "redirect /releases/tag/*** does not name", false},
			"malformed status": {found, raw(v + " nope"), "malformed HTTP", true},
			"latest reason":    {raw("403 no release for " + v), "", "403 no release for ***", false},
		} {
			t.Run(cred.name+"/"+name, func(t *testing.T) {
				m, exe, oldSum := ctrlUEchoMirror(t, cred.query, row.latest, row.download)
				m, _ = ctrlUPress(t, m, ctrlUKey)
				m, cmd := ctrlUPress(t, m, key("enter"))
				if cmd == nil {
					t.Fatal("confirming returned no command")
				}
				m = sendUpdateMsg(t, m, cmd())
				note, frame := m.footerNote(), m.View()
				if strings.ContainsAny(strings.ReplaceAll(note, "vX.Y.Z", ""), "KQZ") {
					t.Fatalf("the echoed credential reached the UI note %q", note)
				}
				if len(v) > 1 && strings.Contains(frame, v) {
					t.Fatalf("the echoed credential %q reached the frame", v)
				}
				if !strings.HasPrefix(note, "update failed") || !strings.Contains(note, row.want) {
					t.Fatalf("note %q lost its non-secret diagnostics (want %q)", note, row.want)
				}
				if row.host && !strings.Contains(note, "127.0.0.1") {
					t.Fatalf("note %q lost the host", note)
				}
				if got := ctrlUFileSum(t, exe); got != oldSum {
					t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
				}
			})
		}
	}
}
