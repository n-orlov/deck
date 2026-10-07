package hookrecv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Origin is where a hook call came from: the pane environment the `_hook`
// process inherited. The zero value is a Claude/Codex/Pi call, which names its
// event inside the payload.
type Origin struct {
	// SessionID is DECK_SESSION_ID, the deck row identity.
	SessionID string
	// LaunchGeneration is DECK_LAUNCH_GENERATION.
	LaunchGeneration string
	// Event is DECK_HOOK_EVENT. Copilot's payloads mostly carry no event name,
	// so every hook entry of deck's plugin names its event here; a non-empty
	// value selects copilotMappings and Copilot's camelCase payload.
	Event string
	// CopilotRoot is the resolved copilot root (COPILOT_HOME, else
	// ~/.copilot) an agentStop transcriptPath must sit under.
	CopilotRoot string
}

// CopilotRoot resolves the copilot root the way the adapter does: COPILOT_HOME
// when non-empty, else <home>/.copilot, else empty (nothing can be under it).
func CopilotRoot(getenv func(string) string, userHome func() (string, error)) string {
	if root := getenv("COPILOT_HOME"); root != "" {
		return root
	}
	if home, err := userHome(); err == nil && home != "" {
		return filepath.Join(home, ".copilot")
	}
	return ""
}

const (
	copilotNotificationKind = "notification"
	// identityMismatchSuffix marks the one event a payload whose sessionId is
	// not the row's conversation id leaves behind.
	identityMismatchSuffix = ".identity_mismatch"
)

// copilotMappings is the hook table of Copilot's six observational events,
// keyed by the DECK_HOOK_EVENT value. notification and errorOccurred are
// refined by copilotMapping: only two notification types are a status.
var copilotMappings = map[string]Mapping{
	"userPromptSubmitted": {Status: "running", Kind: "user_prompt_submitted", Reason: "prompt"},
	"sessionStart":        {Status: "running", Kind: "session_start", ReasonField: "source"},
	"notification":        {Status: "waiting", Kind: copilotNotificationKind, ReasonField: "notification_type"},
	"agentStop":           {Status: "idle", Kind: "stop", ReasonField: "stopReason", Reason: "end_turn"},
	"errorOccurred":       {Kind: "error_occurred"},
	"sessionEnd":          {Status: "stopped", Kind: "session_end", ReasonField: "reason"},
}

// copilotWaitingNotifications are the notification_type values that are a
// status; every other type changes nothing.
var copilotWaitingNotifications = map[string]bool{"permission_prompt": true, "elicitation_dialog": true}

// copilotMapping returns the mapping of one Copilot event. A mapping with an
// empty Status is recorded as an event and never applied.
func copilotMapping(event string, p payload) (Mapping, bool) {
	mapping, ok := copilotMappings[event]
	if ok && event == "notification" && !copilotWaitingNotifications[p.Notification] {
		return Mapping{Kind: copilotNotificationKind}, true
	}
	return mapping, ok
}

// copilotMismatch reports whether a Copilot payload names a conversation other
// than the row's: /clear, /new, /resume and /fork swap the live id, and a
// status from the new conversation must not be written onto this row.
func copilotMismatch(p payload, conversationID string) bool {
	return p.copilot && p.ConversationID != "" && p.ConversationID != conversationID
}

// mismatchReason is the stored reason of an identity-mismatch event.
func mismatchReason(rowConversationID, payloadID string) string {
	return fmt.Sprintf("declined: payload sessionId %q does not match row conversation id %q", payloadID, rowConversationID)
}

// recordCopilotTranscript stores the transcriptPath an applied agentStop
// named, when it passes transcriptUnderRoot. A rejected path is dropped
// silently: the hook is observational and the status already landed.
func recordCopilotTranscript(ctx context.Context, db Store, p payload, root, sessionID string, at int64) error {
	if !p.copilot || p.EventName != "agentStop" {
		return nil
	}
	path, ok := transcriptUnderRoot(p.TranscriptPath, root)
	if !ok {
		return nil
	}
	if err := db.RecordTranscript(ctx, sessionID, path, at); err != nil {
		return fmt.Errorf("record transcript: %w", err)
	}
	return nil
}

// transcriptUnderRoot returns the path when it is a regular file that,
// after symlinks, lies inside the resolved root.
func transcriptUnderRoot(path, root string) (string, bool) {
	if path == "" || root == "" || !filepath.IsAbs(path) {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}
