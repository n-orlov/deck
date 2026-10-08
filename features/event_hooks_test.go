package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

// registerEventHookSteps drives SPEC §10's event hook against the released
// deck binary. The only program any step makes deck spawn for the hook is the
// capture script written by installCaptureScript: it records its argv, a
// selected set of environment variables and the stdin payload, and prints one
// line of output. Nothing here calls a notification service.
func registerEventHookSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario's event hook is the capture script$`, func(ctx context.Context) error {
		return captureScriptConfigured(ctx, "")
	})
	sc.Step(`^the scenario's event hook is the capture script with settings:$`, func(ctx context.Context, doc *godog.DocString) error {
		return captureScriptConfigured(ctx, doc.Content)
	})
	sc.Step(`^the capture script is installed but not configured$`, func(ctx context.Context) error {
		h, err := scenarioHarness(ctx)
		if err != nil {
			return err
		}
		_, err = installCaptureScript(h)
		return err
	})
	sc.Step(`^the capture script holds each invocation open for several seconds$`, captureScriptHolds)
	sc.Step(`^the state database session "([^"]+)" has its own event hook (on|off|inherit)$`, sessionOwnEventHookFlag)
	sc.Step(`^the state database session "([^"]+)" has its own event hook events "([^"]*)"$`, sessionOwnEventHookEvents)
	sc.Step(`^the state database session "([^"]+)" inherits the event hook events$`, sessionInheritsEventHookEvents)
	sc.Step(`^the capture script has recorded exactly (\d+) invocations?$`, captureInvocationCount)
	sc.Step(`^capture invocation (\d+) has the event kind "([^"]+)" as its only argument$`, captureInvocationOnlyArgument)
	sc.Step(`^capture invocation (\d+) has environment "([A-Z_]+)" equal to "([^"]*)"$`, captureInvocationEnv)
	sc.Step(`^capture invocation (\d+) has a non-empty environment "([A-Z_]+)"$`, captureInvocationEnvNonEmpty)
	sc.Step(`^capture invocation (\d+) has stdin JSON field "([a-z_.]+)" equal to "([^"]*)"$`, captureInvocationStdinField)
	sc.Step(`^capture invocation (\d+) has stdin JSON field "([a-z_.]+)" that is non-empty$`, captureInvocationStdinFieldNonEmpty)
	sc.Step(`^the capture script process of invocation (\d+) is gone within (\d+) seconds$`, captureProcessGone)
	sc.Step(`^session "([^"]+)"'s latest "([^"]+)" event records hook kind "([^"]+)", exit status (\d+) and output containing "([^"]*)"$`, eventRecordsHookResult)
	sc.Step(`^session "([^"]+)"'s latest "([^"]+)" event records a timed out hook$`, eventRecordsTimedOutHook)
	sc.Step(`^session "([^"]+)"'s latest "([^"]+)" event records no hook result$`, eventRecordsNoHookResult)
	sc.Step(`^the released deck _hook receives "([^"]+)" for session "([^"]+)" using injected identity and returns within (\d+) seconds:$`, releasedHookReturnsWithin)
	sc.Step(`^the structured log holds (\d+) event-hook invocations? for session "([^"]+)", the last of kind "([^"]+)" (exit status -?\d+|timed out|detached)$`, structuredLogRecordsEventHook)
}

// captureScriptSource is the whole capture script. It uses only shell
// builtins (it never spawns a child), so the test spawns nothing but this one
// script: one record per invocation, appended with a single printf, then one
// line on stdout for the recorded-output tail.
const captureScriptSource = `#!/usr/bin/env bash
dir=%q
nl=$'\n'
rec="--record${nl}pid=$$${nl}argc=$#${nl}"
i=1
for a in "$@"; do rec+="arg$i=$a${nl}"; i=$((i+1)); done
for v in DECK_SESSION_ID DECK_SESSION_NAME DECK_SESSION_AGENT DECK_EVENT_KIND DECK_EVENT_REASON DECK_EVENT_MESSAGE DECK_EVENT_AT; do
  rec+="env.$v=${!v}${nl}"
done
IFS= read -r payload
rec+="stdin=${payload}${nl}--end"
printf '%%s\n' "$rec" >>"$dir/records.log"
printf 'captured %%s\n' "$1"
if [ -e "$dir/hold-requested" ]; then
  read -r -t 8 <>"$dir/hold"
fi
`

func captureDir(h *ScenarioHarness) string { return filepath.Join(h.Home, "capture") }

// installCaptureScript writes the capture script and the fifo its hold mode
// reads from, and returns the script's absolute path.
func installCaptureScript(h *ScenarioHarness) (string, error) {
	dir := captureDir(h)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	script := filepath.Join(dir, "capture.sh")
	if err := os.WriteFile(script, []byte(fmt.Sprintf(captureScriptSource, dir)), 0o700); err != nil {
		return "", fmt.Errorf("write capture script: %w", err)
	}
	hold := filepath.Join(dir, "hold")
	if _, err := os.Stat(hold); errors.Is(err, os.ErrNotExist) {
		if err := syscall.Mkfifo(hold, 0o600); err != nil {
			return "", fmt.Errorf("make capture hold fifo: %w", err)
		}
	}
	return script, nil
}

// captureScriptConfigured (re)writes config.toml naming the capture script as
// event_hook, followed by the scenario's own extra top-level settings.
func captureScriptConfigured(ctx context.Context, extra string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	script, err := installCaptureScript(h)
	if err != nil {
		return err
	}
	content := fmt.Sprintf("event_hook = %q\n%s\n", script, extra)
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml: %w", err)
	}
	return nil
}

func captureScriptHolds(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(captureDir(h), "hold-requested"), nil, 0o600)
}

var eventHookColumnUpdates = map[string]string{
	"event_hook_enabled": `UPDATE sessions SET event_hook_enabled = ? WHERE name = ?`,
	"event_hook_events":  `UPDATE sessions SET event_hook_events = ? WHERE name = ?`,
}

func updateSessionColumn(ctx context.Context, name, column string, value any) error {
	statement, known := eventHookColumnUpdates[column]
	if !known {
		return fmt.Errorf("no update statement for session column %q", column)
	}
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, statement, value, name)
	if err != nil {
		return fmt.Errorf("set %s of session %q: %w", column, name, err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("set %s of session %q: %d rows changed, want 1", column, name, n)
	}
	return nil
}

func sessionOwnEventHookFlag(ctx context.Context, name, flag string) error {
	var value any
	switch flag {
	case "on":
		value = 1
	case "off":
		value = 0
	}
	return updateSessionColumn(ctx, name, "event_hook_enabled", value)
}

func sessionOwnEventHookEvents(ctx context.Context, name, list string) error {
	kinds := []string{}
	for _, kind := range strings.Split(list, ",") {
		if kind = strings.TrimSpace(kind); kind != "" {
			kinds = append(kinds, kind)
		}
	}
	encoded, err := json.Marshal(kinds)
	if err != nil {
		return err
	}
	return updateSessionColumn(ctx, name, "event_hook_events", string(encoded))
}

func sessionInheritsEventHookEvents(ctx context.Context, name string) error {
	return updateSessionColumn(ctx, name, "event_hook_events", nil)
}

// captureRecord is one parsed invocation of the capture script.
type captureRecord struct {
	fields map[string]string
}

func readCaptureRecords(h *ScenarioHarness) ([]captureRecord, error) {
	data, err := os.ReadFile(filepath.Join(captureDir(h), "records.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []captureRecord
	var current *captureRecord
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case line == "--record":
			current = &captureRecord{fields: map[string]string{}}
		case line == "--end":
			if current != nil {
				records = append(records, *current)
			}
			current = nil
		case current != nil:
			if key, value, ok := strings.Cut(line, "="); ok {
				current.fields[key] = value
			}
		}
	}
	return records, nil
}

func captureRecordN(h *ScenarioHarness, n int) (captureRecord, error) {
	records, err := readCaptureRecords(h)
	if err != nil {
		return captureRecord{}, err
	}
	if n < 1 || n > len(records) {
		return captureRecord{}, fmt.Errorf("capture invocation %d does not exist: the script recorded %d", n, len(records))
	}
	return records[n-1], nil
}

// captureInvocationCount waits for the count to reach want (the TUI's
// dispatch is asynchronous) and then requires it to be exactly want. A want of
// zero is read at once: every scenario that expects none fires its event
// through the synchronous released `_hook`, which has dispatched (or decided
// not to) before it exits.
func captureInvocationCount(ctx context.Context, want int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		records, err := readCaptureRecords(h)
		if err != nil {
			return err
		}
		if len(records) >= want {
			if len(records) != want {
				return fmt.Errorf("capture script recorded %d invocations, want exactly %d", len(records), want)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("capture script recorded %d invocations, want exactly %d", len(records), want)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func captureInvocationOnlyArgument(ctx context.Context, n int, kind string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	record, err := captureRecordN(h, n)
	if err != nil {
		return err
	}
	if record.fields["argc"] != "1" || record.fields["arg1"] != kind {
		return fmt.Errorf("invocation %d argv = argc %s arg1 %q, want the single argument %q", n, record.fields["argc"], record.fields["arg1"], kind)
	}
	return nil
}

func captureInvocationEnv(ctx context.Context, n int, name, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	record, err := captureRecordN(h, n)
	if err != nil {
		return err
	}
	if got := record.fields["env."+name]; got != want {
		return fmt.Errorf("invocation %d %s = %q, want %q", n, name, got, want)
	}
	return nil
}

func captureInvocationEnvNonEmpty(ctx context.Context, n int, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	record, err := captureRecordN(h, n)
	if err != nil {
		return err
	}
	if record.fields["env."+name] == "" {
		return fmt.Errorf("invocation %d %s is empty", n, name)
	}
	return nil
}

func stdinJSONField(ctx context.Context, n int, path string) (string, error) {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return "", err
	}
	record, err := captureRecordN(h, n)
	if err != nil {
		return "", err
	}
	var decoded any
	if err := json.Unmarshal([]byte(record.fields["stdin"]), &decoded); err != nil {
		return "", fmt.Errorf("invocation %d stdin is not one JSON object: %w (%q)", n, err, record.fields["stdin"])
	}
	for _, key := range strings.Split(path, ".") {
		object, ok := decoded.(map[string]any)
		if !ok {
			return "", fmt.Errorf("invocation %d stdin has no field %q", n, path)
		}
		if decoded, ok = object[key]; !ok {
			return "", fmt.Errorf("invocation %d stdin has no field %q", n, path)
		}
	}
	return fmt.Sprint(decoded), nil
}

func captureInvocationStdinField(ctx context.Context, n int, path, want string) error {
	got, err := stdinJSONField(ctx, n, path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("invocation %d stdin %s = %q, want %q", n, path, got, want)
	}
	return nil
}

func captureInvocationStdinFieldNonEmpty(ctx context.Context, n int, path string) error {
	got, err := stdinJSONField(ctx, n, path)
	if err != nil {
		return err
	}
	if got == "" {
		return fmt.Errorf("invocation %d stdin %s is empty", n, path)
	}
	return nil
}

// captureProcessGone reads /proc/<pid>/stat (nothing is signalled) until the
// recorded process no longer exists or is only a zombie awaiting its reaper
// (a detached session-end script is reparented to whatever is pid 1, which in
// a container may never reap it).
func captureProcessGone(ctx context.Context, n, seconds int) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	record, err := captureRecordN(h, n)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(record.fields["pid"])
	if err != nil || pid <= 1 {
		return fmt.Errorf("invocation %d recorded pid %q", n, record.fields["pid"])
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		if processGone(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("capture script process %d of invocation %d is still alive %ds after its timeout", pid, n, seconds)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type eventHookRow struct {
	kind     *string
	exit     *int
	timedOut *int
	output   *string
}

// latestEventHookRow polls for the newest event of the stored kind and hands
// it to ok until ok accepts it or five seconds pass.
func latestEventHookRow(ctx context.Context, session, stored string, ok func(eventHookRow) error) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var row eventHookRow
		scanErr := db.QueryRowContext(ctx, `SELECT hook_kind, hook_exit, hook_timed_out, hook_output
			FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?
			ORDER BY seq DESC LIMIT 1`, session, stored).Scan(&row.kind, &row.exit, &row.timedOut, &row.output)
		err := scanErr
		if err == nil {
			err = ok(row)
		}
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q latest %q event: %w", session, stored, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func eventRecordsHookResult(ctx context.Context, session, stored, kind string, exit int, output string) error {
	return latestEventHookRow(ctx, session, stored, func(row eventHookRow) error {
		if row.kind == nil || *row.kind != kind {
			return fmt.Errorf("hook kind = %v, want %q", row.kind, kind)
		}
		if row.exit == nil || *row.exit != exit {
			return fmt.Errorf("hook exit = %v, want %d", row.exit, exit)
		}
		if row.output == nil || !strings.Contains(*row.output, output) {
			return fmt.Errorf("hook output = %v, want it to contain %q", row.output, output)
		}
		return nil
	})
}

func eventRecordsTimedOutHook(ctx context.Context, session, stored string) error {
	return latestEventHookRow(ctx, session, stored, func(row eventHookRow) error {
		if row.timedOut == nil || *row.timedOut != 1 {
			return fmt.Errorf("hook_timed_out = %v, want 1", row.timedOut)
		}
		return nil
	})
}

// eventRecordsNoHookResult is read once the event exists (the row is written
// before any spawn); a detached session-end dispatch never fills it in.
func eventRecordsNoHookResult(ctx context.Context, session, stored string) error {
	return latestEventHookRow(ctx, session, stored, func(row eventHookRow) error {
		if row.kind != nil || row.exit != nil || row.timedOut != nil || row.output != nil {
			return fmt.Errorf("the event carries a hook result (kind %v exit %v timed out %v)", row.kind, row.exit, row.timedOut)
		}
		return nil
	})
}

func releasedHookReturnsWithin(ctx context.Context, event, session string, seconds int, table *godog.Table) error {
	started := time.Now()
	if err := releasedHookFiresForSession(ctx, event, session, "injected", table); err != nil {
		return err
	}
	if elapsed := time.Since(started); elapsed >= time.Duration(seconds)*time.Second {
		return fmt.Errorf("released deck _hook for %q took %s, want under %ds", event, elapsed.Round(time.Millisecond), seconds)
	}
	return nil
}

func processGone(pid int) bool {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	// The state letter follows the parenthesised command name.
	closing := strings.LastIndex(string(stat), ")")
	fields := strings.Fields(string(stat)[closing+1:])
	return len(fields) > 0 && fields[0] == "Z"
}
