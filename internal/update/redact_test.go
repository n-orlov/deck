package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sentinel = "unit-mirror-password-sentinel"

func credURL(srv *httptest.Server) string {
	return strings.Replace(srv.URL, "http://", "http://mirror-user:"+sentinel+"@", 1)
}

func TestRedactText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"GET http://u:" + sentinel + "@127.0.0.1:80/x: 500", "GET http://***@127.0.0.1:80/x: 500"},
		{"https://" + sentinel + "@host/a", "https://***@host/a"}, // a bare token user
		{"https://host/a?token=" + sentinel + "&x=1", "https://host/a?token=***&x=***"},
		{`Get "ftp://a:b@h/p": refused`, `Get "ftp://***@h/p": refused`},
		{"no url here, 5 @ 3", "no url here, 5 @ 3"},
		{"https://github.com/n-orlov/deck/releases", "https://github.com/n-orlov/deck/releases"},
	} {
		if got := RedactText(c.in); got != c.want {
			t.Errorf("RedactText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRedactErrKeepsCause: the redacted error still unwraps to its cause.
func TestRedactErrKeepsCause(t *testing.T) {
	r := &Refusal{Reason: RefusedDev, Message: "see http://u:" + sentinel + "@h/"}
	err := redactErr(r)
	if strings.Contains(err.Error(), sentinel) || !IsRefusal(err) {
		t.Errorf("redactErr = %q, IsRefusal %v", err, IsRefusal(err))
	}
	plain := errors.New("plain")
	if !errors.Is(redactErr(plain), plain) || redactErr(nil) != nil {
		t.Error("a clean error was wrapped")
	}
}

// TestGetErrorsRedactURLCredentials covers the three failures get formats
// itself: HTTP status, body read and size limit.
func TestGetErrorsRedactURLCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/500":
			http.Error(w, "x", http.StatusInternalServerError)
		case "/short":
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("abc"))
		default:
			_, _ = w.Write([]byte("0123456789"))
		}
	}))
	defer srv.Close()
	base := credURL(srv)
	for path, want := range map[string]string{"/500": "500 Internal Server Error", "/short": "unexpected EOF", "/big": "larger than 5 bytes"} {
		_, err := get(context.Background(), newClient(), base+path, 5)
		if err == nil || strings.Contains(err.Error(), sentinel) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("get %s: err = %v, want %q, the host and no password", path, err, want)
		}
	}
}

// TestRunAndCheckRedactEveryFailure drives the exported entry points against
// a credentialed mirror that fails in each way; the old binary stays put.
func TestRunAndCheckRedactEveryFailure(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := credURL(dead)
	dead.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/"+tagV, http.StatusFound)
			return
		}
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	exe := filepath.Join(t.TempDir(), "deck")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	for name, base := range map[string]string{
		"status":      credURL(srv),
		"unreachable": deadURL,
		"scheme":      "ftp://u:" + sentinel + "@mirror.example/x",
		"https-host":  "https://u:" + sentinel + "@127.0.0.1:1",
		"unparsable":  "http://u:" + sentinel + "@[::1/x",
	} {
		_, err := Run(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v0.0.1"})
		if err == nil || strings.Contains(err.Error(), sentinel) {
			t.Errorf("%s: Run err = %v", name, err)
		}
		_, err = Check(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v0.0.1"})
		if err != nil && strings.Contains(err.Error(), sentinel) || err == nil && name != "status" {
			t.Errorf("%s: Check err = %v", name, err)
		}
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Errorf("binary = %q after failed updates", b)
	}
}
