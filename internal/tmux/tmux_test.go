package tmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestDiscoverRealTmux(t *testing.T) {
	version, err := (Client{}).Discover(context.Background())
	if err != nil {
		t.Fatalf("discover installed tmux: %v", err)
	}
	if !version.Supported() {
		t.Fatalf("installed tmux %s was accepted despite being unsupported", version)
	}
}

func TestDiscoverRejectsMissingTmux(t *testing.T) {
	_, err := (Client{Binary: filepath.Join(t.TempDir(), "missing-tmux")}).Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") || !strings.Contains(err.Error(), "3.2") {
		t.Fatalf("missing tmux error = %v, want actionable install guidance", err)
	}
}

func TestDiscoverRejectsPre32Tmux(t *testing.T) {
	binary := versionScript(t, "tmux 3.1c")
	_, err := (Client{Binary: binary}).Discover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "too old") || !strings.Contains(err.Error(), "3.2") {
		t.Fatalf("old tmux error = %v, want actionable minimum-version guidance", err)
	}
}

func TestCreateListAndKillRealTmux(t *testing.T) {
	socket := fmt.Sprintf("deck-lifecycle-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.command(ctx, "kill-server").Run()
	})
	cwd := t.TempDir()
	created, err := client.Create(context.Background(), Launch{
		Slug:    "shell_test",
		CWD:     cwd,
		Command: []string{"sh", "-c", "printf 'cwd=%s env=%s\\n' \"$PWD\" \"$DECK_TEST_ENV\"; sleep 30"},
		Env:     map[string]string{"DECK_TEST_ENV": "present"},
	})
	if err != nil {
		t.Fatalf("create private tmux session: %v", err)
	}
	if created.Name != "deck_shell_test" || len(created.Panes) != 1 {
		t.Fatalf("created session = %#v, want deck_shell_test with one pane", created)
	}

	output, err := client.command(context.Background(), "capture-pane", "-p", "-t", "deck_shell_test").CombinedOutput()
	if err != nil {
		t.Fatalf("capture created pane: %v: %s", err, output)
	}
	if got := string(output); !strings.Contains(got, "cwd="+cwd+" env=present") {
		t.Fatalf("created pane output = %q, want requested cwd and environment", got)
	}
	listed, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("list private sessions: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "deck_shell_test" || len(listed[0].Panes) != 1 {
		t.Fatalf("listed sessions = %#v, want one deck session and pane", listed)
	}
	pane := listed[0].Panes[0]
	if pane.CurrentPath != cwd || pane.ID == "" || pane.PID <= 0 || pane.Command == "" {
		t.Errorf("listed pane facts = %#v, want id, pid, command, and cwd %q", pane, cwd)
	}
	if err := client.Kill(context.Background(), "shell_test"); err != nil {
		t.Fatalf("kill private tmux session: %v", err)
	}
	if err := client.Kill(context.Background(), "shell_test"); err != nil {
		t.Fatalf("repeat kill of disappeared session: %v", err)
	}
	if err := client.command(context.Background(), "kill-server").Run(); err != nil {
		t.Fatalf("kill private tmux server: %v", err)
	}
	if err := client.Kill(context.Background(), "shell_test"); err != nil {
		t.Fatalf("kill after server disappeared: %v", err)
	}
	listed, err = client.List(context.Background())
	if err != nil {
		t.Fatalf("list after kill: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("sessions after kill = %#v, want none", listed)
	}
}

func TestCapturePaneRealTmuxUsesExplicitRange(t *testing.T) {
	socket := fmt.Sprintf("deck-capture-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })
	created, err := client.Create(context.Background(), Launch{
		Slug: "capture_test", CWD: t.TempDir(),
		Command: []string{"sh", "-c", `i=1; while [ "$i" -le 260 ]; do printf 'line-%03d\n' "$i"; i=$((i+1)); done; printf '\033[31mcolored-marker\033[0m\n'; sleep 30`},
	})
	if err != nil {
		t.Fatal(err)
	}
	paneID := created.Panes[0].ID
	all := CaptureOptions{StartLine: "-", EndLine: "-"}
	var plain []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		plain, err = client.CapturePane(context.Background(), paneID, all)
		if err == nil && bytes.Contains(plain, []byte("colored-marker")) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("capture complete pane range: %v", err)
	}
	if !bytes.Contains(plain, []byte("line-001")) || !bytes.Contains(plain, []byte("line-260")) {
		t.Fatalf("full capture did not retain range needed for a 200-line tail: first/last missing from %d bytes", len(plain))
	}
	if bytes.Contains(plain, []byte("\x1b[")) {
		t.Fatalf("plain capture contains terminal escapes: %q", plain)
	}

	escaped, err := client.CapturePane(context.Background(), paneID, CaptureOptions{
		StartLine: "-", EndLine: "-", IncludeEscapeSequences: true,
	})
	if err != nil {
		t.Fatalf("capture replay range with escapes: %v", err)
	}
	if !bytes.Contains(escaped, []byte("\x1b[")) || !bytes.Contains(escaped, []byte("colored-marker")) {
		t.Fatalf("escape-preserving capture = %q, want colored marker and SGR", escaped)
	}

	oneLine, err := client.CapturePane(context.Background(), paneID, CaptureOptions{StartLine: "0", EndLine: "0"})
	if err != nil {
		t.Fatalf("capture explicit single-line range: %v", err)
	}
	if got := bytes.Count(oneLine, []byte("\n")); got != 1 {
		t.Fatalf("single-line capture has %d lines (%q), want 1", got, oneLine)
	}
}

func TestPreviewPaneAndCapturePreviewRealTmux(t *testing.T) {
	socket := fmt.Sprintf("deck-preview-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })

	// No session yet: PreviewPane and CapturePreview both report absence, not
	// an error — a stopped/archived/not-yet-started row must not surface a
	// spurious tmux failure every tick (SPEC requirement 21).
	if _, ok, err := client.PreviewPane(context.Background(), "preview_test"); err != nil || ok {
		t.Fatalf("PreviewPane before create = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	if capture, err := client.CapturePreview(context.Background(), "preview_test"); err != nil || capture.Live {
		t.Fatalf("CapturePreview before create = %#v, err=%v, want inert", capture, err)
	}

	created, err := client.Create(context.Background(), Launch{
		Slug: "preview_test", CWD: t.TempDir(),
		Command: []string{"sh", "-c", `printf '\033[31mred-marker\033[0m\n'; sleep 30`},
	})
	if err != nil {
		t.Fatal(err)
	}

	var capture PreviewCapture
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		capture, err = client.CapturePreview(context.Background(), "preview_test")
		if err == nil && capture.Live && bytes.Contains(capture.Bytes, []byte("red-marker")) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("capture live pane: %v", err)
	}
	if !capture.Live {
		t.Fatalf("capture.Live = false for a running pane")
	}
	if !bytes.Contains(capture.Bytes, []byte("\x1b[")) || !bytes.Contains(capture.Bytes, []byte("red-marker")) {
		t.Fatalf("preview capture = %q, want SGR-preserving red-marker", capture.Bytes)
	}

	pane, ok, err := client.PreviewPane(context.Background(), "preview_test")
	if err != nil || !ok || pane.ID != created.Panes[0].ID {
		t.Fatalf("PreviewPane = (%#v, ok=%v, err=%v), want the created pane", pane, ok, err)
	}
}

// TestCapturePreviewNeverAttachesOrResizes is the black-box half of SPEC
// requirement 22: ticking the preview capture engine repeatedly must never
// leave a tmux client attached and must never change the pane's window
// geometry. The client-facing (deck-process) half of this assertion lands
// with task 022; this proves the tmux primitive itself carries no such
// side effect.
func TestCapturePreviewNeverAttachesOrResizes(t *testing.T) {
	socket := fmt.Sprintf("deck-preview-noattach-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })

	if _, err := client.Create(context.Background(), Launch{
		Slug: "noattach_test", CWD: t.TempDir(),
		Command: []string{"sh", "-c", `i=1; while [ "$i" -le 5 ]; do echo "line-$i"; i=$((i+1)); sleep 0.1; done; sleep 30`},
	}); err != nil {
		t.Fatal(err)
	}

	before, err := client.command(context.Background(), "display-message", "-p", "#{window_width}x#{window_height}").CombinedOutput()
	if err != nil {
		t.Fatalf("read window geometry before preview: %v: %s", err, before)
	}

	for i := 0; i < 10; i++ {
		if _, err := client.CapturePreview(context.Background(), "noattach_test"); err != nil {
			t.Fatalf("capture %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	clients, err := client.command(context.Background(), "list-clients").CombinedOutput()
	if err != nil {
		t.Fatalf("list-clients: %v: %s", err, clients)
	}
	if strings.TrimSpace(string(clients)) != "" {
		t.Fatalf("tmux list-clients = %q after ticking preview, want empty (no attach)", clients)
	}

	after, err := client.command(context.Background(), "display-message", "-p", "#{window_width}x#{window_height}").CombinedOutput()
	if err != nil {
		t.Fatalf("read window geometry after preview: %v: %s", err, after)
	}
	if string(before) != string(after) {
		t.Fatalf("window geometry changed from %q to %q after ten preview captures, want unchanged (no resize)", before, after)
	}
}

func TestCapturePaneRejectsUnsafeOrImplicitRange(t *testing.T) {
	client := Client{Socket: "deck-validation"}
	for _, test := range []struct {
		pane    string
		options CaptureOptions
	}{
		{"deck_session", CaptureOptions{StartLine: "-", EndLine: "-"}},
		{"%1", CaptureOptions{EndLine: "-"}},
		{"%1", CaptureOptions{StartLine: "-", EndLine: "1;kill-server"}},
	} {
		if _, err := client.CapturePane(context.Background(), test.pane, test.options); err == nil {
			t.Fatalf("CapturePane(%q, %#v) accepted unsafe or implicit range", test.pane, test.options)
		}
	}
}

func TestKillStillReportsRealCommandErrors(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "tmux")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'permission denied' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := (Client{Binary: binary, Socket: "deck-errors"}).Kill(context.Background(), "valid_slug")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Kill real command error = %v, want permission-denied diagnostic", err)
	}
}

const attachHelperEnv = "DECK_TMUX_ATTACH_HELPER"

// TestAttachHelper is invoked by TestAttachThroughPTY so Attach owns a real
// terminal rather than the go test process's non-terminal standard streams.
func TestAttachHelper(t *testing.T) {
	if os.Getenv(attachHelperEnv) != "1" {
		return
	}
	client := Client{Socket: os.Getenv("DECK_TMUX_ATTACH_SOCKET")}
	if err := client.Attach(context.Background(), os.Getenv("DECK_TMUX_ATTACH_SLUG")); err != nil {
		t.Fatal(err)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestAttachThroughPTY(t *testing.T) {
	socket := fmt.Sprintf("deck-attach-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	t.Cleanup(func() { _ = client.command(context.Background(), "kill-server").Run() })
	if _, err := client.Create(context.Background(), Launch{
		Slug: "attach_test", CWD: t.TempDir(),
		Command: []string{"sh", "-c", "printf 'deck-attach-live\\n'; sleep 30"},
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAttachHelper$")
	cmd.Env = append(os.Environ(),
		attachHelperEnv+"=1",
		"DECK_TMUX_ATTACH_SOCKET="+socket,
		"DECK_TMUX_ATTACH_SLUG=attach_test",
		"TMUX=/nested,123,0", // Attach must remove this before invoking tmux.
		"TERM=xterm-256color",
	)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = terminal.Close() })
	var output lockedBuffer
	go func() { _, _ = io.Copy(&output, terminal) }()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(output.String(), "deck-attach-live") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := output.String(); !strings.Contains(got, "deck-attach-live") {
		t.Fatalf("attached pane was not visible: %q", got)
	}
	if _, err := terminal.Write([]byte("\x02d")); err != nil { // tmux prefix + detach
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("attach did not return cleanly after detach: %v; output: %q", err, output.String())
	}
	if sessions, err := client.List(context.Background()); err != nil || len(sessions) != 1 {
		t.Fatalf("private server did not survive detach: sessions=%#v err=%v", sessions, err)
	}
}

func TestLifecycleRejectsUnsafeInput(t *testing.T) {
	client := Client{Socket: "deck-validation"}
	if _, err := client.Create(context.Background(), Launch{Slug: "bad.name", CWD: "/tmp", Command: []string{"sh"}}); err == nil {
		t.Fatal("Create accepted a tmux target-syntax slug")
	}
	if err := client.Kill(context.Background(), "bad:name"); err == nil {
		t.Fatal("Kill accepted a tmux target-syntax slug")
	}
}

func TestBootstrapConfiguresOnlyPrivateServer(t *testing.T) {
	socket := fmt.Sprintf("deck-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	client := Client{Socket: socket}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.command(ctx, "kill-server").Run()
	})

	if err := client.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap private server: %v", err)
	}
	for _, check := range []struct {
		args []string
		want string
	}{
		{[]string{"show-options", "-s", "exit-empty"}, "exit-empty off"},
		{[]string{"show-options", "-g", "remain-on-exit"}, "remain-on-exit failed"},
		{[]string{"show-options", "-g", "window-size"}, "window-size latest"},
		{[]string{"show-window-options", "-g", "aggressive-resize"}, "aggressive-resize on"},
	} {
		output, err := client.command(context.Background(), check.args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux -L %s %s: %v\n%s", socket, strings.Join(check.args, " "), err, output)
		}
		if got := strings.TrimSpace(string(output)); got != check.want {
			t.Errorf("tmux -L %s %s = %q, want %q", socket, strings.Join(check.args, " "), got, check.want)
		}
	}

	// A default-socket list must still fail: Bootstrap used only -L socket and
	// therefore did not create the user's interactive tmux server.
	output, err := exec.CommandContext(context.Background(), "tmux", "list-sessions").CombinedOutput()
	if err == nil {
		t.Fatalf("default tmux server unexpectedly responds after private bootstrap: %s", output)
	}
}

func versionScript(t *testing.T, version string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmux")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+version+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
