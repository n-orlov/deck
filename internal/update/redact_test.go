package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	err := redactErr(r, "")
	if strings.Contains(err.Error(), sentinel) || !IsRefusal(err) {
		t.Errorf("redactErr = %q, IsRefusal %v", err, IsRefusal(err))
	}
	plain := errors.New("plain")
	if !errors.Is(redactErr(plain, "https://u:pw-long@h/"), plain) || redactErr(nil, "") != nil {
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

// oddPasswords are valid (or merely tolerated) URL passwords whose bytes a
// "safe character" regex misses: Go's url.Parse takes the userinfo up to the
// LAST '@' and accepts ' ( ) ! $ & * + , ; = ~ % in it. Each one holds the word
// "sentinel" in every piece a partial match could leave behind.
var oddPasswords = map[string]string{
	"apostrophe":  "independent'sentinel-password",
	"at sign":     "sentinel-left@sentinel-right",
	"sub-delims":  "sentinel(!$&*+,;=~)sentinel",
	"percent":     "sentinel%27quoted%40sentinel",
	"apostrophe@": "it's-sentinel@'sentinel'",
}

// TestRedactTextFollowsURLParse: the userinfo masked in free text is exactly
// what url.Parse would take as userinfo, whatever bytes the password holds,
// and the host after it survives.
func TestRedactTextFollowsURLParse(t *testing.T) {
	for name, pw := range oddPasswords {
		raw := "http://mirror:" + pw + "@127.0.0.1:8080/releases/latest"
		u, err := url.Parse(raw)
		if err != nil || u.Host != "127.0.0.1:8080" {
			t.Fatalf("%s: url.Parse(%q) = %v, %v; the row is not a valid URL", name, raw, u, err)
		}
		for _, msg := range []string{
			"GET " + raw + ": 503 Service Unavailable",
			`Get "` + raw + `": dial tcp: connection refused`,
			"GET " + raw + "?token=x: 500",
		} {
			got := RedactText(msg)
			if strings.Contains(got, "sentinel") || !strings.Contains(got, "http://***@127.0.0.1:8080/releases/latest") {
				t.Errorf("%s: RedactText(%q) = %q", name, msg, got)
			}
		}
	}
	for _, c := range []struct{ in, want string }{
		{"GET https://h/x?token=sentinel'quoted&a=1: 500", "GET https://h/x?token=***&a=*** 500"},
		{"GET https://h/x?k=sentinel(1)!$*: 500", "GET https://h/x?k=*** 500"},
		{"see http://h/a and mail me@example.com", "see http://h/a and mail me@example.com"},
	} {
		if got := RedactText(c.in); got != c.want {
			t.Errorf("RedactText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRedactURLUnparsable: a configured URL url.Parse refuses (a space, a '/'
// or a control byte in the password, no "://") is still masked up to its
// last '@', and the host stays.
func TestRedactURLUnparsable(t *testing.T) {
	for _, raw := range []string{
		"http://u:sentinel pass@mirror.example/x",
		"http://u:sentinel/pass@mirror.example/x",
		"http://u:sentinel\x7fpass@mirror.example/x",
		"u:sentinel-pass@mirror.example/x",
		"https://u:sentinel@pass@mirror.example/x?t=sentinel'x",
	} {
		got := RedactURL(raw)
		if strings.Contains(got, "sentinel") || !strings.Contains(got, "***@mirror.example/x") {
			t.Errorf("RedactURL(%q) = %q", raw, got)
		}
	}
}

// TestRedactKnownCoversReencodedSecrets: a cause that quotes the configured
// password decoded or re-escaped, outside any URL shape, is masked too.
func TestRedactKnownCoversReencodedSecrets(t *testing.T) {
	odd := "sentinel%27pw%40x"
	configured := "https://mirror:" + odd + "@mirror.example/base?token=sentinel-token%21"
	for _, msg := range []string{
		"auth failed for password sentinel'pw@x",
		"auth failed for sentinel%27pw%40x",
		"token sentinel-token! rejected",
		`Get "https://mirror:***@mirror.example/base?token=sentinel-token%21/releases/latest": EOF`,
	} {
		got := RedactKnown(msg, configured)
		if strings.Contains(got, "sentinel") {
			t.Errorf("RedactKnown(%q) = %q", msg, got)
		}
	}
	if got := RedactKnown("mirror.example said 500", configured); got != "mirror.example said 500" {
		t.Errorf("a message with no secret changed: %q", got)
	}
}

// TestRunAndCheckRedactOddPasswords drives Run and Check against a loopback
// mirror answering 503 for every odd password: no piece of it reaches the
// error, the status and host do, and the binary is untouched.
func TestRunAndCheckRedactOddPasswords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/"+tagV, http.StatusFound)
			return
		}
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	exe := filepath.Join(t.TempDir(), "deck")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	for name, pw := range oddPasswords {
		base := strings.Replace(srv.URL, "http://", "http://mirror:"+pw+"@", 1)
		_, err := Run(context.Background(), Options{BaseURL: base, Exe: exe, Current: "v0.0.1", Version: tagV})
		if err == nil || strings.Contains(err.Error(), "sentinel") || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("%s: Run err = %v", name, err)
		}
		t.Setenv(EnvBaseURL, base)
		_, err = Run(context.Background(), Options{Exe: exe, Current: "v0.0.1"})
		if err == nil || strings.Contains(err.Error(), "sentinel") {
			t.Errorf("%s: Run via %s err = %v", name, EnvBaseURL, err)
		}
		t.Setenv(EnvBaseURL, "")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Errorf("binary = %q after failed updates", b)
	}
}
