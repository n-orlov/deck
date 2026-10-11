package agent

import (
	"strings"
	"time"
)

// CodexRenameGap is the pause between the typed `/rename <name>` and its
// Enter (R257). Codex's composer treats text and an Enter that arrive together
// as a paste and keeps the Enter as a newline, so the submit follows after
// this gap. It is a measured value, best effort: a slower machine may need
// more.
const CodexRenameGap = 700 * time.Millisecond

// codexModalMarkers are the texts of the full-screen modals Codex draws over
// its composer: the update prompt, the directory trust prompt and an approval
// prompt. While any is up the keyboard belongs to the modal.
var codexModalMarkers = []string{
	"Update available!",
	"Skip until next version",
	"Do you trust the contents of this directory",
	"Press enter to confirm or esc to cancel",
}

// PushName is Codex's rename keystroke plan (R257): `/rename <name>` typed
// as literal text, then a lone Enter. Service sends the Enter after PushGap, so
// Codex sees the command typed before it is submitted. A name with nothing
// left once cleaned is declined.
func (Codex) PushName(in NamePushInput) (keys []string, ok bool) {
	name, ok := SanitisePushName("codex", in.Name)
	if !ok {
		return nil, false
	}
	return []string{"/rename " + name, "Enter"}, true
}

// PushGap is the pause before the plan's final key (CodexRenameGap).
func (Codex) PushGap() time.Duration { return CodexRenameGap }

// DialogFocused reports whether an update, trust or approval modal is on the
// screen.
func (Codex) DialogFocused(screen string) bool {
	for _, marker := range codexModalMarkers {
		if strings.Contains(screen, marker) {
			return true
		}
	}
	return false
}

// PromptReady reports whether the probe sees Codex's empty composer with no
// modal over it: the idle or the first-launch screen, which both show the
// "Ask Codex to do anything" placeholder. A working turn, an error, a typed
// draft (the placeholder is gone) and any other screen are not ready.
func (c Codex) PromptReady(screen string) bool {
	if c.DialogFocused(screen) {
		return false
	}
	status, _ := c.Probe(screen)
	return status == "idle" || status == "starting"
}
