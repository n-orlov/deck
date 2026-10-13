package tui

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
	upd "github.com/n-orlov/deck/internal/update"
)

const (
	ctrlUOldBody = "old deck binary\n"
	ctrlUNewBody = "new deck binary\n"
	ctrlUCurrent = "v1.0.0"
)

var ctrlUKey = tea.KeyMsg(tea.Key{Type: tea.KeyCtrlU})

func ctrlUSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func ctrlUArchive(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: "deck", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ctrlUFakeServer is the local fake release server: the latest release is
// indicatorTag. With badChecksum the published checksum does not match the
// archive, so the verified update must refuse it.
func ctrlUFakeServer(t *testing.T, badChecksum bool) *httptest.Server {
	t.Helper()
	asset, err := upd.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no release asset for this platform: %v", err)
	}
	archive := ctrlUArchive(t, ctrlUNewBody)
	digest := ctrlUSum(archive)
	if badChecksum {
		digest = strings.Repeat("0", 64)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+indicatorTag, http.StatusFound)
	})
	dl := "/releases/download/" + indicatorTag + "/"
	mux.HandleFunc(dl+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc(dl+"checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", digest, asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ctrlUInstallModel is a model whose indicator reads Available and whose
// ctrl+u runs the real verified update against the fake server, on a
// scratch binary. It returns the binary's path and its original sha256.
func ctrlUInstallModel(t *testing.T, badChecksum bool) (Model, string, string, *atomic.Int32) {
	t.Helper()
	srv := ctrlUFakeServer(t, badChecksum)
	t.Setenv("DECK_UPDATE_URL", srv.URL)
	t.Setenv("PATH", t.TempDir()) // no gh: the update is checksum-verified only
	dir := t.TempDir()
	exe := filepath.Join(dir, "deck")
	if err := os.WriteFile(exe, []byte(ctrlUOldBody), 0o755); err != nil {
		t.Fatal(err)
	}
	var runs atomic.Int32
	opts := upd.Options{Exe: exe, Current: ctrlUCurrent, OnlyIfNewer: true}
	m := footerNoteFrameModel(240, 24, false)
	m = m.WithUpdateInstaller(ctrlUCurrent, func(ctx context.Context) (upd.Result, error) {
		runs.Add(1)
		return upd.Run(ctx, opts)
	})
	m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	return m, exe, ctrlUSum([]byte(ctrlUOldBody)), &runs
}

func ctrlUFileSum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return ctrlUSum(b)
}

func ctrlUPress(t *testing.T, m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return out, cmd
}

// TestCtrlUConfirmInstall: ctrl+u while Available opens a confirmation naming
// the current and the target version; Enter starts the update off the UI
// goroutine (nothing on disk has changed when Update returns, the indicator
// reads its progress state); the update's end flips the indicator to Installed.
func TestCtrlUConfirmInstall(t *testing.T) {
	m, exe, oldSum, runs := ctrlUInstallModel(t, false)
	m, cmd := ctrlUPress(t, m, ctrlUKey)
	if cmd != nil || !m.updateConfirming {
		t.Fatalf("ctrl+u opened no confirmation (confirming=%v, cmd=%v)", m.updateConfirming, cmd != nil)
	}
	view := stripANSI(m.View())
	for _, want := range []string{ctrlUCurrent, indicatorTag, "Enter installs", "Esc cancels"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation does not show %q:\n%s", want, view)
		}
	}
	if runs.Load() != 0 {
		t.Fatalf("the update started before the confirmation")
	}

	m, cmd = ctrlUPress(t, m, key("enter"))
	if cmd == nil {
		t.Fatal("confirming returned no command: the update would run on the UI goroutine or not at all")
	}
	if m.updateConfirming {
		t.Fatal("the confirmation stayed open after Enter")
	}
	if runs.Load() != 0 || ctrlUFileSum(t, exe) != oldSum {
		t.Fatalf("the update ran inside Update (runs=%d): it must run in the returned command", runs.Load())
	}
	if m.updateInd.state != updateInstalling {
		t.Fatalf("indicator state = %v, want the progress state", m.updateInd.state)
	}
	if note := m.footerNote(); !strings.Contains(note, "updating to "+indicatorTag) {
		t.Fatalf("footer note %q does not show the progress state", note)
	}
	// While it runs, ctrl+u is inert and a daily check cannot undo the state.
	again, again1 := ctrlUPress(t, m, ctrlUKey)
	if again.updateConfirming || again1 != nil {
		t.Fatal("ctrl+u during the update opened a dialog or started a second update")
	}
	m = sendUpdateMsg(t, m, updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}})
	if m.updateInd.state != updateInstalling {
		t.Fatalf("a daily check replaced the progress state with %v", m.updateInd.state)
	}

	msg := cmd()
	if runs.Load() != 1 {
		t.Fatalf("update ran %d times, want 1", runs.Load())
	}
	m = sendUpdateMsg(t, m, msg)
	const want = "update installed: v1.2.3 — restart deck to apply"
	if got := m.footerNote(); got != want {
		t.Fatalf("after the update: note = %q, want %q", got, want)
	}
	if got, _ := os.ReadFile(exe); string(got) != ctrlUNewBody {
		t.Fatalf("binary = %q, want the new release", got)
	}
}

// TestCtrlUCancelNoChange: Esc in the confirmation starts nothing, leaves the
// binary and the indicator exactly as they were.
func TestCtrlUCancelNoChange(t *testing.T) {
	m, exe, oldSum, runs := ctrlUInstallModel(t, false)
	before := m.updateInd
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if !m.updateConfirming {
		t.Fatal("ctrl+u opened no confirmation")
	}
	m, cmd := ctrlUPress(t, m, key("esc"))
	if cmd != nil || m.updateConfirming {
		t.Fatalf("esc left the dialog open or returned a command (confirming=%v)", m.updateConfirming)
	}
	if m.updateInd != before || runs.Load() != 0 || ctrlUFileSum(t, exe) != oldSum {
		t.Fatalf("cancel changed something: indicator %+v, runs %d", m.updateInd, runs.Load())
	}
	if got := m.footerNote(); got != indicatorAvailble {
		t.Fatalf("after cancel the note = %q, want the Available offer", got)
	}
}

// TestCtrlUInstallFailureKeepsBinary: the published checksum does not match,
// so the verified update refuses; the error shows in the indicator and the
// binary's sha256 and directory are unchanged.
func TestCtrlUInstallFailureKeepsBinary(t *testing.T) {
	m, exe, oldSum, _ := ctrlUInstallModel(t, true)
	m, _ = ctrlUPress(t, m, ctrlUKey)
	m, cmd := ctrlUPress(t, m, key("enter"))
	if cmd == nil {
		t.Fatal("confirming returned no command")
	}
	msg := cmd()
	if _, ok := msg.(updateFailedMsg); !ok {
		t.Fatalf("the update's end = %T, want updateFailedMsg", msg)
	}
	m = sendUpdateMsg(t, m, msg)
	note := m.footerNote()
	if m.updateInd.state != updateFailed || !strings.HasPrefix(note, "update failed") || !strings.Contains(note, "checksum") {
		t.Fatalf("indicator %+v, note %q: the failure's cause is not shown", m.updateInd, note)
	}
	if got := ctrlUFileSum(t, exe); got != oldSum {
		t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
	}
	ents, err := os.ReadDir(filepath.Dir(exe))
	if err != nil || len(ents) != 1 {
		t.Fatalf("install directory = %v, %v; want only the binary", ents, err)
	}
}

// TestCtrlUInertStates: five states where ctrl+u does nothing and is listed
// nowhere, and the Available state where it is listed in the footer and `?`.
func TestCtrlUInertStates(t *testing.T) {
	inert := map[string]bool{"installed": true, "failed": true, "hidden-disabled": true, "unwritable": true, "managed": true}
	seen := 0
	for _, row := range updateIndicatorRows {
		if !inert[row.name] {
			continue
		}
		seen++
		t.Run(row.name, func(t *testing.T) {
			m := row.set(t, footerNoteFrameModel(240, 24, false))
			var runs atomic.Int32
			m = m.WithUpdateInstaller(ctrlUCurrent, func(context.Context) (upd.Result, error) {
				runs.Add(1)
				return upd.Result{}, nil
			})
			before := m.updateInd
			next, cmd := ctrlUPress(t, m, ctrlUKey)
			if next.updateConfirming || cmd != nil || runs.Load() != 0 || next.updateInd != before {
				t.Fatalf("ctrl+u acted: confirming=%v cmd=%v runs=%d indicator %+v -> %+v", next.updateConfirming, cmd != nil, runs.Load(), before, next.updateInd)
			}
			if hints := m.updateFooterHints(); len(hints) != 0 {
				t.Fatalf("footer lists %v", hints)
			}
			segments, _ := m.footerLegendSegments()
			if strings.Contains(stripANSI(strings.Join(segments, " ")), "ctrl+u") {
				t.Fatalf("footer legend lists ctrl+u: %v", segments)
			}
			if strings.Contains(m.helpBody(), "  ctrl+u install") {
				t.Fatal("the ? help lists the update key")
			}
		})
	}
	if seen != 5 {
		t.Fatalf("ran %d inert rows, want 5", seen)
	}
	t.Run("available", func(t *testing.T) {
		m, _, _, _ := ctrlUInstallModel(t, false)
		segments, _ := m.footerLegendSegments()
		if !strings.Contains(stripANSI(strings.Join(segments, " ")), "ctrl+u update deck") {
			t.Fatalf("footer legend does not list ctrl+u: %v", segments)
		}
		if !strings.Contains(m.helpBody(), "  ctrl+u install") {
			t.Fatal("the ? help does not list ctrl+u")
		}
		if next, _ := ctrlUPress(t, m, ctrlUKey); !next.updateConfirming {
			t.Fatal("ctrl+u opened no dialog")
		}
	})
}

var ctrlUTableEntry = regexp.MustCompile(`(?m)^\t\t"ctrl\+u":\s+Model\.(\w+),$`)

// TestCtrlUNoListBindingCollision: ctrl+u is bound once in the list-mode key
// table, to the update handler, and that handler is bound to nothing else.
func TestCtrlUNoListBindingCollision(t *testing.T) {
	src, err := os.ReadFile("list_keys.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(src), "listKeyHandlers = map[string]keyHandler{")
	if start < 0 {
		t.Fatal("listKeyHandlers literal not found")
	}
	table := string(src)[start:]
	if got := ctrlUTableEntry.FindAllStringSubmatch(table, -1); len(got) != 1 || got[0][1] != "keyUpdate" {
		t.Fatalf("ctrl+u entries in listKeyHandlers = %v, want exactly one, bound to keyUpdate", got)
	}
	if n := strings.Count(table, "Model.keyUpdate,"); n != 1 {
		t.Fatalf("keyUpdate is bound %d times, want 1", n)
	}
	if _, ok := listKeyHandlers["ctrl+u"]; !ok {
		t.Fatal("ctrl+u is not in the live listKeyHandlers map")
	}
}

// TestCtrlUTextFieldDeletesToStart: with the Available offer up, ctrl+u in a
// text field (the / filter) still deletes from the caret to the start and
// opens no dialog.
func TestCtrlUTextFieldDeletesToStart(t *testing.T) {
	m, _, _, runs := ctrlUInstallModel(t, false)
	m = sendKeys(m, "/", "h", "e", "l", "l", "o", "left", "left")
	if !m.filtering || m.filterEdit.Value() != "hello" || m.filterEdit.Caret() != 3 {
		t.Fatalf("setup: filtering=%v value=%q caret=%d", m.filtering, m.filterEdit.Value(), m.filterEdit.Caret())
	}
	m, cmd := ctrlUPress(t, m, ctrlUKey)
	if got := m.filterEdit.Value(); got != "lo" || m.filterEdit.Caret() != 0 {
		t.Fatalf("after ctrl+u the field = %q caret %d, want %q caret 0", got, m.filterEdit.Caret(), "lo")
	}
	if m.updateConfirming || runs.Load() != 0 {
		t.Fatalf("ctrl+u in a text field opened the update dialog (confirming=%v) or ran it", m.updateConfirming)
	}
	_ = cmd
}

// ctrlUPaneModel is an interactive-mode model bound to a real tmux pane that
// records the bytes it receives: the pane switches its tty to raw mode, then
// writes the hex of the first byte to a file.
func ctrlUPaneModel(t *testing.T, available bool, runs *atomic.Int32) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "bytes")
	ready := filepath.Join(dir, "ready")
	socket := selectionTestSocket("ctrlu-" + fmt.Sprint(available))
	script := fmt.Sprintf("stty raw -echo; touch %s; dd bs=1 count=1 2>/dev/null | od -An -tx1 > %s; sleep 600", ready, out)
	if o, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "deck_ctrlu", "-x", "80", "-y", "24", "sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("start tmux session: %v: %s", err, o)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the pane never switched to raw mode")
		}
		time.Sleep(20 * time.Millisecond)
	}

	m := New(nil, config.Settings{Mouse: true, Color: true, SelectOnDrag: true}, "")
	m.sessions = []store.Session{{ID: "target", Name: "ctrlu", Slug: "ctrlu", CWD: "/work", Status: "waiting"}}
	m.baseSessions = append([]store.Session(nil), m.sessions...)
	m.width, m.height = 100, 30
	m.selected = rowCursor(0)
	m.tmuxClient = tmux.Client{Socket: socket}
	m = m.WithUpdateInstaller(ctrlUCurrent, func(context.Context) (upd.Result, error) {
		runs.Add(1)
		return upd.Result{}, nil
	})
	if available {
		m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	} else {
		m = checkedModel(t, m, true, upd.Status{}, nil)
	}
	next, _ := m.enterInteractive()
	got := next.(Model)
	if !got.interactive || got.interactiveDispatcher == nil {
		t.Fatalf("enterInteractive did not enter interactive mode (error %q)", got.attachError)
	}
	t.Cleanup(func() { got.exitInteractive() })
	return got, out
}

// TestInteractiveCtrlUPassThrough: in interactive mode ctrl+u is never
// deck's, whatever the update indicator reads: no dialog opens, no update
// starts, and the pane receives the single byte 0x15.
func TestInteractiveCtrlUPassThrough(t *testing.T) {
	for _, row := range []struct {
		name      string
		available bool
	}{{"available", true}, {"hidden", false}} {
		t.Run(row.name, func(t *testing.T) {
			var runs atomic.Int32
			m, out := ctrlUPaneModel(t, row.available, &runs)
			before := m.updateInd
			next, cmd := m.updateInteractive(ctrlUKey)
			got := next.(Model)
			if cmd != nil || got.updateConfirming || runs.Load() != 0 || got.updateInd != before {
				t.Fatalf("ctrl+u acted in interactive mode: cmd=%v confirming=%v runs=%d", cmd != nil, got.updateConfirming, runs.Load())
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				b, _ := os.ReadFile(out)
				if s := strings.TrimSpace(string(b)); s != "" {
					if s != "15" {
						t.Fatalf("the pane received %q, want the single byte 15 (0x15)", s)
					}
					return
				}
				if time.Now().After(deadline) {
					t.Fatal("the pane never received a byte")
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

const ctrlUSentinel = "ui-mirror-password-sentinel"

// ctrlUMirrorModel is ctrlUInstallModel pointed at a credentialed mirror URL
// (user:ctrlUSentinel@127.0.0.1:port) served by handler.
func ctrlUMirrorModel(t *testing.T, handler http.Handler) (Model, string, string) {
	t.Helper()
	return ctrlUMirrorModelAs(t, "ui-user:"+ctrlUSentinel, handler)
}

// ctrlUMirrorModelAs is ctrlUMirrorModel with the mirror URL's userinfo.
func ctrlUMirrorModelAs(t *testing.T, userinfo string, handler http.Handler) (Model, string, string) {
	t.Helper()
	return ctrlUMirrorModelURL(t, func(u string) string { return strings.Replace(u, "http://", "http://"+userinfo+"@", 1) }, handler)
}

// ctrlUMirrorModelURL is ctrlUMirrorModel with DECK_UPDATE_URL set to
// withCreds(the mirror's plain URL).
func ctrlUMirrorModelURL(t *testing.T, withCreds func(string) string, handler http.Handler) (Model, string, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("DECK_UPDATE_URL", withCreds(srv.URL))
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

// TestCtrlUFailureNeverShowsMirrorPassword: a failed ctrl+u against a mirror
// whose URL carries a password never puts the password in the footer note or
// the rendered frame, whether the mirror answers HTTP 500, 404 or dies
// mid-body; the cause stays useful and the binary is untouched.
func TestCtrlUFailureNeverShowsMirrorPassword(t *testing.T) {
	asset, err := upd.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	latest := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+indicatorTag, http.StatusFound)
	}
	rows := map[string]struct {
		handler http.HandlerFunc
		want    string
	}{
		"http 500": {func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/releases/latest" {
				latest(w, r)
				return
			}
			http.Error(w, "boom", http.StatusInternalServerError)
		}, "500"},
		"truncated body": {func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/releases/latest" {
				latest(w, r)
				return
			}
			w.Header().Set("Content-Length", "100000")
			_, _ = w.Write([]byte("short"))
		}, "unexpected EOF"},
		"oversize checksums": {func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/releases/latest":
				latest(w, r)
			case strings.HasSuffix(r.URL.Path, "/"+asset):
				_, _ = w.Write(ctrlUArchive(t, ctrlUNewBody))
			default:
				_, _ = w.Write(bytes.Repeat([]byte("a"), 2<<20))
			}
		}, "larger than"},
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			m, exe, oldSum := ctrlUMirrorModel(t, row.handler)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			m, cmd := ctrlUPress(t, m, key("enter"))
			if cmd == nil {
				t.Fatal("confirming returned no command")
			}
			msg := cmd()
			if _, ok := msg.(updateFailedMsg); !ok {
				t.Fatalf("the update's end = %T, want updateFailedMsg", msg)
			}
			m = sendUpdateMsg(t, m, msg)
			note, frame := m.footerNote(), m.View()
			if strings.Contains(note, ctrlUSentinel) || strings.Contains(frame, ctrlUSentinel) {
				t.Fatalf("the mirror password reached the UI: note %q", note)
			}
			if !strings.HasPrefix(note, "update failed") || !strings.Contains(note, row.want) || !strings.Contains(note, "127.0.0.1") {
				t.Fatalf("note %q lost its non-secret diagnostics (want %q and the host)", note, row.want)
			}
			if got := ctrlUFileSum(t, exe); got != oldSum {
				t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
			}
		})
	}
}

// TestUpdateFailedNoteRedactsAnyCause: whatever produced the cause (a wrapped
// third-party error, an https mirror, a query token), the footer note is
// redacted at the point it is drawn.
func TestUpdateFailedNoteRedactsAnyCause(t *testing.T) {
	for _, cause := range []string{
		`Get "https://bob:` + ctrlUSentinel + `@mirror.example/releases/latest": EOF`,
		"GET https://bob:it's-" + ctrlUSentinel + "@'" + ctrlUSentinel + "'@mirror.example/x: 503 Service Unavailable",
		"GET https://" + ctrlUSentinel + "@mirror.example/x: 502 Bad Gateway",
		"GET https://mirror.example/x?token=" + ctrlUSentinel + "&a=b: 500",
	} {
		note := updateFailedText(cause)
		if strings.Contains(note, ctrlUSentinel) || !strings.Contains(note, "mirror.example") {
			t.Errorf("note %q for cause %q leaks or lost the host", note, cause)
		}
	}
}

// TestCtrlUFailureNeverShowsOddMirrorPassword: the same guarantee for
// passwords url.Parse accepts but a "safe character" match misses (an
// apostrophe, an '@', sub-delims): a 503 from the mirror leaves no piece of
// the password in the footer note or the frame, keeps the status and host,
// and leaves the binary untouched.
func TestCtrlUFailureNeverShowsOddMirrorPassword(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/"+indicatorTag, http.StatusFound)
			return
		}
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	for name, pw := range map[string]string{
		"apostrophe":  "independent'sentinel-password",
		"at sign":     "sentinel-left@sentinel-right",
		"sub-delims":  "sentinel(!$&*+,;=~)sentinel",
		"apostrophe@": "it's-sentinel@'sentinel'",
	} {
		t.Run(name, func(t *testing.T) {
			m, exe, oldSum := ctrlUMirrorModelAs(t, "mirror:"+pw, handler)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			m, cmd := ctrlUPress(t, m, key("enter"))
			if cmd == nil {
				t.Fatal("confirming returned no command")
			}
			m = sendUpdateMsg(t, m, cmd())
			note, frame := m.footerNote(), m.View()
			if strings.Contains(note, "sentinel") || strings.Contains(frame, "sentinel") {
				t.Fatalf("a piece of the mirror password reached the UI: note %q", note)
			}
			if !strings.HasPrefix(note, "update failed") || !strings.Contains(note, "503") || !strings.Contains(note, "127.0.0.1") {
				t.Fatalf("note %q lost its non-secret diagnostics (want 503 and the host)", note)
			}
			if got := ctrlUFileSum(t, exe); got != oldSum {
				t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
			}
		})
	}
}

// TestCtrlUFailureNeverShowsBreakingSecret: a mirror credential holding a
// byte that ends a free-text match ('"', '\\', '<', whitespace), in the query
// or the userinfo, leaves no piece of itself in the footer note or the frame
// after the failure; the note still names the cause (and the host, where the
// cause quotes the URL), and the binary is untouched.
func TestCtrlUFailureNeverShowsBreakingSecret(t *testing.T) {
	// With a query in the base URL the appended path lands in the query, so
	// the latest lookup is recognised by its suffix wherever it ends up; the
	// download then fails with a message that quotes the URL.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") || strings.HasSuffix(r.URL.RawQuery, "/releases/latest") {
			http.Redirect(w, r, "/releases/tag/"+indicatorTag, http.StatusFound)
			return
		}
		http.Error(w, "down", http.StatusServiceUnavailable)
	})
	for name, row := range map[string]struct {
		withCreds func(string) string
		want      string
	}{
		"query quote":          {func(u string) string { return u + "?token=sentinel-left\"sentinel-tail" }, "503"},
		"query backslash":      {func(u string) string { return u + "?token=sentinel-left\\sentinel-tail" }, "503"},
		"query angle":          {func(u string) string { return u + "?token=sentinel-left<sentinel-tail>" }, "503"},
		"query space":          {func(u string) string { return u + "?token=sentinel-left sentinel-tail" }, "400"},
		"short query angle":    {func(u string) string { return u + "?t=K>Z" }, "503"},
		"short query space":    {func(u string) string { return u + "?t=K Z" }, "400"},
		"short query one byte": {func(u string) string { return u + "?t=Z" }, "503"},
		"short query pair":     {func(u string) string { return u + "?a=1&t=K>Z&b=2" }, "503"},
		"short userinfo angle": {func(u string) string { return strings.Replace(u, "http://", "http://m:K>Z@", 1) }, "not a valid absolute URL"},
		"short userinfo space": {func(u string) string { return strings.Replace(u, "http://", "http://m:K Z@", 1) }, "not a valid absolute URL"},
		"userinfo quote": {func(u string) string {
			return strings.Replace(u, "http://", "http://mirror:sentinel-left\"sentinel-tail@", 1)
		}, "not a valid absolute URL"},
	} {
		t.Run(name, func(t *testing.T) {
			m, exe, oldSum := ctrlUMirrorModelURL(t, row.withCreds, handler)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			m, cmd := ctrlUPress(t, m, key("enter"))
			if cmd == nil {
				t.Fatal("confirming returned no command")
			}
			m = sendUpdateMsg(t, m, cmd())
			note, frame := m.footerNote(), m.View()
			if strings.Contains(note, "sentinel") || strings.Contains(frame, "sentinel") {
				t.Fatalf("a piece of the mirror secret reached the UI: note %q", note)
			}
			if strings.HasPrefix(name, "short ") && strings.Contains(note, "Z") {
				t.Fatalf("a short mirror credential reached the UI note %q", note)
			}
			if !strings.HasPrefix(note, "update failed") || !strings.Contains(note, row.want) {
				t.Fatalf("note %q lost its non-secret diagnostics (want %q)", note, row.want)
			}
			if row.want != "400" && !strings.Contains(note, "127.0.0.1") {
				t.Fatalf("note %q lost the host", note)
			}
			if got := ctrlUFileSum(t, exe); got != oldSum {
				t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
			}
		})
	}
}
