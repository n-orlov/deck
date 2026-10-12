package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// that does not exist, or is not a regular file, is "unavailable".
func (Pi) PushControl(in ControlPushInput) (bool, error) {
	path, ok := PiControlFile(in.DeckHome, in.ConversationID)
	if !ok {
		return false, nil
	}
	name, ok := SanitisePushName("pi", in.Name)
	if !ok {
		return false, nil
	}
	usable, err := controlFileUsable(path)
	if !usable || err != nil {
		return false, err
	}
	return appendControlLine(path, EncodePiControlName(name))
}

// controlFileUsable reports whether path is an existing regular file; a file
// that is missing or of another kind is unavailable, not an error.
func controlFileUsable(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pi control file: %w", err)
	}
	return info.Mode().IsRegular(), nil
}

// appendControlLine appends one line to an existing control file; a file that
// vanished between the check and the open is unavailable.
func appendControlLine(path string, line []byte) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) //nolint:gosec // G304: path is PiControlFile of a validated conversation id under the profile's deck home
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
