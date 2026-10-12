package agent

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // the driver Codex's state_5.sqlite is read through
)

// codexStateFile is the Codex database ReadName reads: <CODEX_HOME>/state_5.sqlite,
// table `threads`, columns `id` and `name` (NULL until the thread has a name).
// The `_5` is Codex's own schema version, so a release that bumps it simply
// leaves this file missing and the name falls back to the pane title.
const codexStateFile = "state_5.sqlite"

// codexReadTimeout bounds the whole database read, busy wait included.
const codexReadTimeout = 500 * time.Millisecond

// codexBusyTimeout is the busy_timeout of the read-only connection, in ms.
const codexBusyTimeout = 250

// codexNameReadRunes caps the `threads.name` text the read copies out of the
// database (SPEC §8): the query takes at most this many characters, so a
// huge or hostile value is never read whole. It is above MaxHarnessNameRunes
// so that stripping escapes and controls still leaves a full-length name.
const codexNameReadRunes = 1024

// Why a read of state_5.sqlite degraded to the pane title (R258).
const (
	codexDegradeNoDB     = "state_5.sqlite is missing or unreadable"
	codexDegradeNoTable  = "state_5.sqlite has no threads table"
	codexDegradeBadShape = "state_5.sqlite threads table lacks the id or name column"
)

// nameDegradeLog receives a one-line message the first time a name source
// degrades in this process. The default drops it; cmd/deck points it at the
// audit log through SetNameDegradeLog.
var (
	nameDegradeMu  sync.Mutex
	nameDegradeLog = func(string) {}
	nameDegradeOne sync.Map
)

// SetNameDegradeLog installs the sink for degrade messages (nil restores the
// default, which drops them) and forgets which degrades were already logged.
func SetNameDegradeLog(sink func(string)) {
	nameDegradeMu.Lock()
	defer nameDegradeMu.Unlock()
	if sink == nil {
		sink = func(string) {}
	}
	nameDegradeLog = sink
	nameDegradeOne = sync.Map{}
}

// logNameDegrade reports msg once per process (per distinct message).
func logNameDegrade(msg string) {
	nameDegradeMu.Lock()
	sink, seen := nameDegradeLog, &nameDegradeOne
	nameDegradeMu.Unlock()
	if _, dup := seen.LoadOrStore(msg, true); !dup {
		sink("codex name: " + msg + "; using the pane title")
	}
}

// ReadName returns Codex's name for the conversation (R258): `threads.name`
// from state_5.sqlite, opened mode=ro with a short busy timeout and never
// written, falling back to the pane title when the database, its table or its
// columns are not what deck expects (logged once). A NULL name means Codex
// has not named the thread yet, which is no name, not a reason to degrade.
//
// Codex does not say whether a name was chosen by a human. explicit is the
// best-effort heuristic in SPEC §8.2: a name that differs from the one deck
// already adopted from Codex (NameProbeInput.AdoptedName) was changed after
// Codex generated its title, so a human did it.
func (Codex) ReadName(in NameProbeInput) (string, bool, bool) {
	name, found, degraded := codexThreadName(in)
	if degraded {
		name = codexPaneName(in)
		found = name != ""
	}
	if !found {
		return "", false, false
	}
	return name, codexNameChanged(name, in.AdoptedName), true
}

// codexThreadName reads the name from the database; degraded reports that the
// database could not answer at all.
func codexThreadName(in NameProbeInput) (name string, found, degraded bool) {
	path := codexStatePath(in)
	if path == "" || !safeConversationID(in.ConversationID) {
		return "", false, true
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		logNameDegrade(codexDegradeNoDB)
		return "", false, true
	}
	name, found, reason := queryCodexThread(path, in.ConversationID)
	if reason != "" {
		logNameDegrade(reason)
		return "", false, true
	}
	return name, found, false
}

// codexStatePath is the database path, or "" when no Codex home is known.
func codexStatePath(in NameProbeInput) string {
	root := in.Env["CODEX_HOME"]
	if root == "" {
		if in.Home == "" {
			return ""
		}
		root = filepath.Join(in.Home, ".codex")
	}
	return filepath.Join(root, codexStateFile)
}

// codexDSN is the read-only, short-busy-timeout connection string for path.
func codexDSN(path string) string {
	query := url.Values{"mode": {"ro"}}
	query.Add("_pragma", "busy_timeout("+strconv.Itoa(codexBusyTimeout)+")")
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
}

// queryCodexThread opens path read-only and reads the thread's name. A
// non-empty reason says the database is not the expected shape.
func queryCodexThread(path, id string) (name string, found bool, reason string) {
	db, err := sql.Open("sqlite", codexDSN(path))
	if err != nil {
		return "", false, codexDegradeNoDB
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), codexReadTimeout)
	defer cancel()
	if reason := checkCodexShape(ctx, db); reason != "" {
		return "", false, reason
	}
	var raw sql.NullString
	switch err := db.QueryRowContext(ctx, `SELECT substr(name, 1, ?) FROM threads WHERE id = ?`, codexNameReadRunes, id).Scan(&raw); err {
	case nil:
		clean := SanitiseHarnessName(raw.String)
		return clean, raw.Valid && clean != "", ""
	case sql.ErrNoRows:
		return "", false, ""
	default:
		return "", false, codexDegradeBadShape
	}
}

// checkCodexShape is "" when threads has the id and name columns, else the
// degrade reason.
func checkCodexShape(ctx context.Context, db *sql.DB) string {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(threads)`)
	if err != nil {
		return codexDegradeNoDB
	}
	defer func() { _ = rows.Close() }()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var column, ctype string
		var dflt sql.NullString
		if rows.Scan(&cid, &column, &ctype, &notnull, &dflt, &pk) != nil {
			return codexDegradeBadShape
		}
		columns[column] = true
	}
	switch {
	case rows.Err() != nil:
		return codexDegradeNoDB
	case len(columns) == 0:
		return codexDegradeNoTable
	case !columns["id"] || !columns["name"]:
		return codexDegradeBadShape
	}
	return ""
}

// codexSpinnerGlyph matches the activity glyph and space Codex puts in front
// of its terminal title while it works.
var codexSpinnerGlyph = regexp.MustCompile(`^[\x{2800}-\x{28FF}✳✶✻✽✢·⠋]\s+`)

// codexPaneName parses the pane title `[spinner ]<name> | <cwd-basename>`. A
// title without the separator and the working directory's name is not
// Codex's name for the thread (tmux shows the host name until Codex sets a
// title), so it is no name.
func codexPaneName(in NameProbeInput) string {
	if in.PaneTitle == nil || in.CWD == "" {
		return ""
	}
	title := codexSpinnerGlyph.ReplaceAllString(strings.TrimSpace(in.PaneTitle()), "")
	suffix := " | " + filepath.Base(filepath.Clean(in.CWD))
	if !strings.HasSuffix(title, suffix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(title, suffix))
}

// codexSuffixed matches the "-2", "-3" ... deck appends to keep names apart.
var codexSuffixed = regexp.MustCompile(`-\d+$`)

// codexNameChanged is the explicit/generated heuristic: a name differing from
// the adopted one (ignoring deck's own collision suffix) was set after Codex's
// generated title. With nothing adopted yet, the name is the first one.
func codexNameChanged(name, adopted string) bool {
	if adopted == "" {
		return false
	}
	return name != adopted && name != codexSuffixed.ReplaceAllString(adopted, "")
}
