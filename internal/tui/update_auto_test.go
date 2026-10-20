package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	upd "github.com/n-orlov/deck/internal/update"
)

// autoServer is the local fake release server for the auto-update tests: the
// latest release is indicatorTag, and it counts the archive downloads and the
// checks it answers. With badChecksum the published checksum does not match.
type autoServer struct {
	*httptest.Server
	archives atomic.Int32
	checks   atomic.Int32
}

func newAutoServer(t *testing.T, badChecksum bool) *autoServer {
	t.Helper()
	asset, err := upd.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("no release asset for this platform: %v", err)
	}
	archive := ctrlUArchive(t, ctrlUNewBody(t))
	digest := ctrlUSum(archive)
	if badChecksum {
		digest = strings.Repeat("0", 64)
	}
	s := &autoServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		s.checks.Add(1)
		http.Redirect(w, r, "/releases/tag/"+indicatorTag, http.StatusFound)
	})
	dl := "/releases/download/" + indicatorTag + "/"
	mux.HandleFunc(dl+asset, func(w http.ResponseWriter, _ *http.Request) {
		s.archives.Add(1)
		_, _ = w.Write(archive)
	})
	mux.HandleFunc(dl+"checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", digest, asset)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// autoRun starts a model the way deck does at launch: a real throttled
// Checker and Preflight against srv for the binary at exe (running as
// current), the real verified installer, and auto_update set as given. It
// delivers the daily check's answer and then runs every command the model
// returns in turn, as the runtime does, and returns the settled model.
func autoRun(t *testing.T, srv *autoServer, exe, current string, autoUpdate bool) Model {
	t.Helper()
	return autoRunCache(t, srv, exe, current, autoUpdate, filepath.Join(t.TempDir(), "update-check.json"))
}

// autoRunCache is autoRun with the shared update-check cache at cache, so two
// launches can share one 24-hour window.
func autoRunCache(t *testing.T, srv *autoServer, exe, current string, autoUpdate bool, cache string) Model {
	t.Helper()
	t.Setenv("DECK_UPDATE_URL", srv.URL)
	settings := config.Settings{AutoUpdate: autoUpdate, UpdateCheck: true}
	opts := upd.Options{Exe: exe, Current: current}
	checker := &upd.Checker{
		Enabled:   settings.UpdateCheckEnabled,
		CachePath: cache,
		Options:   opts,
	}
	install := opts
	install.OnlyIfNewer = true
	m := footerNoteFrameModel(240, 24, false)
	m.settings = settings
	m = m.WithUpdateSource(checker.Check, func() error { return upd.Preflight(opts) })
	m = m.WithUpdateInstaller(current, func(ctx context.Context) (upd.Result, error) { return upd.Run(ctx, install) })

	msg := m.updateCheckCmd()()
	for steps := 0; msg != nil; steps++ {
		if steps > 4 {
			t.Fatal("the model kept returning commands")
		}
		next, cmd := m.Update(msg)
		m = next.(Model)
		msg = nil
		if cmd != nil {
			msg = cmd()
		}
	}
	return m
}

func autoBinary(t *testing.T) (string, string) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "deck")
	if err := os.WriteFile(exe, []byte(ctrlUOldBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe, ctrlUSum([]byte(ctrlUOldBody))
}

// TestAutoUpdate: with auto_update on and a newer release the daily check
// installs it through the verified path with no prompt, and the indicator
// reads Installed.
func TestAutoUpdate(t *testing.T) {
	srv := newAutoServer(t, false)
	exe, _ := autoBinary(t)
	m := autoRun(t, srv, exe, ctrlUCurrent, true)
	if m.updateConfirming {
		t.Fatal("auto-update opened a confirmation")
	}
	if got, want := m.footerNote(), "update installed: v1.2.3 (not verified immutable) — restart deck to apply"; got != want {
		t.Fatalf("indicator = %q, want %q", got, want)
	}
	if got := srv.archives.Load(); got != 1 {
		t.Fatalf("archive downloads = %d, want 1", got)
	}
	if got, _ := os.ReadFile(exe); string(got) != ctrlUNewBody(t) {
		t.Fatalf("binary = %q, want the new release", got)
	}
}

// TestAutoUpdateFailure: an install failure (the published checksum does not
// match) leaves the binary's sha256 unchanged and shows the Failed notice.
func TestAutoUpdateFailure(t *testing.T) {
	srv := newAutoServer(t, true)
	exe, oldSum := autoBinary(t)
	m := autoRun(t, srv, exe, ctrlUCurrent, true)
	note := m.footerNote()
	if m.updateInd.state != updateFailed || !strings.HasPrefix(note, "update failed") || !strings.Contains(note, "checksum") {
		t.Fatalf("indicator %+v, note %q: want the Failed notice with its cause", m.updateInd, note)
	}
	if got := ctrlUFileSum(t, exe); got != oldSum {
		t.Fatalf("binary sha256 = %s, want the original %s", got, oldSum)
	}
	ents, err := os.ReadDir(filepath.Dir(exe))
	if err != nil || len(ents) != 1 {
		t.Fatalf("install directory = %v, %v; want only the binary", ents, err)
	}
}

// TestAutoUpdateSkipped: with auto_update on, an install that would be refused
// downloads no archive, and the indicator names why: the unwritable and
// managed rows show their refusal reason, and the dev build row (which the
// check never calls outdated) shows that auto-update skipped it.
func TestAutoUpdateSkipped(t *testing.T) {
	rows := []struct {
		name string
		set  func(t *testing.T) (exe, current string)
		show string // substring the footer note must contain
	}{
		{"unwritable", func(t *testing.T) (string, string) {
			if os.Geteuid() == 0 {
				t.Skip("root can write anywhere")
			}
			exe, _ := autoBinary(t)
			dir := filepath.Dir(exe)
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			return exe, ctrlUCurrent
		}, "install directory is not writable"},
		{"managed-prefix", func(*testing.T) (string, string) { return "/usr/bin/deck", ctrlUCurrent }, "installed by a package manager"},
		{"dev-build", func(t *testing.T) (string, string) {
			exe, _ := autoBinary(t)
			return exe, upd.DevVersion
		}, "auto-update skipped: v1.2.3 — dev build"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			srv := newAutoServer(t, false)
			exe, current := row.set(t)
			m := autoRun(t, srv, exe, current, true)
			if got := srv.archives.Load(); got != 0 {
				t.Fatalf("archive downloads = %d, want 0", got)
			}
			if m.updateInd.state == updateInstalled || m.updateInd.state == updateInstalling || m.updateInd.state == updateFailed {
				t.Fatalf("indicator state = %v, want a skipped install", m.updateInd.state)
			}
			note := m.footerNote()
			if note == "" || !strings.Contains(note, row.show) {
				t.Fatalf("note = %q, want it to contain %q", note, row.show)
			}
		})
	}
}

// TestAutoUpdateOff: with auto_update off and a newer release available the
// check offers it (Available) and downloads no archive.
func TestAutoUpdateOff(t *testing.T) {
	srv := newAutoServer(t, false)
	exe, oldSum := autoBinary(t)
	m := autoRun(t, srv, exe, ctrlUCurrent, false)
	if got := srv.archives.Load(); got != 0 {
		t.Fatalf("archive downloads = %d, want 0", got)
	}
	if got := srv.checks.Load(); got != 1 {
		t.Fatalf("checks = %d, want 1", got)
	}
	if got := m.footerNote(); got != indicatorAvailble {
		t.Fatalf("indicator = %q, want the Available offer", got)
	}
	if got := ctrlUFileSum(t, exe); got != oldSum {
		t.Fatalf("binary changed with auto_update off")
	}
}

// TestAutoUpdateFailureRetriesAtNextStart: a running deck attempts the install
// once, at its start-up check. A later start inside the 24-hour window reads
// the cached answer (its check makes no request) and attempts the install
// again; each failed attempt leaves the binary's sha256 unchanged and shows
// the Failed notice (SPEC §6.5).
func TestAutoUpdateFailureRetriesAtNextStart(t *testing.T) {
	srv := newAutoServer(t, true)
	exe, oldSum := autoBinary(t)
	cache := filepath.Join(t.TempDir(), "update-check.json")
	var perStart [3]int32 // /releases/latest requests made during each start
	for start := 1; start <= 2; start++ {
		before := srv.checks.Load()
		m := autoRunCache(t, srv, exe, ctrlUCurrent, true, cache)
		perStart[start] = srv.checks.Load() - before
		if m.updateInd.state != updateFailed {
			t.Fatalf("start %d: indicator %+v, want Failed", start, m.updateInd)
		}
		if got := srv.archives.Load(); got != int32(start) {
			t.Fatalf("start %d: archive downloads = %d, want %d (one attempt per start)", start, got, start)
		}
		if got := ctrlUFileSum(t, exe); got != oldSum {
			t.Fatalf("start %d: binary sha256 = %s, want the original %s", start, got, oldSum)
		}
	}
	// Start 1 makes the daily check's request plus the install's own release
	// lookup; start 2's check reads the cache, so only the install's lookup.
	if perStart[2] != perStart[1]-1 {
		t.Fatalf("latest-release requests per start = %d then %d; want the second start's check to make none", perStart[1], perStart[2])
	}
}
