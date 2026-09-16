package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/agent"
)

// registerCodexHooksSteps backs SPEC §13.4's @codex scenario (R127, task
// 024): the window between a Codex row's creation and its first prompt,
// when it carries no conversation id at all, and the two hook-driven
// transitions that follow -- SessionStart's id adoption and
// PermissionRequest's tool-named "waiting". It drives the fake-codex
// fixture's own pane-command surface (cmd/fake-codex's "prompt"/
// "permission" commands, task 021) exactly the way registerClaudeHookStatusSteps
// drives fake-claude's "hook" command. codexTranscriptPathForConversationID
// below duplicates cmd/fake-codex's own rollout-file convention (never
// internal/agent/codex.go's TranscriptPaths) purely as a helper for the
// existing cross-transcript-mention check; sessionCodexPersistedIdentityMatchesPaneAnnouncement
// (task 008, B3) is the one place this file DOES import internal/agent --
// deliberately, to compare the STORED row and PRODUCTION's own
// Codex.TranscriptPaths seam against each pane's independently-captured
// SessionStart announcement, never against the store's or the glob's own
// idea of where the transcript lives.
func registerCodexHooksSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the deck config probes agent panes quickly$`, deckConfigProbesQuickly)
	sc.Step(`^the state database session "([^"]+)" has no conversation id$`, sessionHasNoConversationID)
	sc.Step(`^fake Codex session "([^"]+)" is prompted with "([^"]*)"$`, fakeCodexIsPromptedWith)
	sc.Step(`^fake Codex session "([^"]+)" requests approval to run tool "([^"]+)"$`, fakeCodexRequestsApprovalForTool)
	sc.Step(`^session "([^"]+)"'s codex transcript does not mention session "([^"]+)"'s conversation id$`, sessionCodexTranscriptDoesNotMentionOthersConversationID)
	sc.Step(`^session "([^"]+)"'s persisted conversation id and transcript path match its own pane-announced SessionStart identity$`, sessionCodexPersistedIdentityMatchesPaneAnnouncement)
	sc.Step(`^within 3 seconds deck client "([^"]+)" row "([^"]+)" contains "([^"]+)"$`, clientRowContainsWithinThreeSeconds)
	sc.Step(`^the state database sessions "([^"]+)" and "([^"]+)" persisted the same working directory$`, sessionsPersistedSameWorkingDirectory)
	sc.Step(`^the state database sessions "([^"]+)" and "([^"]+)" were created within 2 seconds of each other$`, sessionsCreatedWithinTwoSeconds)
}

// deckConfigProbesQuickly writes stale_after = 1 (the schema's own declared
// minimum, internal/config/schema.go) to the scenario's config.toml, so a
// freshly created row (StatusAt = its own creation time) becomes probe-
// eligible (internal/service/reconcile.go's probeEligible) after 1 real
// second rather than the schema's 45s default -- this feature has no need
// for status_probe.feature's own frozen/steppable DECK_CLOCK machinery,
// since it never asserts precedence at the exact stale_after boundary,
// only that a sampled verdict eventually appears. Must run before "deck
// client ... is started": config.toml is read once, at client startup.
func deckConfigProbesQuickly(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "config.toml")
	if err := os.WriteFile(path, []byte("stale_after = 1\n"), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml: %w", err)
	}
	return nil
}

// clientRowContainsWithinThreeSeconds is status_probe_test.go's
// clientRowContainsWithinReconcile widened to a fixed 3s deadline -- long
// enough to cross deckConfigProbesQuickly's 1s stale_after floor plus at
// least one reconcile tick, which the reconcile-interval-only wait
// (scenarioReconcileInterval + 250ms = 500ms) cannot reach.
func clientRowContainsWithinThreeSeconds(ctx context.Context, clientName, rowName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, line := range strings.Split(client.Frame(false), "\n") {
			if strings.Contains(line, rowName) && strings.Contains(line, want) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q row %q did not contain %q within 3 seconds\nframe:\n%s", clientName, rowName, want, client.Frame(false))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sessionHasNoConversationID is sessionHasNonEmptyConversationID's negative
// counterpart: proves a just-created codex row mints nothing at launch
// (SPEC §8.2) -- unlike claude/pi, which sessionHasNonEmptyConversationID
// already covers.
func sessionHasNoConversationID(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if id != "" {
		return fmt.Errorf("session %q has conversation id %q, want none yet", name, id)
	}
	return nil
}

// codexPaneAnnouncement is one Codex pane's own authoritative SessionStart
// identity (task 008, B3): its own session_id and its own transcript_path,
// both taken directly off cmd/fake-codex's banner lines -- never off the
// state database, never off a glob keyed on a stored id.
type codexPaneAnnouncement struct {
	SessionID      string
	TranscriptPath string
}

// codexPaneAnnouncementFromCapture scans a tmux pane capture for
// cmd/fake-codex's two SessionStart-adjacent banner lines
// ("fake-codex session-id: <uuid>" and "fake-codex transcript-path:
// <path>", both emitted once, together, the moment submitPrompt mints or
// resumes a conversation) and returns whatever it found. ok is true only
// once the session-id line itself has appeared -- the transcript-path line
// is optional (cmd/fake-codex omits it when its own CODEX_HOME/HOME
// resolution degrades to "nowhere writable"), so TranscriptPath may be ""
// even when ok is true.
func codexPaneAnnouncementFromCapture(capture string) (announcement codexPaneAnnouncement, ok bool) {
	for _, line := range strings.Split(capture, "\n") {
		if id, found := strings.CutPrefix(line, "fake-codex session-id: "); found {
			announcement.SessionID = id
		}
		if path, found := strings.CutPrefix(line, "fake-codex transcript-path: "); found {
			announcement.TranscriptPath = path
		}
	}
	return announcement, announcement.SessionID != ""
}

// fakeCodexIsPromptedWith sends cmd/fake-codex's "prompt" pane command
// (task 021) into the named session's private tmux pane, which mints (or
// reuses, on a `resume <id>` invocation) that invocation's one conversation
// id, fires SessionStart on the very first call and UserPromptSubmit on
// every call, and writes the rollout transcript's session_meta line. It
// polls the pane's own echoed "fake-codex session-id: <uuid>" banner line
// (cmd/fake-codex's submitPrompt) rather than returning as soon as
// send-keys completes, so a caller relying on the id having been minted
// (e.g. a later transcript-path lookup) never races the fixture's own
// asynchronous pane output. It also captures BOTH that line and the
// adjacent "fake-codex transcript-path: <path>" banner into
// h.codexPaneAnnouncements[name] (task 008) -- this pane's own
// authoritative SessionStart identity, held independently of anything the
// store ever learns, for a later step to compare the store's and
// production's own ideas of that identity against.
func fakeCodexIsPromptedWith(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	request, err := json.Marshal(map[string]any{"command": "prompt", "text": text})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send prompt command to fake Codex pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit prompt command to fake Codex pane %q: %w", target, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		// -J joins tmux's own soft-wrapped physical rows back into one
		// logical line (status_probe_test.go's own convention) -- without
		// it, a transcript path longer than the pane's own width would be
		// split mid-path across two capture-pane lines, and neither half
		// would carry the "fake-codex transcript-path: " prefix intact.
		output, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		if err == nil {
			if announcement, ok := codexPaneAnnouncementFromCapture(string(output)); ok {
				if h.codexPaneAnnouncements == nil {
					h.codexPaneAnnouncements = make(map[string]codexPaneAnnouncement)
				}
				h.codexPaneAnnouncements[name] = announcement
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fake Codex pane %q never announced a minted session id; last capture (err=%v):\n%s", target, err, string(output))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// fakeCodexRequestsApprovalForTool sends cmd/fake-codex's "permission" pane
// command, firing PermissionRequest with tool_name (task 018's own receiver
// mapping: PermissionRequest -> "waiting", ReasonField "tool_name") and
// printing the real approval-prompt text codex's own "waiting" probe rule
// keys on (task 019). name must already have been prompted at least once
// (cmd/fake-codex's requestPermission refuses otherwise).
func fakeCodexRequestsApprovalForTool(ctx context.Context, name, toolName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&before); err != nil {
		return err
	}
	request, err := json.Marshal(map[string]any{"command": "permission", "tool_name": toolName})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send permission command to fake Codex pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit permission command to fake Codex pane %q: %w", target, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
			return err
		}
		if count > before {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
			return fmt.Errorf("fake Codex pane %q did not persist PermissionRequest; pane:\n%s", target, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// codexTranscriptPathForConversationID mirrors cmd/fake-codex's own
// writeRolloutSessionMeta convention (also internal/agent/codex.go's
// TranscriptPaths, task 015's provenance) against the scenario's own
// fixture HOME (h.agentHOMEDir, which lifecycle_test.go passes as every
// subsequently started named client's HOME=): <HOME>/.codex/sessions/
// <yyyy>/<mm>/<dd>/rollout-<ISO>-<conversationID>.jsonl. It globs on
// everything but the id-bearing filename suffix, since the date directories
// and timestamp prefix are wall-clock-derived and not reproduced here.
func codexTranscriptPathForConversationID(h *ScenarioHarness, conversationID string) (string, error) {
	if h.agentHOMEDir == "" {
		return "", fmt.Errorf("fixture HOME directory was not configured (call the fake codex PATH step first)")
	}
	if conversationID == "" {
		return "", fmt.Errorf("empty conversation id")
	}
	pattern := filepath.Join(h.agentHOMEDir, ".codex", "sessions", "*", "*", "*", "rollout-*-"+conversationID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob codex rollout transcript for id %q: %w", conversationID, err)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("codex rollout transcript glob %q matched %d files, want 1: %v", pattern, len(matches), matches)
	}
	return matches[0], nil
}

// sessionCodexTranscriptDoesNotMentionOthersConversationID resolves name's
// own rollout transcript file (by name's own conversation id, exactly the
// filename cmd/fake-codex minted it under) and asserts other's conversation
// id string never appears anywhere in its bytes -- proving the two
// sessions created in one working directory (this feature's own scenario)
// never cross-wrote or cross-referenced each other's transcript (SPEC
// §13.4's "neither row's id is the other's transcript").
func sessionCodexTranscriptDoesNotMentionOthersConversationID(ctx context.Context, name, other string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	selfID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if selfID == "" {
		return fmt.Errorf("session %q has no conversation id", name)
	}
	otherID, err := sessionConversationID(h, other)
	if err != nil {
		return err
	}
	if otherID == "" {
		return fmt.Errorf("session %q has no conversation id", other)
	}
	path, err := codexTranscriptPathForConversationID(h, selfID)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read codex transcript %q for session %q: %w", path, name, err)
	}
	if strings.Contains(string(content), otherID) {
		return fmt.Errorf("session %q's codex transcript %q unexpectedly mentions session %q's conversation id %q", name, path, other, otherID)
	}
	return nil
}

// sessionCWD reads a session's own persisted cwd straight from the
// sessions table, exactly like sessionConversationID/sessionIDByName above
// -- production's own idea of where that session runs, never the pane's.
func sessionCWD(h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var cwd string
	if err := db.QueryRow(`SELECT cwd FROM sessions WHERE name = ?`, name).Scan(&cwd); err != nil {
		return "", fmt.Errorf("observe session %q cwd: %w", name, err)
	}
	return cwd, nil
}

// sessionCreatedAt reads a session's own persisted created_at millisecond
// timestamp straight off the sessions table -- production's own clock read
// at store.CreateSession time (internal/service/agent.go's `s.Clock.Now().
// UnixMilli()`), never scenario wall-clock prose.
func sessionCreatedAt(h *ScenarioHarness, name string) (int64, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var createdAt int64
	if err := db.QueryRow(`SELECT created_at FROM sessions WHERE name = ?`, name).Scan(&createdAt); err != nil {
		return 0, fmt.Errorf("observe session %q created_at: %w", name, err)
	}
	return createdAt, nil
}

// sessionsPersistedSameWorkingDirectory asserts both named sessions' own
// persisted cwd column (sessionCWD, never scenario prose) name the same
// directory -- this scenario's own two codex rows are created back-to-back
// through one deck client without the working-directory field ever being
// changed (positionCreateModalOnProfileField's own h.workingDir reuse), so
// production's persisted idea of each row's cwd should agree.
func sessionsPersistedSameWorkingDirectory(ctx context.Context, first, second string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	firstCWD, err := sessionCWD(h, first)
	if err != nil {
		return err
	}
	secondCWD, err := sessionCWD(h, second)
	if err != nil {
		return err
	}
	if firstCWD == "" || secondCWD == "" {
		return fmt.Errorf("session %q or %q has an empty persisted cwd (%q, %q)", first, second, firstCWD, secondCWD)
	}
	if firstCWD != secondCWD {
		return fmt.Errorf("sessions %q and %q persisted different working directories: %q vs %q", first, second, firstCWD, secondCWD)
	}
	return nil
}

// sessionsCreatedWithinTwoSeconds asserts both named sessions' own persisted
// created_at millisecond timestamps (sessionCreatedAt, store.CreateSession's
// own clock read) land within 2 seconds of each other -- measured from the
// store's own recorded creation timestamps, never from how long the
// scenario's own steps took to run.
func sessionsCreatedWithinTwoSeconds(ctx context.Context, first, second string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	firstCreatedAt, err := sessionCreatedAt(h, first)
	if err != nil {
		return err
	}
	secondCreatedAt, err := sessionCreatedAt(h, second)
	if err != nil {
		return err
	}
	delta := firstCreatedAt - secondCreatedAt
	if delta < 0 {
		delta = -delta
	}
	const twoSecondsMillis = 2000
	if delta > twoSecondsMillis {
		return fmt.Errorf("sessions %q and %q were created %dms apart (created_at %d, %d), want within %dms", first, second, delta, firstCreatedAt, secondCreatedAt, twoSecondsMillis)
	}
	return nil
}

// codexIdentityMismatch is B3's two-sided oracle itself (task 008): it
// compares a row's own STORED conversation id, and the path PRODUCTION's
// own shipped internal/agent Codex.TranscriptPaths seam resolves for that
// id, against a pane's own independently-captured SessionStart
// announcement -- entirely independently of each other, so a caller wrong
// on one side is never masked by the other side happening to be right.
// resolvedPath is returned whenever TranscriptPaths itself resolved
// something (even alongside a non-nil pathErr, when what it resolved
// merely doesn't match the announcement), so a caller wanting the
// strongest check -- reading the file production actually named and
// inspecting its own session_meta line -- still has a path to open.
func codexIdentityMismatch(home, storedConversationID string, announcement codexPaneAnnouncement) (resolvedPath string, idErr, pathErr error) {
	if storedConversationID != announcement.SessionID {
		idErr = fmt.Errorf("stored conversation id %q does not match the pane's own announced session_id %q", storedConversationID, announcement.SessionID)
	}
	path, ok := agent.NewCodex().TranscriptPaths(agent.TranscriptInput{
		Home:           home,
		ConversationID: storedConversationID,
	})
	if !ok {
		pathErr = fmt.Errorf("production TranscriptPaths declined to resolve a path for stored conversation id %q", storedConversationID)
		return "", idErr, pathErr
	}
	if path != announcement.TranscriptPath {
		pathErr = fmt.Errorf("production-resolved transcript path %q does not match the pane's own announced transcript_path %q", path, announcement.TranscriptPath)
	}
	return path, idErr, pathErr
}

// codexTranscriptSessionMetaMatches reads path's own first line -- the
// session_meta line cmd/fake-codex's writeRolloutSessionMeta writes,
// exactly the file PRODUCTION resolution named -- and asserts it carries
// wantSessionID and wantCWD, proving the resolved file is not merely a
// byte-identical path string but is itself the transcript that session
// actually started (SPEC §8.2's own session_meta convention).
func codexTranscriptSessionMetaMatches(path, wantSessionID, wantCWD string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read codex transcript %q: %w", path, err)
	}
	firstLine, _, _ := strings.Cut(string(content), "\n")
	var meta struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(firstLine), &meta); err != nil {
		return fmt.Errorf("decode codex transcript %q session_meta line %q: %w", path, firstLine, err)
	}
	if meta.SessionID != wantSessionID {
		return fmt.Errorf("codex transcript %q session_meta session_id = %q, want %q", path, meta.SessionID, wantSessionID)
	}
	if meta.CWD != wantCWD {
		return fmt.Errorf("codex transcript %q session_meta cwd = %q, want %q", path, meta.CWD, wantCWD)
	}
	return nil
}

// sessionCodexPersistedIdentityMatchesPaneAnnouncement is B3's own feature
// step (task 008): it ties name's row to its own pane, both ways --
// asserting the STORED conversation id equals that pane's own announced
// session_id, and that PRODUCTION's own TranscriptPaths resolution for the
// stored id is exactly that pane's own announced transcript_path, whose
// first-line session_meta itself carries the same session_id and the
// session's own stored cwd. codex_hooks_swap_test.go's
// TestCodexIdentityMismatchCatchesSwappedStoredIDs feeds codexIdentityMismatch
// (the comparison this step calls) the two scenario rows' stored ids
// swapped and proves it fails on both halves -- the negative control this
// step's own oracle needs to be attribution-sensitive rather than merely
// distinctness-sensitive.
func sessionCodexPersistedIdentityMatchesPaneAnnouncement(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	announcement, ok := h.codexPaneAnnouncements[name]
	if !ok {
		return fmt.Errorf("session %q has no captured pane SessionStart announcement (call the prompt step first)", name)
	}
	storedID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	storedCWD, err := sessionCWD(h, name)
	if err != nil {
		return err
	}
	resolvedPath, idErr, pathErr := codexIdentityMismatch(h.agentHOMEDir, storedID, announcement)
	if idErr != nil {
		return idErr
	}
	if pathErr != nil {
		return pathErr
	}
	return codexTranscriptSessionMetaMatches(resolvedPath, announcement.SessionID, storedCWD)
}
