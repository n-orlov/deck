package features

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

// R265: the @real-agents name-contract pins (real_agent_names.feature). Every
// assertion is on the raw upstream shape, with no alias or coercion, so an
// upstream rename fails here by naming the record, key or column that moved.

const realNamesTimeout = 90 * time.Second

// The Codex "resolves on PATH, or skipped" Given is registered by
// registerRealAgentCodexHookSteps.
func registerRealAgentNamesSteps(sc *godog.ScenarioContext) {
	for kind, display := range map[string]string{"claude": "Claude", "pi": "Pi"} {
		sc.Step(`^the installed `+display+` CLI resolves on PATH, or this scenario is skipped with a stated reason$`, realNamesCLIOrSkip(kind))
	}
	sc.Step(`^deck client "([^"]+)" creates ([a-z]+) session unnamed, labelled "([^"]+)", with permission profile "([^"]+)"$`, clientCreatesUnnamedAgentSession)
	sc.Step(`^session "([^"]+)" types the line "([^"]*)" into its real pane$`, typeLineIntoRealPane)
	sc.Step(`^the real Claude transcript of session "([^"]+)" holds an? "([a-z-]+)" record whose "([A-Za-z]+)" is a non-empty string$`, realClaudeRecordNonEmpty)
	sc.Step(`^the real Claude transcript of session "([^"]+)" holds an? "([a-z-]+)" record whose "([A-Za-z]+)" is "([^"]+)"$`, realClaudeRecordIs)
	sc.Step(`^the real Codex state_5.sqlite has a threads table with id and name columns$`, realCodexThreadsColumns)
	sc.Step(`^the real Codex state_5.sqlite thread of session "([^"]+)" has name "([^"]+)"$`, realCodexThreadName)
	sc.Step(`^the real Copilot workspace.yaml of session "([^"]+)" has "([a-z_]+)" "([^"]+)" and "([a-z_]+)" true$`, realCopilotWorkspaceYAML)
	sc.Step(`^the real Pi session file of session "([^"]+)" holds a "([a-z_]+)" entry whose "([a-z]+)" is "([^"]+)"$`, realPiSessionInfo)
}

func realNamesCLIOrSkip(kind string) func(context.Context) error {
	return func(ctx context.Context) error {
		if _, err := exec.LookPath(kind); err != nil {
			godog.Log(ctx, fmt.Sprintf("skipping real %s name contract scenario: no installed %s CLI on PATH: %v", kind, kind, err))
			return godog.ErrSkip
		}
		return nil
	}
}

func typeLineIntoRealPane(ctx context.Context, name, line string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	pane := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", pane, "-l", line); err != nil {
		return fmt.Errorf("type %q into pane %q: %w", line, pane, err)
	}
	// A slash command opens a completion list that swallows an instant Enter.
	time.Sleep(500 * time.Millisecond)
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", pane, "Enter"); err != nil {
		return fmt.Errorf("submit %q in pane %q: %w", line, pane, err)
	}
	return nil
}

// jsonlRecords decodes every complete JSON object line of path; a partial last
// line is skipped, as deck's own tail reader does.
func jsonlRecords(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var records []map[string]any
	reader := bufio.NewReaderSize(file, 1<<20)
	for {
		line, readErr := reader.ReadBytes('\n')
		var record map[string]any
		if json.Unmarshal(line, &record) == nil {
			records = append(records, record)
		}
		if readErr != nil {
			return records, nil
		}
	}
}

// waitForJSONLRecord polls path until a record of the given type has field
// satisfying accept, or the timeout elapses with the shapes seen so far.
func waitForJSONLRecord(path, typeKey, recordType, field string, accept func(string) bool) error {
	return pollUntil(realNamesTimeout, func() error {
		return jsonlRecordOnce(path, typeKey, recordType, field, accept)
	})
}

func realClaudeTranscript(ctx context.Context, name string) (string, error) {
	payload, _, err := waitForRealClaudeHook(ctx, name, "UserPromptSubmit")
	if err != nil {
		return "", err
	}
	path, _ := payload["transcript_path"].(string)
	if path == "" {
		return "", errors.New("real Claude's UserPromptSubmit hook carried no transcript_path")
	}
	return path, nil
}

func realClaudeRecordNonEmpty(ctx context.Context, name, recordType, field string) error {
	path, err := realClaudeTranscript(ctx, name)
	if err != nil {
		return err
	}
	return waitForJSONLRecord(path, "type", recordType, field, func(v string) bool { return strings.TrimSpace(v) != "" })
}

func realClaudeRecordIs(ctx context.Context, name, recordType, field, want string) error {
	path, err := realClaudeTranscript(ctx, name)
	if err != nil {
		return err
	}
	return waitForJSONLRecord(path, "type", recordType, field, func(v string) bool { return v == want })
}

// realCodexStatePath is where deck's reader looks: $CODEX_HOME, else ~/.codex.
func realCodexStatePath() (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".codex")
	}
	return filepath.Join(home, "state_5.sqlite"), nil
}

func openRealCodexState() (*sql.DB, error) {
	path, err := realCodexStatePath()
	if err != nil {
		return nil, err
	}
	return sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(250)")
}

func realCodexThreadsColumns(context.Context) error {
	return pollUntil(realNamesTimeout, func() error {
		db, err := openRealCodexState()
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		rows, err := db.Query(`PRAGMA table_info(threads)`)
		if err != nil {
			return fmt.Errorf("name contract unsupported: threads table unreadable: %w", err)
		}
		defer func() { _ = rows.Close() }()
		columns := map[string]bool{}
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var dflt any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				return err
			}
			columns[name] = true
		}
		if !columns["id"] || !columns["name"] {
			return fmt.Errorf("name contract unsupported: threads columns = %v, want id and name", columns)
		}
		return nil
	})
}

func realCodexThreadName(ctx context.Context, session, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, session)
	if err != nil {
		return err
	}
	return pollUntil(realNamesTimeout, func() error {
		db, err := openRealCodexState()
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		var name sql.NullString
		if err := db.QueryRow(`SELECT name FROM threads WHERE id = ?`, id).Scan(&name); err != nil {
			return fmt.Errorf("name contract unsupported: threads.name of %s: %w", id, err)
		}
		if name.String != want {
			return fmt.Errorf("name contract unsupported: threads.name = %q, want %q", name.String, want)
		}
		return nil
	})
}

func realCopilotWorkspaceYAML(ctx context.Context, session, nameKey, want, namedKey string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, session)
	if err != nil {
		return err
	}
	home := ""
	for _, kv := range h.clientEnv {
		if v, ok := strings.CutPrefix(kv, "COPILOT_HOME="); ok {
			home = v
		}
	}
	if home == "" {
		return errors.New("the real Copilot CLI was not isolated in a temporary COPILOT_HOME")
	}
	path := filepath.Join(home, "session-state", id, "workspace.yaml")
	return pollUntil(realNamesTimeout, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("name contract unsupported: workspace.yaml is not YAML: %w", err)
		}
		if got, _ := doc[nameKey].(string); got != want {
			return fmt.Errorf("name contract unsupported: workspace.yaml %q = %v, want %q (keys: %v)", nameKey, doc[nameKey], want, keysOf(doc))
		}
		if got, _ := doc[namedKey].(bool); !got {
			return fmt.Errorf("name contract unsupported: workspace.yaml %q = %v, want boolean true", namedKey, doc[namedKey])
		}
		return nil
	})
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// realPiSessionRoot is where Pi keeps session files for the current user.
func realPiSessionRoot() (string, error) {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return filepath.Join(dir, "sessions"), nil
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(user, ".pi", "agent", "sessions"), nil
}

func realPiSessionInfo(ctx context.Context, session, entryType, field, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, session)
	if err != nil {
		return err
	}
	root, err := realPiSessionRoot()
	if err != nil {
		return err
	}
	return pollUntil(realNamesTimeout, func() error {
		matches, _ := filepath.Glob(filepath.Join(root, "*", "*_"+id+".jsonl"))
		if len(matches) == 0 {
			return fmt.Errorf("name contract unsupported: no Pi session file *_%s.jsonl under %s", id, root)
		}
		return jsonlRecordOnce(matches[0], "type", entryType, field, func(v string) bool { return v == want })
	})
}

// jsonlRecordOnce is one look at path (the caller is polling).
func jsonlRecordOnce(path, typeKey, recordType, field string, accept func(string) bool) error {
	records, err := jsonlRecords(path)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, record := range records {
		kind, _ := record[typeKey].(string)
		seen[kind] = true
		if value, ok := record[field].(string); kind == recordType && ok && accept(value) {
			return nil
		}
	}
	return fmt.Errorf("name contract unsupported: no %q entry with a string %q in %s (entry types seen: %v)", recordType, field, path, seen)
}

// resolveSessionName maps a scenario's label for an unnamed session to the
// row's CURRENT name (deck renames the row when it adopts the harness's
// ai-title, so the alias is the row id, not the derived name); every other
// name is returned unchanged.
func (h *ScenarioHarness) resolveSessionName(name string) string {
	id, ok := h.sessionNameAliases[name]
	if !ok {
		return name
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return name
	}
	defer func() { _ = db.Close() }()
	var current string
	if err := db.QueryRow(`SELECT name FROM sessions WHERE id = ?`, id).Scan(&current); err != nil {
		return name
	}
	return current
}

// clientCreatesUnnamedAgentSession submits the create modal with the name left
// blank, so the row is name_source=derived and the agent is launched without a
// name flag (R254): only then may the harness generate its own title (Claude's
// ai-title). The derived name is clock-dependent, so the step finds the new
// row in the state database and registers label as an alias for its id.
func clientCreatesUnnamedAgentSession(ctx context.Context, clientName, kind, label, profile string) error {
	h, client, err := positionCreateModalOnProfileField(ctx, clientName, kind, "", profile)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return pollUntil(realNamesTimeout, func() error {
		db, err := openObservedDatabase(h)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		rows, err := db.Query(`SELECT id FROM sessions WHERE name_source = 'derived' AND agent = ? ORDER BY created_at DESC`, kind)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		taken := map[string]bool{}
		for _, id := range h.sessionNameAliases {
			taken[id] = true
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			if !taken[id] {
				if h.sessionNameAliases == nil {
					h.sessionNameAliases = map[string]string{}
				}
				h.sessionNameAliases[label] = id
				return nil
			}
		}
		return fmt.Errorf("no new derived-name %s session row appeared after the blank-name create", kind)
	})
}
