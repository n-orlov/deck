package features

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerHarnessNamePiSteps backs the Pi @harness-names scenarios (R264). Pi
// keeps a session's name as session_info entries in its session file, reports a
// change through the deck extension's hooks, and takes deck's push through the
// extension's control file. The steps assert through that file, the fake's pane
// and deck's store and screen, polling each with a deadline and never pausing
// for a fixed time.
func registerHarnessNamePiSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" creates pi session with a blank name expecting the derived name "([^"]+)"$`, func(ctx context.Context, client, derived string) error {
		return clientCreatesAgentSessionWithBlankName(ctx, client, "pi", derived)
	})
	sc.Step(`^fake Pi session "([^"]+)" starts its session$`, fakePiStartsSession)
	sc.Step(`^fake Pi session "([^"]+)" is prompted and finishes its turn$`, fakePiPromptedAndFinishes)
	sc.Step(`^fake Pi session "([^"]+)" is renamed inside Pi to "([^"]+)"$`, fakePiIsRenamedInside)
	sc.Step(`^the fake Pi session file of session "([^"]+)" has the name "([^"]+)"$`, fakePiSessionFileHasName)
	sc.Step(`^the fake Pi session file of session "([^"]+)" has no session_info record$`, fakePiSessionFileHasNoName)
	sc.Step(`^the fake Pi session file of session "([^"]+)" has (\d+) session_info records?$`, fakePiSessionFileHasRecords)
	sc.Step(`^the fake Pi control file of session "([^"]+)" is open$`, fakePiControlFileIsOpen)
	sc.Step(`^the fake Pi control file of session "([^"]+)" does not exist$`, fakePiControlFileDoesNotExist)
	sc.Step(`^the fake Pi pane of session "([^"]+)" shows "([^"]+)"$`, fakeCopilotPaneShows)
	sc.Step(`^the fake Pi pane of session "([^"]+)" does not show "([^"]+)"$`, fakePiPaneDoesNotShow)
	sc.Step(`^the deck extension of fake Pi session "([^"]+)" loses its control file$`, fakePiLosesControlFile)
	sc.Step(`^fake Pi session "([^"]+)" opens a confirm dialog "([^"]+)"$`, fakePiOpensConfirmDialog)
}

// piSessionInfo is the fields of a session_info entry the steps read.
type piSessionInfo struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// piSessionInfos reads every session_info entry of the conversation's file in
// the fake's HOME, in file order; found is false while no file exists.
func piSessionInfos(h *ScenarioHarness, conversation string) (infos []piSessionInfo, found bool, err error) {
	pattern := filepath.Join(h.agentHOMEDir, ".pi", "agent", "sessions", "*", "*_"+conversation+".jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return nil, false, err
	}
	file, err := os.Open(files[0])
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry piSessionInfo
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.Type == "session_info" {
			infos = append(infos, entry)
		}
	}
	return infos, true, scanner.Err()
}

// pollPiSessionFile polls the session file of the conversation until ok holds
// for its session_info entries. The conversation id is resolved once, up front:
// deck may adopt a new name for the row while the poll runs.
func pollPiSessionFile(ctx context.Context, session, what string, ok func(infos []piSessionInfo, found bool) bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var last []piSessionInfo
	var lastErr error
	for {
		infos, found, err := piSessionInfos(h, id)
		if err == nil && ok(infos, found) {
			return nil
		}
		last, lastErr = infos, err
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Pi session file of conversation %q never showed %s (last %+v, error: %w)", id, what, last, lastErr)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakePiSessionFileHasName(ctx context.Context, session, want string) error {
	return pollPiSessionFile(ctx, session, fmt.Sprintf("the last session_info named %q", want), func(infos []piSessionInfo, _ bool) bool {
		return len(infos) > 0 && infos[len(infos)-1].Name == want
	})
}

// fakePiSessionFileHasNoName holds when the file has no session_info entry at
// all; Pi writes one only for a launch --name, /name or setSessionName.
func fakePiSessionFileHasNoName(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	infos, _, err := piSessionInfos(h, id)
	if err != nil {
		return err
	}
	if len(infos) != 0 {
		return fmt.Errorf("fake Pi session file of %q holds session_info entries %+v, want none", session, infos)
	}
	return nil
}

func fakePiSessionFileHasRecords(ctx context.Context, session string, count int) error {
	return pollPiSessionFile(ctx, session, fmt.Sprintf("%d session_info records", count), func(infos []piSessionInfo, _ bool) bool {
		return len(infos) == count
	})
}

func piControlFilePath(h *ScenarioHarness, conversation string) string {
	return filepath.Join(h.Home, "pi", "control", conversation+".jsonl")
}

func fakePiControlFileIsOpen(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	path := piControlFilePath(h, id)
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Pi never opened its control file %s", path)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakePiControlFileDoesNotExist(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	if _, err := os.Stat(piControlFilePath(h, id)); err == nil {
		return fmt.Errorf("fake Pi session %q has a control file, want none", session)
	}
	return nil
}

// sendFakePiCommand writes one JSON pane command to the fake and returns once
// the pane shows one more "fake-pi hook fired: <event>" line than before: the
// hook has then run through the real receiver.
func sendFakePiCommand(ctx context.Context, session string, command map[string]any, event string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	fired := "fake-pi hook fired: " + event + "\n"
	count := func() (int, string) {
		out, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		return strings.Count(string(out)+"\n", fired), string(out)
	}
	before, _ := count()
	if err := sendFakeCodexCommand(ctx, session, command); err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	for {
		after, pane := count()
		if after > before {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Pi session %q never fired %s after %v; pane:\n%s", session, event, command["command"], pane)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakePiStartsSession(ctx context.Context, session string) error {
	return sendFakePiCommand(ctx, session, map[string]any{"command": "start"}, "SessionStart")
}

func fakePiPromptedAndFinishes(ctx context.Context, session string) error {
	hook := func(event string) map[string]any {
		return map[string]any{"command": "hook", "event": event, "payload": map[string]any{}}
	}
	if err := sendFakePiCommand(ctx, session, hook("UserPromptSubmit"), "UserPromptSubmit"); err != nil {
		return err
	}
	return sendFakePiCommand(ctx, session, hook("Stop"), "Stop")
}

// fakePiIsRenamedInside plays Pi's built-in /name: the session file gains the
// entry and the extension reports session_info_changed.
func fakePiIsRenamedInside(ctx context.Context, session, title string) error {
	return sendFakePiCommand(ctx, session, map[string]any{"command": "name", "name": title}, "session_info_changed")
}

// fakePiLosesControlFile removes the session's control file, as when the
// extension could not keep it: deck's push then falls back to keystrokes.
func fakePiLosesControlFile(ctx context.Context, session string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	return os.Remove(piControlFilePath(h, id))
}

// fakePiOpensConfirmDialog plays an extension's ui.confirm and returns once the
// pane shows the selector's key-hint line, the text deck's Pi adapter reads.
func fakePiOpensConfirmDialog(ctx context.Context, session, title string) error {
	if err := sendFakeCodexCommand(ctx, session, map[string]any{"command": "dialog", "name": title}); err != nil {
		return err
	}
	return fakeCopilotPaneShows(ctx, session, "enter select  escape/ctrl+c cancel")
}

// fakePiPaneDoesNotShow reads the pane's scrollback once: callers assert it
// after the event that decided the outcome has already been observed.
func fakePiPaneDoesNotShow(ctx context.Context, session, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return err
	}
	out, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", "deck_"+slug)
	if err != nil {
		return err
	}
	if strings.Contains(string(out), text) {
		return fmt.Errorf("fake Pi pane of session %q shows %q:\n%s", session, text, out)
	}
	return nil
}
