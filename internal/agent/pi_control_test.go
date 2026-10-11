package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestPiControlFilePathIsPerProfile: the control file lives under the deck home
// of the profile that launched Pi, keyed by the conversation id, and the
// launch hands the extension that directory.
func TestPiControlFilePathIsPerProfile(t *testing.T) {
	a, ok := PiControlFile("/data/deck", "conv-1")
	b, _ := PiControlFile("/data/deck-work", "conv-1")
	if !ok || a != "/data/deck/pi/control/conv-1.jsonl" || b != "/data/deck-work/pi/control/conv-1.jsonl" {
		t.Fatalf("paths = %q, %q", a, b)
	}
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		if path, ok := PiControlFile("/data/deck", id); ok {
			t.Errorf("PiControlFile(%q) = %q, want refused", id, path)
		}
	}
	if _, ok := PiControlFile("", "conv-1"); ok {
		t.Error("an empty deck home produced a control file path")
	}
	_, env := Pi{}.Instrument(LaunchInput{DeckExecutable: "/d", DeckHome: "/data/deck-work"})
	if env[PiControlDirEnv] != "/data/deck-work/pi/control" {
		t.Errorf("control dir env = %q", env[PiControlDirEnv])
	}
	if _, env := (Pi{}).Instrument(LaunchInput{DeckExecutable: "/d"}); env[PiControlDirEnv] != "" {
		t.Errorf("control dir env without a deck home = %q", env[PiControlDirEnv])
	}
}

// TestPiControlFilePushAppendsOneLinePerName: the deck push path appends one
// JSON line per name to an existing control file, sanitised, and never creates
// the file.
func TestPiControlFilePushAppendsOneLinePerName(t *testing.T) {
	home := t.TempDir()
	push := func(name string) (bool, error) {
		return Pi{}.PushControl(ControlPushInput{DeckHome: home, ConversationID: "conv-1", Name: name})
	}
	file, _ := PiControlFile(home, "conv-1")
	if delivered, err := push("first"); delivered || err != nil {
		t.Fatalf("push with no file = %v, %v; want unavailable", delivered, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("push created the file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "se\ncond \"q\""} {
		if delivered, err := push(name); !delivered || err != nil {
			t.Fatalf("push(%q) = %v, %v", name, delivered, err)
		}
	}
	data, _ := os.ReadFile(file)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var got []string
	for _, line := range lines {
		name, ok := DecodePiControlLine([]byte(line))
		if !ok {
			t.Fatalf("unreadable line %q", line)
		}
		got = append(got, name)
	}
	if want := []string{"first", `se cond "q"`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q, want %q (file %q)", got, want, data)
	}
	if delivered, err := push("  "); delivered || err != nil {
		t.Fatalf("a blank name was delivered: %v, %v", delivered, err)
	}
}

// TestPiControlFileFallbackKeystrokes: the send-keys plan is the fallback and
// types /name, never a blank name.
func TestPiControlFileFallbackKeystrokes(t *testing.T) {
	keys, ok := PushName(NewPi(), NamePushInput{Name: "My  name"})
	if want := []string{"\x15", "/name My name", "Enter"}; !ok || !reflect.DeepEqual(keys, want) {
		t.Fatalf("plan = %q, %v; want %q", keys, ok, want)
	}
	if keys, ok := PushName(NewPi(), NamePushInput{Name: " \n"}); ok {
		t.Fatalf("blank name produced %q", keys)
	}
	if delivered, err := PushControl(NewShell(), ControlPushInput{DeckHome: t.TempDir(), ConversationID: "c", Name: "x"}); delivered || err != nil {
		t.Fatalf("shell has a control channel: %v, %v", delivered, err)
	}
}

// TestPiControlFileExtensionLifecycle pins the extension text: the file is
// opened keyed by Pi's session id in session_start, closed in session_shutdown,
// and each line reaches pi.setSessionName.
func TestPiControlFileExtensionLifecycle(t *testing.T) {
	src := PiExtensionSource
	startAt := strings.Index(src, `pi.on("session_start"`)
	shutdownAt := strings.Index(src, `pi.on("session_shutdown"`)
	if startAt < 0 || shutdownAt < startAt {
		t.Fatal("extension lacks session_start/session_shutdown handlers")
	}
	start := src[startAt:shutdownAt]
	if !strings.Contains(start, "openControl(") || !strings.Contains(start, "getSessionId()") {
		t.Errorf("session_start does not open the control file by session id:\n%s", start)
	}
	end := src[shutdownAt:]
	infoAt := strings.Index(end, `pi.on("session_info_changed"`)
	if infoAt < 0 || !strings.Contains(end[:infoAt], "closeControl()") {
		t.Error("session_shutdown does not close the control file")
	}
	for _, want := range []string{"pi.setSessionName(name)", "process.env." + PiControlDirEnv, `fs.openSync(file, "w+"`, "fs.unlinkSync(open.file)"} {
		if !strings.Contains(src, want) {
			t.Errorf("extension lacks %q", want)
		}
	}
}

// TestPiControlFileExtensionUnderNode runs the real extension text under node
// against a stub pi: session_start creates the control file, a line deck
// appends reaches setSessionName, session_shutdown removes the file. Skipped
// where node is not installed (the CI image has none).
func TestPiControlFileExtensionUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	ext := filepath.Join(dir, "deck-hook.js")
	if err := os.WriteFile(ext, []byte(PiExtensionSource), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	file, _ := PiControlFile(home, "conv-1")
	driver := filepath.Join(dir, "driver.mjs")
	script := `import fs from "node:fs";
const handlers = {};
const names = [];
const pi = { on: (e, h) => { handlers[e] = h; }, getSessionName: () => "", setSessionName: (n) => names.push(n) };
const mod = await import(process.argv[2]);
mod.default(pi);
const ctx = { sessionManager: { getSessionId: () => "conv-1" } };
handlers.session_start({ reason: "startup" }, ctx);
const file = process.argv[3];
if (!fs.existsSync(file)) throw new Error("control file not created");
fs.appendFileSync(file, '{"name":"from deck"}\n{"name":"two"}\nnot json\n{"name":"par');
await new Promise((r) => setTimeout(r, 600));
fs.appendFileSync(file, 'tial"}\n');
await new Promise((r) => setTimeout(r, 600));
handlers.session_shutdown({ reason: "quit" }, ctx);
if (fs.existsSync(file)) throw new Error("control file survived shutdown");
console.log(JSON.stringify(names));
`
	if err := os.WriteFile(driver, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, driver, ext, file)
	cmd.Env = append(os.Environ(), PiControlDirEnv+"="+PiControlDir(home), PiHookExecutableEnv+"=true")
	timer := time.AfterFunc(30*time.Second, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != `["from deck","two","partial"]` {
		t.Fatalf("setSessionName calls = %s", got)
	}
}
