package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// piSpawnCall pulls the extension's one spawnSync call out of the source: the
// callee's file argument as written, and its argv array decoded.
func piSpawnCall(t *testing.T) (file string, args []string) {
	t.Helper()
	match := regexp.MustCompile(`spawnSync\(([^,()]+), (\[[^\]]*\]), \{`).FindStringSubmatch(PiExtensionSource)
	if match == nil {
		t.Fatalf("the extension has no spawnSync(<file>, [<argv>], {...}) call:\n%s", PiExtensionSource)
	}
	if err := json.Unmarshal([]byte(match[2]), &args); err != nil {
		t.Fatalf("spawnSync argv %s is not a literal string array: %v", match[2], err)
	}
	return strings.TrimSpace(match[1]), args
}

// R213 item 4: the Pi extension runs the hook as the executable path and
// `_hook` in separate argv elements, never through `sh -c` with a joined
// string. The file the extension spawns is read from PiHookExecutableEnv as
// the launch wrote it; here that is a path holding a space, a quote and a `;`,
// and the process the extension's own argv starts must receive exactly
// [path, "_hook"]. A shell in between would split on the space, drop the
// quote or end the command at the `;`, so the argv would not arrive intact.
func TestPiExtensionRunsTheHookExecutableWithSeparateArgvAndNoShell(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deck builds", `it's "x"; touch pwned`)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "deck")
	record := filepath.Join(t.TempDir(), "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$0\" \"$@\" > '" + record + "'\n"
	if err := os.WriteFile(executable, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, env := Pi{}.Instrument(LaunchInput{DeckExecutable: executable, DeckHome: "/deck-home"})
	if env[PiHookExecutableEnv] != executable {
		t.Fatalf("%s = %q, want the raw path %q (nothing quotes it for a shell)", PiHookExecutableEnv, env[PiHookExecutableEnv], executable)
	}
	file, args := piSpawnCall(t)
	if file != "executable" || !strings.Contains(PiExtensionSource, "const executable = process.env."+PiHookExecutableEnv+";") {
		t.Fatalf("the extension spawns %q; want the executable read from %s", file, PiHookExecutableEnv)
	}
	if !reflect.DeepEqual(args, []string{"_hook"}) || !reflect.DeepEqual(args, PiHookArgs) {
		t.Fatalf("the extension's argv = %#v, want exactly [\"_hook\"]", args)
	}
	for _, shell := range []string{"/bin/sh", `"-c"`, "shell:", "command +", "${command}"} {
		if strings.Contains(PiExtensionSource, shell) {
			t.Errorf("the extension involves a shell or a joined command string: it contains %q", shell)
		}
	}

	if out, err := exec.Command(env[PiHookExecutableEnv], args...).CombinedOutput(); err != nil {
		t.Fatalf("run the hook executable: %v\n%s", err, out)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if want := executable + "\n_hook\n"; string(got) != want {
		t.Fatalf("hook received argv %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Fatal("the `;` in the path ran a second command")
	}
}
