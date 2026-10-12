package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// PiControlDirEnv names the environment variable that carries the directory
// the deck extension keeps its per-session control files in (SPEC §8.3, R263).
// It is under the launching profile's own deck home, so one profile never
// reaches another profile's Pi.
const PiControlDirEnv = "DECK_PI_CONTROL_DIR"

// PiControlDir is the directory of Pi control files under deckHome.
func PiControlDir(deckHome string) string {
	return filepath.Join(deckHome, "pi", "control")
}

// PiControlFile is the control file of the Pi conversation id, under deckHome.
// ok=false when either is empty or the id is not a single safe path component.
func PiControlFile(deckHome, conversationID string) (string, bool) {
	if deckHome == "" || !safeConversationID(conversationID) {
		return "", false
	}
	return filepath.Join(PiControlDir(deckHome), conversationID+".jsonl"), true
}

type piControlLine struct {
	Name string `json:"name"`
}

// EncodePiControlName is one control-file line: a JSON object with the name,
// then a newline. It is the only thing deck writes to the file.
func EncodePiControlName(name string) []byte {
	encoded, _ := json.Marshal(piControlLine{Name: name})
	return append(encoded, '\n')
}

// DecodePiControlLine reads one control-file line back; ok=false for a line
// that is not a JSON object with a non-blank name (the extension ignores it).
func DecodePiControlLine(line []byte) (string, bool) {
	var parsed piControlLine
	if err := json.Unmarshal(line, &parsed); err != nil || strings.TrimSpace(parsed.Name) == "" {
		return "", false
	}
	return parsed.Name, true
}

// ControlPushInput identifies the conversation and the name to hand it.
type ControlPushInput struct {
	// DeckHome is the launching profile's resolved data root.
	DeckHome string
	// ConversationID is the bound conversation.
	ConversationID string
	// Name is the already-sanitised new name; never blank.
	Name string
}

// ControlPusher is the optional interface of an adapter that can hand a rename
// to a running agent through a file its injected extension watches, which
// needs no keystroke and no idle pane. delivered=false with a nil error means
// the channel is unavailable (the extension has not opened its file, or has
// closed it), and the caller falls back to PushName's keystroke plan.
type ControlPusher interface {
	PushControl(in ControlPushInput) (delivered bool, err error)
}

// PushControl appends the name to the session's control file, which the deck
// extension created in session_start and removes in session_shutdown. A file
// that does not exist, is not a regular file, or whose owner cannot be shown
// to be running (see controlFileLive) is "unavailable"; a file whose owner is
// shown dead is removed, so the push falls back to the keystroke plan instead
// of reporting a write nobody will read as sent.
func (Pi) PushControl(in ControlPushInput) (bool, error) {
	path, ok := PiControlFile(in.DeckHome, in.ConversationID)
	if !ok {
		return false, nil
	}
	name, ok := SanitisePushName("pi", in.Name)
	if !ok {
		return false, nil
	}
	live, err := controlFileLive(path)
	if !live || err != nil {
		return false, err
	}
	return appendControlLine(path, EncodePiControlName(name))
}

// piControlPidLine is the first line of a control file: the extension writes
// {"pid":<Pi's process id>} when it creates the file (SPEC §8.3, R269). The
// extension ignores it as it ignores any line without a name.
type piControlPidLine struct {
	Pid int `json:"pid"`
}

// EncodePiControlPid is the pid line the extension writes first.
func EncodePiControlPid(pid int) []byte {
	encoded, _ := json.Marshal(piControlPidLine{Pid: pid})
	return append(encoded, '\n')
}

// decodePiControlPid reads the pid out of a control file's first line.
func decodePiControlPid(first []byte) (int, bool) {
	var parsed piControlPidLine
	if err := json.Unmarshal(first, &parsed); err != nil || parsed.Pid <= 1 {
		return 0, false
	}
	return parsed.Pid, true
}

// controlFileLive reports whether path is a regular file whose owning Pi is
// shown running: its first line carries a pid and a signal-0 probe of that pid
// succeeds. A missing file, another kind of file, a file with no pid line yet
// (the extension is between creating it and writing the line) or a pid the
// probe cannot reach are all "not live" and change nothing, except a pid the
// probe reports gone: that file was left by a dead Pi and is removed.
func controlFileLive(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pi control file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	pid, ok := readControlPid(path)
	if !ok {
		return false, nil
	}
	if processRunning(pid) {
		return true, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove stale pi control file: %w", err)
	}
	return false, nil
}

// readControlPid reads the first line of the control file without following a
// symlink or blocking on a non-regular file.
func readControlPid(path string) (int, bool) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) //nolint:gosec // G304: path is PiControlFile of a validated conversation id under the profile's deck home
	if err != nil {
		return 0, false
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, 128)
	n, _ := file.Read(buf)
	end := strings.IndexByte(string(buf[:n]), '\n')
	if end < 0 {
		return 0, false
	}
	return decodePiControlPid(buf[:end])
}

// processRunning is the signal-0 probe: a process that exists (even one this
// user may not signal) is running, one that does not is gone.
func processRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// appendControlLine appends one line to an existing control file; a file that
// vanished between the check and the open is unavailable. It never follows a
// symlink at path.
func appendControlLine(path string, line []byte) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0) //nolint:gosec // G304: path is PiControlFile of a validated conversation id under the profile's deck home
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open pi control file: %w", err)
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("write pi control file: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close pi control file: %w", err)
	}
	return true, nil
}

// piClearLine is Ctrl-U, which empties Pi's input line before the command is typed.
const piClearLine = "\x15"

// PushName is Pi's keystroke fallback (R263), used only when the control file
// is unavailable: clear the input line, type `/name <name>`, then a lone Enter.
func (Pi) PushName(in NamePushInput) (keys []string, ok bool) {
	name, ok := SanitisePushName("pi", in.Name)
	if !ok {
		return nil, false
	}
	return []string{piClearLine, "/name " + name, "Enter"}, true
}

// PushControl is the dispatcher for ControlPusher: an adapter that does not
// implement it, or whose caps do not claim a push, has no control channel.
func PushControl(a Adapter, in ControlPushInput) (delivered bool, err error) {
	pusher, ok := a.(ControlPusher)
	if !ok || !a.Capabilities().Name.ClaimsPush() {
		return false, nil
	}
	return pusher.PushControl(in)
}
