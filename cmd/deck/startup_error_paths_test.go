package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// isolateDeckEnv points every variable run() and its helpers read at an
// empty private world, so a test only sees what it sets afterwards.
func isolateDeckEnv(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(key, "DECK_") {
			t.Setenv(key, "")
		}
	}
	for _, key := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "TMUX_PANE"} {
		t.Setenv(key, "")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DECK_HOME", filepath.Join(home, "deckhome"))
	// An empty PATH means no tmux binary can be found even if one is installed.
	t.Setenv("PATH", t.TempDir())
	return home
}

func runCapture(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"deck"}, args...), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunHookWithUnresolvableProfileHomeExitsOneNamingTheProfile(t *testing.T) {
	isolateDeckEnv(t)
	t.Setenv("HOME", "")
	t.Setenv("DECK_HOME", "")
	t.Setenv("DECK_PROFILE", "work")
	t.Setenv("TMUX_PANE", "%7")
	code, _, stderr := runCapture(t, `{"hook_event_name":"Notification"}`, "_hook")
	if code != 1 {
		t.Fatalf("exit code = %d, want exactly 1 (SPEC 3.4); stderr=%q", code, stderr)
	}
	for _, want := range []string{`TMUX_PANE="%7"`, `profile "work"`, "resolve home directory"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr %q lacks %q", stderr, want)
		}
	}
}

func TestRunWithUnresolvableProfileHomeReportsAndExitsZero(t *testing.T) {
	isolateDeckEnv(t)
	t.Setenv("HOME", "")
	t.Setenv("DECK_HOME", "")
	t.Setenv("DECK_PROFILE", "work")
	code, stdout, stderr := runCapture(t, "")
	if code != 0 || stdout != "" || !strings.HasPrefix(stderr, "deck profile:") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0, empty stdout and a 'deck profile:' line", code, stdout, stderr)
	}
}

func TestRunInvalidConfigurationStopsBeforeAnyStateIsOpened(t *testing.T) {
	isolateDeckEnv(t)
	deckHome := absoluteEnvPath(t, "DECK_HOME")
	t.Setenv("DECK_CLOCK", "yesterday-ish") // not RFC3339: LoadFromProfile refuses it
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"hook exits 1", []string{"_hook"}, 1},
		{"tui launch exits 0", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCapture(t, `{"hook_event_name":"Notification"}`, tc.args...)
			if code != tc.want || !strings.HasPrefix(stderr, "deck configuration:") || !strings.Contains(stderr, "DECK_CLOCK must be RFC3339") {
				t.Fatalf("code=%d stderr=%q, want %d and a 'deck configuration:' line naming DECK_CLOCK", code, stderr, tc.want)
			}
			if _, err := os.Stat(filepath.Join(deckHome, "state.db")); !os.IsNotExist(err) {
				t.Fatalf("state.db was created despite the configuration error: %v", err)
			}
		})
	}
}

func TestRunHookReportsHookFailureAsExitOne(t *testing.T) {
	isolateDeckEnv(t)
	code, _, stderr := runCapture(t, "not json at all", "_hook")
	if code != 1 || !strings.HasPrefix(stderr, "deck hook: read one JSON object") {
		t.Fatalf("code=%d stderr=%q, want 1 and 'deck hook: read one JSON object'", code, stderr)
	}
}

func TestRunStartupFailuresAreReportedAndNeverStartTheTUI(t *testing.T) {
	t.Run("last_used marker that cannot be touched is reported and startup continues", func(t *testing.T) {
		isolateDeckEnv(t)
		deckHome := absoluteEnvPath(t, "DECK_HOME")
		if err := os.MkdirAll(filepath.Join(deckHome, "last_used"), 0o700); err != nil {
			t.Fatal(err)
		}
		// A corrupt state.db then ends startup deterministically, after the
		// last_used step, without ever reaching the TUI.
		if err := os.WriteFile(filepath.Join(deckHome, "state.db"), []byte("garbage, not sqlite"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCapture(t, "")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
		}
		lastUsed := strings.Index(stderr, "deck last used:")
		state := strings.Index(stderr, "deck state:")
		if lastUsed < 0 || state < 0 || lastUsed > state {
			t.Fatalf("stderr %q must report 'deck last used:' then stop at 'deck state:'", stderr)
		}
	})
	t.Run("audit log that cannot open stops startup with exit 0", func(t *testing.T) {
		isolateDeckEnv(t)
		deckHome := absoluteEnvPath(t, "DECK_HOME")
		if err := os.MkdirAll(deckHome, 0o700); err != nil {
			t.Fatal(err)
		}
		// log is a regular file, so audit.New cannot create its directory.
		if err := os.WriteFile(filepath.Join(deckHome, "log"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCapture(t, "")
		if code != 0 || !strings.Contains(stderr, "deck audit:") {
			t.Fatalf("code=%d stderr=%q, want 0 and a 'deck audit:' line", code, stderr)
		}
		if strings.Contains(stderr, "deck state:") {
			t.Fatalf("the state database opened fine and must not be blamed: %q", stderr)
		}
	})
}

func TestRunHookFailureModes(t *testing.T) {
	object := `{"hook_event_name":"Notification","session_id":"nobody"}`
	for _, tc := range []struct {
		name    string
		stdin   string
		prepare func(t *testing.T, settings *config.Settings)
		want    string
	}{
		{name: "empty stdin", stdin: "", want: "read one JSON object"},
		{name: "truncated object", stdin: `{"hook_event_name":`, want: "read one JSON object"},
		{name: "array instead of object", stdin: `[1]`, want: "stdin must contain one JSON object"},
		{name: "scalar instead of object", stdin: `"text"`, want: "stdin must contain one JSON object"},
		{name: "two objects", stdin: object + object, want: "stdin contains more than one JSON value"},
		{name: "trailing garbage", stdin: object + " }", want: "reject trailing stdin"},
		{name: "state database missing", stdin: object, want: "state database does not exist"},
		{
			name: "state database is a directory", stdin: object, want: "state database is not a regular file",
			prepare: func(t *testing.T, settings *config.Settings) {
				paths := settings.Paths
				if err := os.MkdirAll(paths.StateDB, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "state database path unreachable", stdin: object, want: "inspect state database",
			prepare: func(t *testing.T, settings *config.Settings) {
				paths := settings.Paths
				blocker := filepath.Join(paths.Home, "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				settings.Paths.StateDB = filepath.Join(blocker, "state.db")
			},
		},
		{
			name: "state database is not sqlite", stdin: object, want: "open existing state database",
			prepare: func(t *testing.T, settings *config.Settings) {
				paths := settings.Paths
				if err := os.WriteFile(paths.StateDB, []byte("garbage, not a database"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "audit log directory cannot be created", stdin: object, want: "open audit log",
			prepare: func(t *testing.T, settings *config.Settings) {
				paths := settings.Paths
				db, err := store.Open(paths)
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.LogDir, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, paths := hookCapSettings(t)
			if tc.prepare != nil {
				tc.prepare(t, &settings)
			}
			err := runHook(context.Background(), settings, strings.NewReader(tc.stdin))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runHook err = %v, want it to contain %q", err, tc.want)
			}
			if tc.name == "state database missing" {
				if _, statErr := os.Stat(paths.StateDB); !os.IsNotExist(statErr) {
					t.Fatalf("a late hook recreated the deleted state database: %v", statErr)
				}
			}
		})
	}
}

func TestRunHookUnresolvedHookIsPreservedAsAnOrphanEventAndRejected(t *testing.T) {
	settings, paths := hookCapSettings(t)
	t.Setenv("DECK_SESSION_ID", "")
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	payload := `{"hook_event_name":"Notification","session_id":"conversation-nobody","notification_type":"permission_prompt"}`
	err = runHook(context.Background(), settings, strings.NewReader(payload))
	if !errors.Is(err, hookrecv.ErrUnresolved) {
		t.Fatalf("runHook err = %v, want hookrecv.ErrUnresolved", err)
	}
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].SessionID != "" || events[0].Payload != payload {
		t.Fatalf("events = %#v, want exactly one session-less orphan event carrying the payload", events)
	}
}

func TestRunHookReportsAFailedPostHookLivenessPass(t *testing.T) {
	settings, paths := hookCapSettings(t)
	t.Setenv("DECK_SESSION_ID", "")
	t.Setenv("PATH", t.TempDir()) // no tmux binary anywhere
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "live", Name: "live", CWD: paths.Home, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conversation-live",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	settings.Reconcile = 50 * time.Millisecond
	err = runHook(context.Background(), settings, strings.NewReader(`{"hook_event_name":"Notification","session_id":"conversation-live","notification_type":"permission_prompt"}`))
	if err == nil || !strings.Contains(err.Error(), "post-hook liveness pass") {
		t.Fatalf("runHook err = %v, want a 'post-hook liveness pass' failure", err)
	}
	// The hook's own write is durable even though the liveness pass failed.
	db, err = store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.SessionID == "live" && e.Kind == "notification" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hook event was not stored before the liveness pass failed: %#v", events)
	}
}

func TestCappedHookReaderStaysFailedAfterExceedingTheCap(t *testing.T) {
	c := &cappedHookReader{r: strings.NewReader("abcdef"), remaining: 3}
	buf := make([]byte, 3)
	if n, err := c.Read(buf); n != 3 || err != nil {
		t.Fatalf("first read = %d, %v, want the 3 permitted bytes", n, err)
	}
	if _, err := c.Read(buf); !errors.Is(err, errHookPayloadTooLarge) {
		t.Fatalf("read past the cap err = %v, want errHookPayloadTooLarge", err)
	}
	// A later read must keep failing rather than pull more of the payload.
	if n, err := c.Read(buf); n != 0 || !errors.Is(err, errHookPayloadTooLarge) {
		t.Fatalf("read after exceeded = %d, %v, want 0 and errHookPayloadTooLarge", n, err)
	}
}

func failingUserHome() (string, error) { return "", errors.New("no home here") }

func TestConfirmAndCreateProfileFailureAndRefusalPaths(t *testing.T) {
	noEnv := func(string) string { return "" }
	t.Run("known profiles cannot be listed", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := confirmAndCreateProfile("work", noEnv, failingUserHome, strings.NewReader(""), &stderr); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "deck profile:") || !strings.Contains(stderr.String(), "no home here") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})
	openTTY := func(t *testing.T, answer string) *os.File {
		t.Helper()
		master, slave, err := pty.Open()
		if err != nil {
			t.Skipf("no pty available: %v", err)
		}
		t.Cleanup(func() { _ = master.Close(); _ = slave.Close() })
		if _, err := master.WriteString(answer); err != nil {
			t.Fatal(err)
		}
		return slave
	}
	for _, answer := range []string{"n\n", "\n", "Y\n", "yes\n", " y\n"} {
		t.Run(fmt.Sprintf("answer %q creates nothing", answer), func(t *testing.T) {
			root := t.TempDir()
			getenv := func(k string) string {
				if k == "DECK_HOME" {
					return root
				}
				return ""
			}
			var stderr bytes.Buffer
			if code := confirmAndCreateProfile("work", getenv, os.UserHomeDir, openTTY(t, answer), &stderr); code != 1 {
				t.Fatalf("code = %d, want 1; stderr=%q", code, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(root, "profiles", "work")); !os.IsNotExist(err) {
				t.Fatalf("a declined prompt created the profile directory: %v", err)
			}
			if !strings.Contains(stderr.String(), "Create it? [y/N]") {
				t.Fatalf("stderr = %q lacks the prompt", stderr.String())
			}
		})
	}
	t.Run("a creation failure after y exits 1 and reports it", func(t *testing.T) {
		root := t.TempDir()
		// profiles/work/log is a regular file, so MkdirAll(LogDir) must fail.
		if err := os.MkdirAll(filepath.Join(root, "profiles", "work"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "profiles", "work", "log"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		getenv := func(k string) string {
			if k == "DECK_HOME" {
				return root
			}
			return ""
		}
		var stderr bytes.Buffer
		if code := confirmAndCreateProfile("work", getenv, os.UserHomeDir, openTTY(t, "y\n"), &stderr); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "deck profile:") || !strings.Contains(stderr.String(), "log directory") {
			t.Fatalf("stderr = %q, want a 'deck profile:' line naming the log directory", stderr.String())
		}
	})
	t.Run("y creates the profile directories and returns 0", func(t *testing.T) {
		root := t.TempDir()
		getenv := func(k string) string {
			if k == "DECK_HOME" {
				return root
			}
			return ""
		}
		var stderr bytes.Buffer
		if code := confirmAndCreateProfile("work", getenv, os.UserHomeDir, openTTY(t, "y\n"), &stderr); code != 0 {
			t.Fatalf("code = %d, want 0; stderr=%q", code, stderr.String())
		}
		if info, err := os.Stat(filepath.Join(root, "profiles", "work", "log")); err != nil || !info.IsDir() {
			t.Fatalf("profile log directory missing after y: %v", err)
		}
	})
}

func TestRunProfilesListingReportsAnUnresolvableHomeAndStillExitsZero(t *testing.T) {
	var stdout bytes.Buffer
	code := runProfilesListing(func(string) string { return "" }, failingUserHome, &stdout)
	if code != 0 || !strings.HasPrefix(stdout.String(), "deck profiles:") || !strings.Contains(stdout.String(), "no home here") {
		t.Fatalf("code=%d stdout=%q, want 0 and a 'deck profiles:' error line", code, stdout.String())
	}
}

func TestPreFrameTombstoneSweepReportsFailureAndStepsOver(t *testing.T) {
	settings, paths := hookCapSettings(t)
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	// Must return normally (startup continues) and say what failed.
	preFrameTombstoneSweep(context.Background(), db, settings, &stderr)
	if !strings.HasPrefix(stderr.String(), "deck tombstone sweep:") {
		t.Fatalf("stderr = %q, want a 'deck tombstone sweep:' line", stderr.String())
	}
}

func TestNewTUIReconcileSurfacesTheLivenessProbeError(t *testing.T) {
	settings, paths := hookCapSettings(t)
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings.StaleAfter = 0 // ReconcileWithProbes rejects a non-positive window
	reconcile := newTUIReconcile(db, service.Service{Store: db}, settings)
	err = reconcile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "stale_after must be positive") {
		t.Fatalf("reconcile err = %v, want the probe's own stale_after error", err)
	}
}

func TestClockStepSignalFailureIsReportedOnStderr(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deckhome")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, err := config.LoadFrom(func(key string) string {
		return map[string]string{
			"DECK_HOME":       home,
			"DECK_CLOCK":      "2025-01-02T03:04:05Z",
			"DECK_CLOCK_STEP": "45s",
		}[key]
	}, os.UserHomeDir)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the shared-clock directory with a file after the clock loaded.
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr syncBuffer
	stop := startClockStepTrigger(settings.Clock, &stderr)
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(stderr.String(), "deck clock step:") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(stderr.String(), "deck clock step:") {
		t.Fatalf("stderr = %q, want a 'deck clock step:' line", stderr.String())
	}
	if got := settings.Clock.Now().Format(time.RFC3339); got != "2025-01-02T03:04:05Z" {
		t.Fatalf("a failed step advanced the clock to %s", got)
	}
}

// syncBuffer is a bytes.Buffer safe for the trigger goroutine to write while
// the test polls it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

var _ io.Writer = (*syncBuffer)(nil)

// shutdownRecorder is a tea.Model that can be torn down and that panics when
// asked to Update, standing in for the real model under the panic guard.
type shutdownRecorder struct {
	shutdowns *int
	panicWith any
}

func (m shutdownRecorder) Init() tea.Cmd                       { return nil }
func (m shutdownRecorder) View() string                        { return "" }
func (m shutdownRecorder) Update(tea.Msg) (tea.Model, tea.Cmd) { panic(m.panicWith) }
func (m shutdownRecorder) ShutdownInteractive(context.Context) { *m.shutdowns++ }

// plainModel neither unwraps nor can shut anything down.
type plainModel struct{}

func (plainModel) Init() tea.Cmd                       { return nil }
func (plainModel) View() string                        { return "plain" }
func (plainModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return plainModel{}, nil }

func TestInteractiveShutdownGuardTearsDownThenRepanicsOnAnUpdatePanic(t *testing.T) {
	shutdowns := 0
	guard := wrapForInteractiveShutdownOnPanic(shutdownRecorder{shutdowns: &shutdowns, panicWith: "boom"})
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		guard.Update(tea.KeyMsg{})
	}()
	if recovered != "boom" {
		t.Fatalf("recovered = %v, want the original panic value re-raised", recovered)
	}
	if shutdowns != 1 {
		t.Fatalf("ShutdownInteractive ran %d times, want exactly once before the panic escaped", shutdowns)
	}
}

func TestShutdownArmedInteractiveClaimWalksWrappersAndIgnoresUnshutdownableModels(t *testing.T) {
	shutdowns := 0
	inner := shutdownRecorder{shutdowns: &shutdowns}
	shutdownArmedInteractiveClaim(wrapForInteractiveShutdownOnPanic(wrapForInteractiveShutdownOnPanic(inner)))
	if shutdowns != 1 {
		t.Fatalf("shutdown through two guard layers ran %d times, want 1", shutdowns)
	}
	// Nothing to unwrap and no ShutdownInteractive: silently left alone.
	shutdownArmedInteractiveClaim(plainModel{})
	shutdownArmedInteractiveClaim(nil)
	if shutdowns != 1 {
		t.Fatalf("a model without ShutdownInteractive caused a shutdown: %d", shutdowns)
	}
}

// absoluteEnvPath returns the path an isolated test run exported in the named
// variable (isolateDeckEnv sets it to a t.TempDir() path) and fails the test
// when it is not absolute, so a missing or relative value can never make the
// fixture write outside the temp tree.
func absoluteEnvPath(t *testing.T, name string) string {
	t.Helper()
	p := os.Getenv(name)
	if !filepath.IsAbs(p) {
		t.Fatalf("%s=%q, want an absolute path set by isolateDeckEnv", name, p)
	}
	return p
}
