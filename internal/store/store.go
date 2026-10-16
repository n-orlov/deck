package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/config"
	sqlite "modernc.org/sqlite"
)

// busyTimeoutMillis is the SQLite contention budget of every store connection.
const busyTimeoutMillis = 5000

// SchemaVersion is the newest schema understood by this binary.
const SchemaVersion = 21

// SupportedSchemaVersion is the newest schema this binary opens: SchemaVersion
// in every shipped build. A test build (-tags deckoldschema, see
// schema_old_hook.go) lowers it to stand in for an older release.
func SupportedSchemaVersion() int { return supportedSchema }

// NewerSchemaError reports a state database written by a newer deck than this
// binary. Its message keeps the TUI wording ("upgrade deck"); `deck _hook`
// recognises the type with errors.As and prints its own restart-the-session
// message instead (SPEC section 3.1, R204).
type NewerSchemaError struct {
	DB        int // schema version recorded in the database
	Supported int // newest schema this binary understands
	// WriterBinary is the absolute path of the deck binary the database
	// records as its last writer, "" when none is recorded. `deck _hook`
	// re-execs it (SPEC section 3.1, R204).
	WriterBinary string
}

func (e *NewerSchemaError) Error() string {
	return fmt.Sprintf("state database schema version %d is newer than supported version %d; upgrade deck", e.DB, e.Supported)
}

// DefaultLayoutMode and DefaultSidebarWidth are the documented degrade-to
// values a missing ui_state row implies (SPEC §11.2): ui_state is
// explicitly not load-bearing, so losing it (or never having written it)
// must fall back to these rather than to an error.
const (
	DefaultLayoutMode   = "auto"
	DefaultSidebarWidth = 35
)

// Store is deck's sole SQLite writer API. Call Close when the client exits.
type Store struct {
	db   *sql.DB
	path string
	// eventsListCalls is R61's (steer 3e-001 §6.3) own instrumentation: an
	// atomic counter of every real ListEvents call this *Store instance has
	// served, read back through ListEventsCallCount. It exists purely so
	// internal/tui's render-path test can prove the event log genuinely
	// reads the store once per open rather than once per frame, without
	// introducing a separate mock/interface layer over the concrete *Store
	// type every other tui code path already depends on directly.
	eventsListCalls int64
}

func init() { sqlite.RegisterConnectionHook(persistWAL) }

// openUmaskMu serializes OpenPath's umask-tightening window (see OpenPath)
// against any other concurrent OpenPath call in the same process; the umask
// is process-wide, not per-goroutine.
var openUmaskMu sync.Mutex

// persistWAL keeps state.db-wal on disk when the last connection closes
// (SQLITE_FCNTL_PERSIST_WAL). Without it that close deletes the WAL, and the
// next writer -- in practice a short-lived `deck _hook` while no TUI is open
// -- must start a fresh one, which SQLite fsyncs before the first frame even
// at synchronous=NORMAL. That one fsync was nearly the whole of the hook's
// measured store write (SPEC §3.1's 20 ms budget): about 5 ms on an idle
// btrfs disk and past 20 ms on a busy CI host, against about 0.1 ms for the
// transaction itself. The frames left behind are already checkpointed, so
// state.db alone stays complete. It is a driver-wide connection hook rather
// than a step in OpenPath so that every connection gets it, including one the
// pool reopens and any other connection in the same process (test fixtures
// included), since any connection that closes without it deletes the WAL.
func persistWAL(conn sqlite.ExecQuerierContext, _ string) error {
	fc, ok := conn.(sqlite.FileControl)
	if !ok {
		return fmt.Errorf("sqlite connection %T has no file control for a persistent WAL", conn)
	}
	if _, err := fc.FileControlPersistWAL("main", 1); err != nil {
		return fmt.Errorf("keep the state database WAL across closes: %w", err)
	}
	return nil
}

// Open creates or migrates the state database addressed by paths. The database
// directory and file are deliberately private because session environment
// values are durable state.
func Open(paths config.Paths) (*Store, error) { return OpenPath(paths.Home, paths.StateDB) }

// OpenPath is primarily useful to callers that have already resolved runtime
// paths and to tests constructing old-schema fixtures.
func OpenPath(home, path string) (*Store, error) {
	if home == "" {
		home = filepath.Dir(path)
	}
	if path == "" {
		return nil, errors.New("store database path is required")
	}
	// The directory and file below are created, chmodded and opened from these
	// two caller-resolved paths; normalise them once so every later operation
	// works on the same cleaned spelling.
	home, path = filepath.Clean(home), filepath.Clean(path)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}
	// Do not change permissions until after the schema compatibility check:
	// opening a newer database must leave its fixture completely untouched.
	//
	// _txlock=immediate forces every BeginTx to take SQLite's write lock up
	// front (BEGIN IMMEDIATE) rather than the driver default deferred
	// transaction that only upgrades to a write lock on its first write.
	// Without this, two Store processes racing a read-then-write transaction
	// against the same row (e.g. AcquireLaunchLease, task 008/027) can hit
	// SQLITE_BUSY_SNAPSHOT on the upgrade, a stale-snapshot conflict that
	// busy_timeout does not retry, surfacing as an immediate, un-retried
	// locked-database error instead of the intended CAS-loses-the-race
	// outcome. Taking the write lock immediately makes losing that race wait
	// out busy_timeout and retry like every other contended write, matching
	// SPEC section 9.3's guarantee that no case wedges the row under real
	// concurrent callers.
	dsn := immediateTxDSN(path)
	// Tighten the process umask before the first connection opens (and so,
	// for a database that does not exist yet, before SQLite creates it): a
	// brand-new state.db -- and the -wal/-shm siblings SQLite gives the main
	// file's mode at their own creation -- must land on disk already 0600,
	// never briefly at the default create mode while a chmod-after catches
	// up (SPEC section 6.4, R197/#61). The umask is process-wide state, so
	// openUmaskMu serializes this window against any other concurrent
	// OpenPath call in the same process rather than letting them race each
	// other's umask; it is restored when OpenPath returns, by which point
	// every file this call creates already exists.
	defer tightenUmask()()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	// SQLite PRAGMAs such as foreign_keys and busy_timeout are connection-local.
	// A single pooled connection preserves these required settings for every
	// Store operation; concurrent clients use independent Store connections.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path}
	if err := store.configure(home); err != nil {
		return nil, closeAfter(db, err)
	}
	return store, nil
}

// configure checks the on-disk schema version, secures the directory and
// database files, applies the connection PRAGMAs and migrates. OpenPath closes
// the database handle when it returns an error.
func (s *Store) configure(home string) error {
	version, err := s.version()
	if err != nil {
		return err
	}
	// Check this before setting journal mode or running a migration. A future
	// database is read only from this binary's point of view.
	if version > supportedSchema {
		writer, _ := readWriterBinary(s.db)
		return &NewerSchemaError{DB: version, Supported: supportedSchema, WriterBinary: writer}
	}
	if err := os.Chmod(home, 0o700); err != nil { //nolint:gosec // G302: home is a directory, 0o700 is owner-only (gosec reads it as a file mode)
		return fmt.Errorf("secure store directory: %w", err)
	}
	// Tighten state.db before journal_mode=WAL: SQLite creates -wal and -shm
	// with the main file's mode, and the WAL now persists across closes, so a
	// loose main file would leave session env values in a 0644 WAL on disk
	// (SPEC section 6.4). Siblings left loose by an earlier binary are fixed
	// here too; SQLite never changes an existing file's mode.
	if err := secureStateFiles(s.path); err != nil {
		return err
	}
	// secure_delete=ON (R197/#61) zeroes freed database pages in state.db,
	// so a deleted or overwritten session env value is not left recoverable
	// from unallocated space there. Stale WAL frames are NOT rewritten: they
	// persist in state.db-wal until SQLite overwrites them in a later
	// checkpoint cycle, and the 0600 mode on state.db and its -wal/-shm
	// siblings (SPEC section 6.4) protects that window. journal_size_limit is deliberately left unset: forcing it to 0
	// would reset (truncate and recreate) the WAL on every checkpoint, and a
	// fresh WAL's header is fsynced before the first frame -- the exact cost
	// persistWAL above exists to avoid (SPEC section 3.1's 20ms hook write
	// budget).
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=` + strconv.Itoa(busyTimeoutMillis) + `; PRAGMA foreign_keys=ON; PRAGMA secure_delete=ON`); err != nil {
		return fmt.Errorf("configure state database: %w", err)
	}
	if err := s.migrate(version); err != nil {
		return err
	}
	s.recordWriterBinary()
	return nil
}

// immediateTxDSN appends _txlock=immediate to the database path, whether or
// not the path already carries a query string (see OpenPath for why).
func immediateTxDSN(path string) string {
	// busy_timeout rides the DSN as a connection pragma so it is in force
	// before the very first statement: the schema-version read in configure
	// runs ahead of the PRAGMA batch there, and without this a writer holding
	// the lock at that instant failed the read immediately (a hook opening the
	// store during detached Codex startup lost SessionStart). A read-only
	// pragma writes nothing, so the newer-schema refusal stays no-write.
	extra := "_txlock=immediate&_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMillis) + ")"
	if strings.Contains(path, "?") {
		return path + "&" + extra
	}
	return path + "?" + extra
}

// tightenUmask serializes against other OpenPath callers, sets the process
// umask to 0o177 and returns the function that restores the previous umask
// and releases the serialization.
func tightenUmask() func() {
	openUmaskMu.Lock()
	oldUmask := syscall.Umask(0o177)
	return func() {
		syscall.Umask(oldUmask)
		openUmaskMu.Unlock()
	}
}

// secureStateFiles chmods the state database and its -wal/-shm siblings to
// 0o600; a sibling that does not exist yet is skipped.
func secureStateFiles(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("secure state database: %w", err)
		}
	}
	return nil
}

// DB exposes the connection for narrowly scoped queries. Product mutations
// belong on Store methods so they remain targeted and transactional.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database location for external diagnostics.
func (s *Store) Path() string { return s.path }

// Close closes the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// CreateSessionInput is the durable identity supplied when a session is first
// created. CWD deliberately has no uniqueness constraint: several sessions
// may work in the same directory.
type CreateSessionInput struct {
	ID           string
	Name         string
	CWD          string
	Agent        string
	CapturedPath string
	Status       string
	StatusSource string
	StatusAt     int64
	CreatedAt    int64
	// NameSource records who chose Name (R248): "user" (typed or renamed),
	// "derived" (the blank-name default) or "harness" (adopted). Empty is "user".
	NameSource string

	// LaunchArgs are extra argv tokens appended after the adapter's own
	// flags. Persisted as a JSON array; a nil slice round-trips as an empty
	// array, never null, so readers never need to guard against null.
	LaunchArgs []string
	// Env is the session-layer environment, the last (highest-priority) layer
	// in the SPEC §6.3 PATH resolution order. Persisted as a JSON object.
	Env map[string]string
	// PreLaunch is an optional shell command run in the pane before the
	// agent argv, e.g. to source secrets (SPEC §6.4).
	PreLaunch string
	// EventHookEnabled and EventHookEvents are the session's own event-hook
	// controls (SPEC §10.2); nil stores NULL, which inherits the global value.
	EventHookEnabled *bool
	EventHookEvents  []string
	// PostDestroy is an optional shell command run, as its own deck
	// subprocess (never a pane), after this session's row has been durably
	// archived or deleted (SPEC §9.2, R107). It never runs on Kill or any
	// reap path -- that is enforced by the callers that run it (task 013),
	// not by this field. Empty is the common, valid case of nothing
	// configured.
	PostDestroy string
	// LoginShell runs PreLaunch (and the agent) via "$SHELL -lc" rather than
	// relying on CapturedPath, when true.
	LoginShell bool
	// PermissionProfile selects the adapter's permission mapping (e.g.
	// safe|plan|edits|yolo); shell sessions use the empty string.
	PermissionProfile string
	// PermissionProfileReason, when non-empty, is the human-readable
	// explanation recorded when the requested profile could not be honoured
	// as-is and was degraded to PermissionProfile (SPEC §5). Empty when the
	// stored profile is exactly what was requested.
	PermissionProfileReason string
	// ConversationID is the deck-assigned (or caller-pinned) conversation
	// identity handed to adapters that AssignsConversationID.
	ConversationID string
	// ResumePin holds a specific conversation id to prefer on resume when
	// ResumeState is "pinned".
	ResumePin string
	// ResumeState is one of pinned|auto|fresh-once.
	ResumeState string
	// GroupID is SPEC §4/§11's manual group (R130) this session is created
	// into, verbatim onto the sessions.group_id column -- nil for the
	// structural default group, exactly as Session.GroupID's own doc
	// comment describes for every later read. The create modal's Group
	// field (internal/tui, R130) is the only populated caller today; every
	// existing caller predating this task leaves it nil, the exact
	// pre-task behaviour (group_id was always written NULL).
	GroupID *int64
}

// Session is the identity information needed by callers immediately after a
// successful create.
type Session struct {
	ID           string
	Name         string
	Slug         string
	CWD          string
	Agent        string
	CapturedPath string
	Status       string
	StatusReason string
	StatusSource string
	StatusAt     int64
	CreatedAt    int64
	// NameSource is SPEC §4's sessions.name_source: "user", "derived" or "harness".
	NameSource string
	// NameAdopted is SPEC §4's sessions.name_adopted: true when Name came from
	// the harness (adopted, including a name a human chose inside the harness
	// and so frozen as name_source 'user'). Such a name is data only and never
	// reaches a launch or resume argv; a rename typed in deck clears it.
	NameAdopted bool
	// PendingHarnessName is SPEC §4's sessions.pending_harness_name: the name
	// deck pushed into the running harness and has not yet seen confirmed
	// ("" when nothing is pending); PendingHarnessNameAt is the millisecond
	// the push was sent.
	PendingHarnessName   string
	PendingHarnessNameAt int64
	// HarnessNameAdopt is the sessions.harness_name_adopt tri-state (SPEC
	// §11.4): nil inherits adopt_harness_names, otherwise the session's own
	// on/off for adopting the harness's name and pushing renames into it.
	HarnessNameAdopt *bool
	// GroupID is SPEC §4's sessions.group_id column verbatim: the manual
	// group (§11) this row belongs to, nil exactly when the column reads
	// NULL -- either because the row has never been assigned a group (the
	// implicit "default") or because the group it was assigned has since
	// been deleted by another client and the id no longer resolves (SPEC
	// §11: "renders under default rather than vanishing", the reason
	// group_id carries no foreign key at all -- see schemaV7's own
	// comment). There is deliberately no cwd-derived fallback here the
	// way the removed Workspace field had one: SPEC's manual-groups model
	// never derives a group from the filesystem.
	GroupID *int64
	// GroupName is the resolved §11 group label for GroupID, read via a
	// LEFT JOIN against the groups table so a dangling GroupID (the group
	// it named was deleted after this row's group_id was written) reads
	// back empty exactly like the true-default nil case, both of which
	// display as "default" per SPEC §11. Empty whenever GroupID is nil or
	// does not resolve; the group's real name otherwise.
	GroupName string

	KilledByUser   bool
	PaneExitStatus *int
	CrashTail      string
	NotifyEpoch    int64
	LastMessage    string
	// SeenStatusAt is the status_at stamp the user has seen (R318): the row's
	// status is seen exactly when it equals StatusAt (StatusSeen).
	SeenStatusAt int64

	LaunchArgs []string
	Env        map[string]string
	PreLaunch  string
	// EventHookEnabled is the sessions.event_hook_enabled tri-state (SPEC
	// §10.2): nil inherits event_hook_default, otherwise the session's own
	// on/off.
	EventHookEnabled *bool
	// EventHookEvents is sessions.event_hook_events: nil inherits the global
	// list, a non-nil list (possibly empty: offer nothing) replaces it.
	EventHookEvents []string
	// PostDestroy is the sessions.post_destroy column verbatim (SPEC §9.2,
	// R107): the per-session teardown hook run after this row's own Archive
	// or Delete has durably succeeded, before the global post_destroy hook
	// (task 013 runs both; this field is just the stored value). Empty
	// exactly when the row has never had one recorded.
	PostDestroy             string
	LoginShell              bool
	PermissionProfile       string
	PermissionProfileReason string
	ConversationID          string
	ResumePin               string
	ResumeState             string
	// LaunchGeneration is the generation half of the row's own
	// launch_lease_owner ("pid@boot_id#generation"): the per-launch
	// discriminator minted by the AcquireLaunchLease of the launch deck
	// currently considers the live one (issue #11, R74). Empty for a row no
	// lease has ever been taken on -- notably a brand-new row whose first
	// launch came straight from CreateAgent -- which means "this row names no
	// particular launch", not "a launch whose token is empty". The hook
	// receiver compares it with the token the hook's pane carries, so a write
	// from a pane deck has already replaced can be recognized as superseded
	// (internal/hookrecv). It is read here rather than through a dedicated
	// query because every hook already resolves through a Session.
	LaunchGeneration string
	// LastProbeAt is the wall-clock millisecond timestamp of the most recent
	// stale-eligible pane sample that matched no probe.go rule at all (a
	// total miss), recorded by RecordProbeMiss. It deliberately never
	// changes Status/StatusSource/StatusAt (SPEC §7 is untouched by a
	// miss) and exists only so the `i` detail dialog can tell "sampled,
	// no rule matched" apart from "never sampled" (task 009). It is zero
	// until the first miss, and is superseded (LastProbeAt <= StatusAt)
	// the moment any later verdict — probe or otherwise — updates the row,
	// so a stale miss can never be shown once fresher evidence exists.
	LastProbeAt int64
	// ProbeMissStreak counts the consecutive probe total misses since the
	// last verdict (R315): RecordProbeMiss adds one, any probe or hook verdict
	// (and any status write) resets it to 0. A column, never an event.
	ProbeMissStreak int
	// LastEventAt is the `at` of the newest event recorded for the row, 0
	// when none has been (R315). It is derived at read time from the events
	// table (indexed by events_session_at), not stored on the row, so
	// recording an event never rewrites the session row, yet every event — a
	// repeat verdict that leaves status_at alone included — restarts the
	// unverified quiet window.
	LastEventAt int64
	// LastOutputAt is the wall-clock millisecond time of the newest
	// transcript-region output confirmed at reconcile (R304), 0 when none has
	// been. It is the output clock: footer/statusline churn never advances it,
	// and it is written by RecordOutput as a column, never an event.
	LastOutputAt int64
	// LastSeenAt is the last_output_at value the user has viewed (R324), 0
	// until the first view. MarkViewed copies last_output_at into it.
	LastSeenAt int64
	// HookAt is the wall-clock millisecond time of the newest hook verdict
	// the store applied (R303, SPEC §7), 0 until one has been. It is hook
	// evidence's own clock: a hook repeating the current status leaves
	// status_at (the transition stamp) alone but still advances this, so the
	// freshness of a hook verdict never depends on when the status last
	// changed.
	HookAt int64
	// StatusSeq counts the row's status transitions (schemaV21); with StatusAt
	// it names the status episode (Episode).
	StatusSeq int64
	// EnvDirty is SPEC §6.1/§6.3's env_dirty column (task 021): true once a
	// session-env edit has been written and mirrored into tmux's own
	// environment table for future panes, but has NOT yet reached the
	// pane's already-running process -- that only happens on an explicit
	// restart (task 022's `R`), which is also what clears this back to
	// false. It is the row's queryable, persisted source for the `env↻`
	// sidebar badge; nothing here ever touches the live pane itself.
	EnvDirty bool
	// LaunchDirty is the sessions.launch_dirty column (task 020): true once
	// SetLaunchInputs has persisted an edit to one of the four editable
	// launch inputs (PreLaunch, PostDestroy, LaunchArgs, LoginShell) but the
	// change has not yet reached a live pane -- only a later relaunch does
	// that, and clears it back to false via ClearLaunchDirty. Like EnvDirty,
	// this never touches or implies anything about the live pane itself; it
	// is only the row's queryable, persisted "pending launch change" flag.
	LaunchDirty bool
	// DeletedAt is the SPEC delete-tombstone timestamp (task 104): zero for a
	// live row, and the wall-clock millisecond `dd`+confirm ran at once a row
	// has been soft-deleted (SoftDeleteSession). A tombstoned row is excluded
	// from ListSessions' default view but still exists for RestoreSession (a
	// grace-window undo, task 106) or ReapSession (a permanent delete once the
	// grace window has elapsed) to act on by id; ListDeletedSessions is the
	// only accessor that surfaces it.
	DeletedAt int64
	// ArchivedAt is SPEC requirement 27's `A` retention flag (task 111): zero
	// for a live row, and the wall-clock millisecond ArchiveSession ran at
	// once a row has been archived. It is a FLAG, never a status -- an
	// archived row keeps whatever Status it had (SPEC.md:284-287), so
	// nothing here ever writes or reads Status. Like DeletedAt, an archived
	// row is excluded from ListSessions' default view; unlike DeletedAt there
	// is (yet) no restore -- requirement 33's `/` filter (task 123) is how an
	// archived row is reached again, not an undo.
	ArchivedAt int64
	// PinnedAt is the sessions.pinned_at column verbatim (task 002, the
	// sidebar pin `p`, phase 4f, GH #51): zero for a never-pinned row, and
	// the wall-clock millisecond the pin was set at once a row has been
	// pinned. Like ArchivedAt/DeletedAt, it is a FLAG-with-timestamp rather
	// than a Status transition -- nothing here writes or reads Status.
	PinnedAt int64
	// HookExecutable is the sessions.hook_executable column (schemaV9, R204c):
	// the absolute path of the deck binary the session's agent was last
	// launched with, which its hook command stays bound to until the agent
	// is restarted or resumed. Empty when no launch has recorded one.
	HookExecutable string
}

// CapturedPathAdvisory reports whether this row's CapturedPath is advisory
// only, per SPEC §6.3: "enabling login_shell marks captured_path advisory
// and the health view says so". login_shell (persisted, queryable — the
// sessions.login_shell column, never a comment) and reliance on
// captured_path are mutually exclusive by design (§6.3), so the marking
// this reports IS the LoginShell column: a login_shell=1 row still stores
// its create-time CapturedPath (it is never cleared or blanked), but the
// $SHELL -lc launch lets rc files rewrite PATH, discarding it, so the
// stored value is a record of what PATH used to be, not what governs the
// pane.
func (s Session) CapturedPathAdvisory() bool { return s.LoginShell }

// StatusUpdateInput describes one durable state transition. EventKind is kept
// separate because a caller may record a more specific source event than the
// resulting session status; when omitted it is the status itself.
type StatusUpdateInput struct {
	SessionID string
	Status    string
	Reason    string
	Source    string
	At        int64
	EventKind string
	Payload   string

	// StaleAfter is required for probe verdicts and is expressed in the same
	// milliseconds as At. A probe may replace a hook verdict only after this
	// interval; the check and update happen under one write transaction.
	StaleAfter int64
	// KilledByUser marks an explicit terminal user action. ClearKilledByUser
	// makes the row automation-writable again and has exactly two callers:
	// resume (SPEC §9.1 -- once the user resumes, the kill verdict is spent),
	// and the reconciler's §7 terminal-row-with-a-live-pane repair, where tmux
	// has directly observed that the kill did not take. Both are cases where
	// the flag's whole purpose (an in-flight hook cannot undo an explicit
	// kill) is already served and keeping it would outrank every later hook
	// forever.
	KilledByUser      bool
	ClearKilledByUser bool
	// ClearCrashVerdict drops a stored pane_exit_status and crash tail. Like
	// ClearKilledByUser it belongs to the operations that have established the
	// verdict is spent: resume replaces the pane it described, and the §7
	// repair has observed a live pane under the row it froze. A crash verdict
	// that outlives its pane keeps the row terminal, which silently removes it
	// from reconciliation and blocks the transitions that would correct it.
	ClearCrashVerdict bool
	// ExpectedStatus makes a transition conditional on the row still having the
	// status the caller observed. Attach uses this to clear only a waiting
	// episode; a hook racing the keypress must not be overwritten by stale UI.
	ExpectedStatus string
	// PaneExitStatus and CrashTail persist a crash observation atomically with
	// its error verdict. The first pane observation wins.
	PaneExitStatus *int
	CrashTail      string
	// LastMessage is sourced from the hook payload, not the transcript. It is
	// stored as valid UTF-8 no larger than 2 KiB.
	LastMessage string
	// AllowedCurrentStatuses makes a caller's exhaustive transition policy part
	// of this transaction. A non-matching event is still recorded, but cannot
	// mutate any session metadata.
	AllowedCurrentStatuses []string
	// EventSeq, when non-nil, receives the seq of the event row this update
	// appended, so a caller that dispatches the event hook (SPEC §10.4) can
	// record the script's result against that very row. It is an output: the
	// store writes it after the row is inserted and before commit returns.
	EventSeq *int64
}

// EventInput records an event which could not be resolved to a session. It is
// deliberately separate from status transitions: a resolved event must use
// UpdateSessionStatus so the row and event cannot diverge.
type EventInput struct {
	At      int64
	Kind    string
	Reason  string
	Payload string
}

// Slug derives a tmux-safe session name component. In particular it never
// emits dot or colon, which tmux treats as target syntax separators.
func Slug(name string) string {
	var out strings.Builder
	separator := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			separator = false
		case r == '-' || r == '_':
			out.WriteRune(r)
			separator = false
		default:
			if out.Len() > 0 && !separator {
				out.WriteByte('-')
				separator = true
			}
		}
	}
	return strings.Trim(out.String(), "-_")
}

// CreateSession inserts exactly one session in a transaction. It intentionally
// uses an INSERT rather than a list rewrite so independent deck clients cannot
// overwrite each other's rows.
func (s *Store) CreateSession(ctx context.Context, input CreateSessionInput) (Session, error) {
	input, slug, err := normalizeCreateInput(input)
	if err != nil {
		return Session{}, err
	}
	launchArgsJSON, err := marshalStrings(input.LaunchArgs)
	if err != nil {
		return Session{}, fmt.Errorf("encode launch args: %w", err)
	}
	envJSON, err := marshalEnv(input.Env)
	if err != nil {
		return Session{}, fmt.Errorf("encode env: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, fmt.Errorf("begin create session: %w", err)
	}
	defer rollbackTx(tx)
	if err := claimSessionNameTx(ctx, tx, input, slug); err != nil {
		return Session{}, err
	}
	if err := insertSessionRowTx(ctx, tx, input, slug, launchArgsJSON, envJSON); err != nil {
		return Session{}, err
	}
	groupName, err := createdSessionGroupName(ctx, tx, input.GroupID)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("commit create session: %w", err)
	}
	return Session{
		ID: input.ID, Name: input.Name, Slug: slug, CWD: input.CWD, Agent: input.Agent,
		Status: input.Status, StatusSource: input.StatusSource, StatusAt: input.StatusAt, CreatedAt: input.CreatedAt,
		NameSource: input.NameSource, SeenStatusAt: input.StatusAt,
		LaunchArgs: input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, PostDestroy: input.PostDestroy, LoginShell: input.LoginShell,
		EventHookEnabled: input.EventHookEnabled, EventHookEvents: input.EventHookEvents,
		PermissionProfile: input.PermissionProfile, PermissionProfileReason: input.PermissionProfileReason,
		ConversationID: input.ConversationID,
		ResumePin:      input.ResumePin, ResumeState: input.ResumeState,
		GroupID: input.GroupID, GroupName: groupName,
	}, nil
}

// normalizeCreateInput validates a CreateSessionInput's required fields and
// fills the documented defaults, returning the normalised input and the slug
// derived from its name.
func normalizeCreateInput(input CreateSessionInput) (CreateSessionInput, string, error) {
	if input.ID == "" || input.Name == "" || input.CWD == "" || input.Agent == "" || input.CapturedPath == "" {
		return input, "", errors.New("session id, name, cwd, agent, and captured path are required")
	}
	slug := Slug(input.Name)
	if slug == "" {
		return input, "", fmt.Errorf("session name %q does not produce a usable slug", input.Name)
	}
	if input.StatusAt == 0 || input.CreatedAt == 0 {
		return input, "", errors.New("session status_at and created_at timestamps are required")
	}
	input, err := applyCreateDefaults(input)
	if err != nil {
		return input, "", err
	}
	return input, slug, nil
}

// applyCreateDefaults fills the documented defaults of a CreateSessionInput.
func applyCreateDefaults(input CreateSessionInput) (CreateSessionInput, error) {
	var err error
	input.Status = valueOrDefault(input.Status, "starting")
	input.StatusSource = valueOrDefault(input.StatusSource, "user")
	input.ResumeState = valueOrDefault(input.ResumeState, "auto")
	input.PermissionProfile = valueOrDefault(input.PermissionProfile, "safe")
	input.NameSource, err = normalizeNameSource(input.NameSource)
	return input, err
}

// normalizeNameSource defaults an empty provenance to "user" and refuses a
// value the sessions.name_source column would reject.
func normalizeNameSource(source string) (string, error) {
	source = valueOrDefault(source, "user")
	if source != "user" && source != "derived" && source != "harness" {
		return "", fmt.Errorf("session name source %q is not user, derived or harness", source)
	}
	return source, nil
}

// valueOrDefault returns def when value is empty.
func valueOrDefault(value, def string) string {
	if value == "" {
		return def
	}
	return value
}

// claimSessionNameTx frees the name and slug a new session wants, or reports
// why it cannot, inside CreateSession's transaction.
func claimSessionNameTx(ctx context.Context, tx *sql.Tx, input CreateSessionInput, slug string) error {
	// A row whose name is exactly input.Name always computes the same slug
	// (Slug is a pure function of the name), so an identical-name insert
	// violates BOTH the name and slug UNIQUE constraints at once; which one
	// the INSERT below reports is an accident of SQLite's own internal
	// constraint-check order, not something this package controls or can
	// rely on to distinguish "this exact name is taken" from "a different
	// name happens to slug the same". Checking name equality first, inside
	// this same transaction, makes the "already exists" message
	// deterministic and reachable rather than incidentally shadowed by the
	// slug branch below -- which remains exactly what surfaces a genuine
	// slug collision (a different name whose slug matches an existing row).
	//
	// SPEC.md §9.2 (R77): a name or slug held ONLY by a tombstoned row is
	// available again, and taking it reaps that row -- in this same
	// transaction, via reapTombstonedHolderTx, never a second BeginTx
	// against the shared *sql.DB (that would block on SQLite's single
	// writer lock behind this very tx). A live holder still refuses with the
	// bare "already exists"; an archived holder refuses too, but (task 007,
	// R78) with a message that names it and both routes out.
	if err := reapTombstonedHolderTx(ctx, tx, "name", input.Name, "",
		fmt.Sprintf("session name %q already exists", input.Name),
		func(holderName string) error {
			return fmt.Errorf("the archived session %q holds this name; press U to unarchive it, or dd to delete it, to free the name", holderName)
		}, input.CreatedAt); err != nil {
		return err
	}
	if err := reapTombstonedHolderTx(ctx, tx, "slug", slug, "",
		fmt.Sprintf("session name %q collides with existing slug %q", input.Name, slug),
		func(holderName string) error {
			return fmt.Errorf("session name %q collides with the archived session %q's slug; press U to unarchive it, or dd to delete it, to free the slug", input.Name, holderName)
		}, input.CreatedAt); err != nil {
		return err
	}
	return nil
}

// insertSessionRowTx INSERTs the session row inside CreateSession's
// transaction, translating a UNIQUE violation into the user-facing message.
func insertSessionRowTx(ctx context.Context, tx *sql.Tx, input CreateSessionInput, slug, launchArgsJSON, envJSON string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions
		(id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at,
		 launch_args, env, pre_launch, post_destroy, login_shell, permission_profile, permission_profile_reason, conversation_id, resume_pin, resume_state, group_id,
		 event_hook_enabled, event_hook_events, name_source, seen_status_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, slug, input.CWD, input.Agent, input.CapturedPath,
		input.Status, input.StatusSource, input.StatusAt, input.CreatedAt,
		launchArgsJSON, envJSON, nullableString(input.PreLaunch), nullableString(input.PostDestroy), input.LoginShell,
		input.PermissionProfile, nullableString(input.PermissionProfileReason), nullableString(input.ConversationID), nullableString(input.ResumePin), input.ResumeState,
		nullableGroupID(input.GroupID), eventHookEnabledArg(input.EventHookEnabled), eventHookEventsArg(input.EventHookEvents), input.NameSource, input.StatusAt)
	if err != nil {
		if strings.Contains(err.Error(), "sessions.name") || strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.name") {
			return fmt.Errorf("session name %q already exists", input.Name)
		}
		if strings.Contains(err.Error(), "sessions.slug") || strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.slug") {
			return fmt.Errorf("session name %q collides with existing slug %q", input.Name, slug)
		}
		return fmt.Errorf("insert session: %w", err)

	}
	return nil
}

// createdSessionGroupName resolves the created session's group name inside
// CreateSession's transaction.
func createdSessionGroupName(ctx context.Context, tx *sql.Tx, groupID *int64) (string, error) {
	// Resolve GroupName inside this same transaction, mirroring
	// sessionsFromClause's LEFT JOIN semantics (ListSessions/GetSession):
	// empty for a nil GroupID (the implicit default group) and for a
	// GroupID that names no live groups row (dangling membership renders
	// under default rather than vanishing, SPEC §11) -- never a second,
	// separate read after commit that could race a concurrent rename.
	var groupName string
	if groupID != nil {
		if err := tx.QueryRowContext(ctx, `SELECT name FROM groups WHERE id = ?`, *groupID).Scan(&groupName); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("resolve created session's group name: %w", err)
		}
	}
	return groupName, nil
}

// marshalStrings encodes a launch-args slice as a JSON array, defaulting a
// nil slice to an empty array so no reader ever sees a JSON null.
func marshalStrings(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// marshalEnv encodes the session env layer as a JSON object, defaulting a nil
// map to an empty object so no reader ever sees a JSON null.
func marshalEnv(values map[string]string) (string, error) {
	if values == nil {
		values = map[string]string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// nullableString turns an empty string into a SQL NULL so the column's
// nullability matches the schema's intent for "unset".
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nullableGroupID turns a nil *int64 into a SQL NULL, mirroring
// nullableString's identical purpose for CreateSessionInput.GroupID: NULL
// is the structural default group (SPEC §11), never a sentinel value like
// 0.
func nullableGroupID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

// scanSession reads the extended Phase 1 create fields shared by ListSessions
// and GetSession.
func scanSession(row interface {
	Scan(dest ...any) error
}) (Session, error) {
	var session Session
	var launchArgsJSON, envJSON string
	var preLaunch, permissionProfileReason, conversationID, resumePin, crashTail, lastMessage, postDestroy sql.NullString
	var groupID sql.NullInt64
	var groupName sql.NullString
	var leaseOwner string
	var loginShell, killedByUser, envDirty, launchDirty int
	var paneExitStatus, eventHookEnabled sql.NullInt64
	var eventHookEvents, pendingName sql.NullString
	var pendingAt, nameAdopt sql.NullInt64
	var nameAdopted int
	if err := row.Scan(&session.ID, &session.Name, &session.Slug, &session.CWD,
		&session.Agent, &session.CapturedPath, &session.Status, &session.StatusReason, &session.StatusSource,
		&session.StatusAt, &session.CreatedAt, &killedByUser, &paneExitStatus, &crashTail,
		&session.NotifyEpoch, &lastMessage, &session.SeenStatusAt, &launchArgsJSON, &envJSON, &preLaunch, &postDestroy,
		&loginShell, &session.PermissionProfile, &permissionProfileReason, &conversationID, &resumePin, &session.ResumeState,
		&groupID, &groupName, &session.LastProbeAt, &session.LastOutputAt, &session.LastSeenAt, &session.ProbeMissStreak, &session.LastEventAt, &envDirty, &session.DeletedAt, &session.ArchivedAt, &launchDirty,
		&leaseOwner, &session.PinnedAt, &session.HookExecutable, &eventHookEnabled, &eventHookEvents, &session.NameSource, &pendingName, &pendingAt, &nameAdopt, &nameAdopted, &session.HookAt, &session.StatusSeq); err != nil {
		return Session{}, err
	}
	session.NameAdopted = nameAdopted != 0
	session.EventHookEnabled, session.EventHookEvents = decodeEventHook(eventHookEnabled, eventHookEvents)
	session.PendingHarnessName, session.PendingHarnessNameAt = pendingName.String, pendingAt.Int64
	session.HarnessNameAdopt, _ = decodeEventHook(nameAdopt, sql.NullString{})
	// Only the generation half is surfaced: the launcher identity is lease
	// bookkeeping (SPEC §9.3) that no reader outside this package needs, while
	// the generation is the discriminator a hook hands back (issue #11, R74).
	_, session.LaunchGeneration = splitOwnerGeneration(leaseOwner)
	session.EnvDirty = envDirty != 0
	session.LaunchDirty = launchDirty != 0
	if groupID.Valid {
		id := groupID.Int64
		session.GroupID = &id
	}
	// groupName comes from the LEFT JOIN against groups: NULL both when
	// GroupID is nil (true default) and when GroupID names a group row
	// that no longer exists (SPEC §11's "renders under default rather
	// than vanishing") -- either way GroupName reads back empty, with no
	// cwd-derived fallback (that concept left with Workspace).
	session.GroupName = groupName.String
	if err := json.Unmarshal([]byte(launchArgsJSON), &session.LaunchArgs); err != nil {
		return Session{}, fmt.Errorf("decode launch args: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &session.Env); err != nil {
		return Session{}, fmt.Errorf("decode env: %w", err)
	}
	session.PreLaunch = preLaunch.String
	session.PostDestroy = postDestroy.String
	session.PermissionProfileReason = permissionProfileReason.String
	session.ConversationID = conversationID.String
	session.ResumePin = resumePin.String
	session.LoginShell = loginShell != 0
	session.KilledByUser = killedByUser != 0
	if paneExitStatus.Valid {
		status := int(paneExitStatus.Int64)
		session.PaneExitStatus = &status
	}
	session.CrashTail = crashTail.String
	session.LastMessage = lastMessage.String
	return session, nil
}

const sessionColumns = `sessions.id, sessions.name, slug, cwd, agent, captured_path, status,
		COALESCE(status_reason, ''), status_source, status_at, created_at,
		killed_by_user, pane_exit_status, crash_tail, notify_epoch, last_message, seen_status_at,
		launch_args, env, pre_launch, post_destroy, login_shell, permission_profile, permission_profile_reason, conversation_id, resume_pin, resume_state,
		sessions.group_id, groups.name, last_probe_at, last_output_at, last_seen_at, probe_miss_streak, COALESCE((SELECT MAX(events.at) FROM events WHERE events.session_id = sessions.id), 0), env_dirty, deleted_at, archived_at, launch_dirty,
		COALESCE(launch_lease_owner, ''), pinned_at, COALESCE(hook_executable, ''),
		event_hook_enabled, event_hook_events, name_source, pending_harness_name, pending_harness_name_at, harness_name_adopt, name_adopted, hook_at, status_seq`

// sessionsFromClause is every sessionColumns-backed query's shared FROM:
// a LEFT JOIN against groups so GroupName resolves (or reads back empty
// for the default group, or for a GroupID that no longer names a live
// group row -- SPEC §11's "renders under default rather than vanishing")
// in the same read as the session row itself, with no second query per
// row. LEFT, never an inner JOIN, so a session whose group_id is NULL
// (the common case) is not silently dropped from the result set.
const sessionsFromClause = `FROM sessions LEFT JOIN groups ON groups.id = sessions.group_id`

// GetSession returns exactly one session by id, including every Phase 1
// create field.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` `+sessionsFromClause+` WHERE sessions.id = ?`, id)
	session, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, fmt.Errorf("session %q not found", id)
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return session, nil
}

// statusUpdateChecks is UpdateSessionStatus's argument validation, in the
// order the errors are reported: the first rule whose invalid predicate
// holds names the error. It runs after the source default is applied, so the
// probe and killed_by_user rules see the effective source.
var statusUpdateChecks = []struct {
	invalid func(StatusUpdateInput) bool
	message string
}{
	{func(in StatusUpdateInput) bool { return in.SessionID == "" || in.Status == "" }, "session id and status are required"},
	{func(in StatusUpdateInput) bool { return in.At == 0 }, "session status timestamp is required"},
	{func(in StatusUpdateInput) bool { return in.Source == "probe" && in.StaleAfter <= 0 }, "probe stale_after is required"},
	{func(in StatusUpdateInput) bool {
		return in.KilledByUser && (in.Source != "user" || in.Status != "stopped")
	}, "killed_by_user requires a user-sourced stopped transition"},
}

// normalizeStatusUpdate fills the defaults an update may leave blank: the
// source is the user, and the event kind is the status itself.
func normalizeStatusUpdate(input StatusUpdateInput) StatusUpdateInput {
	if input.Source == "" {
		input.Source = "user"
	}
	if input.EventKind == "" {
		input.EventKind = input.Status
	}
	return input
}

// validateStatusUpdate returns the first failed rule of statusUpdateChecks.
func validateStatusUpdate(input StatusUpdateInput) error {
	for _, check := range statusUpdateChecks {
		if check.invalid(input) {
			return errors.New(check.message)
		}
	}
	return nil
}

// statusRow is the slice of a session row UpdateSessionStatus reads before
// deciding whether a verdict wins.
type statusRow struct {
	status, source, agent string
	at, notifyEpoch       int64
	hookAt                int64
	seq                   int64
	killedByUser          int
	seenStatusAt          int64
	paneExitStatus        sql.NullInt64
}

// readStatusRow loads the row a status update is judged against.
func readStatusRow(ctx context.Context, tx *sql.Tx, sessionID string) (statusRow, error) {
	var row statusRow
	if err := tx.QueryRowContext(ctx, `SELECT status, status_source, status_at, agent,
		killed_by_user, seen_status_at, notify_epoch, pane_exit_status, hook_at, status_seq
		FROM sessions WHERE id = ?`, sessionID).Scan(
		&row.status, &row.source, &row.at, &row.agent, &row.killedByUser,
		&row.seenStatusAt, &row.notifyEpoch, &row.paneExitStatus, &row.hookAt, &row.seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return statusRow{}, fmt.Errorf("session %q not found", sessionID)
		}
		return statusRow{}, fmt.Errorf("read session status: %w", err)
	}
	return row, nil
}

// statusVetoes are the precedence rules of a status update, each a pure
// predicate that is true when the incoming verdict must lose to the stored
// row. The event is recorded either way; any single veto leaves the row
// untouched, so the order is immaterial.
var statusVetoes = []func(in StatusUpdateInput, cur statusRow) bool{
	vetoKilledByUser,
	vetoDisallowedCurrentStatus,
	vetoHookOnStopped,
	vetoTmuxStopOverStopped,
	vetoHookRunningAfterCrash,
	vetoFreshHookOverProbe,
	vetoMovementOutOfBounds,
	vetoTmuxInventedState,
	vetoSecondCrashCollection,
}

// vetoKilledByUser: an explicit kill is a terminal verdict only a caller that
// says so (ClearKilledByUser, or the kill itself) may replace.
func vetoKilledByUser(in StatusUpdateInput, cur statusRow) bool {
	return cur.killedByUser != 0 && !in.ClearKilledByUser && !in.KilledByUser
}

// vetoDisallowedCurrentStatus: a transition limited to certain current
// statuses does not apply to any other.
func vetoDisallowedCurrentStatus(in StatusUpdateInput, cur statusRow) bool {
	return len(in.AllowedCurrentStatuses) > 0 && !containsString(in.AllowedCurrentStatuses, cur.status)
}

// vetoHookOnStopped: §7's transition table gives "stopped" no return edge except the explicit
// `r` resume (starting), which flips the row outside this function
// (AcquireLaunchLease). A stopped row means tmux verified the session is
// gone -- there is no live pane left for a hook to have truthfully come
// from, so a hook arriving for one anyway is stale/out-of-order and must
// not resurrect it. This is the hook-layer analogue of killed_by_user: a
// terminal state a later hook cannot undo.
//
// The one hook write that resurrects nothing is a hook "stopped" onto a
// tmux-sourced "stopped": the status does not change, and SPEC §7's
// precedence (hook > tmux; tmux "only ever supplies liveness") makes the
// agent's own end-of-session verdict -- its reason included -- the one
// to keep over the bare "tmux session disappeared" liveness fact. A
// SessionEnd hook subprocess is fire-and-forget and can land after the
// pane is gone and a reconcile pass has already recorded that fact;
// refusing it there made the stored verdict depend only on which writer
// won the race (status_claude_hooks.feature's clean-exit SessionEnd,
// task 026). A user-sourced stop still outranks every hook.
func vetoHookOnStopped(in StatusUpdateInput, cur statusRow) bool {
	hookRefinesTmuxStop := in.Status == "stopped" && cur.source == "tmux"
	return in.Source == "hook" && cur.status == "stopped" && !hookRefinesTmuxStop
}

// vetoTmuxStopOverStopped is the same precedence, read from the other side: a reconcile pass that
// read the row before that hook landed, then saw the session gone,
// writes tmux "stopped" onto a row a hook (or the user) already stopped.
// Liveness has nothing to add to a row that already says stopped, so it
// must not replace the higher-precedence verdict either.
func vetoTmuxStopOverStopped(in StatusUpdateInput, cur statusRow) bool {
	return in.Source == "tmux" && in.Status == "stopped" && cur.status == "stopped" && cur.source != "tmux"
}

// vetoHookRunningAfterCrash: an error carrying a pane exit status is a terminal process-crash verdict,
// not the recoverable turn/API failure represented by error -> running. A
// SessionStart lands on idle (R307) but is vetoed the same way: it is the
// event a replaced pane's agent still sends, and an idle landing would
// revive the crashed row exactly as the running one did.
func vetoHookRunningAfterCrash(in StatusUpdateInput, cur statusRow) bool {
	revives := in.Status == "running" || in.EventKind == "session_start"
	return in.Source == "hook" && revives && cur.paneExitStatus.Valid
}

// freshHook reports whether the stored row carries a hook verdict younger
// than the incoming write's StaleAfter. Freshness is measured from the newest
// hook evidence (hook_at), which a hook repeating the current status advances
// although it leaves status_at alone (R303); the later of the two stamps
// counts so a row never reads older than its own transition.
func freshHook(in StatusUpdateInput, cur statusRow) bool {
	return cur.source == "hook" && in.At-max(cur.at, cur.hookAt) < in.StaleAfter
}

// vetoFreshHookOverProbe: a probe never replaces a hook verdict that is still fresh.
func vetoFreshHookOverProbe(in StatusUpdateInput, cur statusRow) bool {
	return in.Source == "probe" && freshHook(in, cur)
}

// vetoMovementOutOfBounds: the output-movement fallback (R316) may only move
// an idle row to running, and only take back a running row it set itself. A
// non-zero StaleAfter also makes it lose to a hook verdict that is still fresh
// (pi passes none: it has no hook verdict to defer to).
func vetoMovementOutOfBounds(in StatusUpdateInput, cur statusRow) bool {
	if in.Source != "movement" {
		return false
	}
	if in.StaleAfter > 0 && freshHook(in, cur) {
		return true
	}
	enter := in.Status == "running" && cur.status == "idle"
	leave := in.Status == "idle" && cur.status == "running" && cur.source == "movement"
	return !enter && !leave
}

// vetoTmuxInventedState: tmux supplies terminal liveness, plus the one explicit shell promotion;
// it cannot invent an agent's working state.
func vetoTmuxInventedState(in StatusUpdateInput, cur statusRow) bool {
	if in.Source != "tmux" || in.Status == "stopped" || in.Status == "error" {
		return false
	}
	return !tmuxShellPromotion(in, cur) && !tmuxLaunchObservation(in, cur) && !tmuxTerminalRepair(in, cur)
}

func tmuxLaunchObservation(in StatusUpdateInput, cur statusRow) bool {
	return in.Status == "starting" && cur.status == "starting" && cur.source == "user"
}

func tmuxShellPromotion(in StatusUpdateInput, cur statusRow) bool {
	return in.Status == "running" && cur.agent == "shell" && cur.status == "starting"
}

// tmuxTerminalRepair is SPEC §7's one self-healing rule, as this gate sees it. Which rows are
// eligible is not decided here: reconcile.go's terminal-row branch repairs
// a stopped row always, but an error row only when it carries a pane-exit
// or tmux/user-sourced verdict. A bare hook- or probe-sourced error row is
// deliberately never repaired -- §7's running --turn or API failure--> error
// transition needs no pane death at all, so that row is the agent's own
// considered verdict (finding F40, task 901) -- and no repair write for one
// ever reaches this predicate. What the predicate does is admit the repair
// write that does arrive: it is from tmux liveness alone, so a shell
// promotes straight to running (its only rule) and everything else resets
// to the neutral starting a fresh pane always begins at, since tmux still
// cannot fabricate an agent's working state even while repairing this
// violation.
//
// The repair is also the one tmux write allowed past the two terminal
// verdicts, and only because tmux has observed the pane they describe to be
// alive: killed_by_user exists so an in-flight hook cannot undo an explicit
// kill, and a crash verdict describes a pane that died. Neither claim
// survives a live pane, and leaving either one set would repair the status
// into a row that still outranks every later hook forever (SPEC §9.1's
// "spent verdict" hazard), i.e. exactly the frozen row §7 sends the repair
// to fix. The reconciler therefore passes ClearKilledByUser and
// ClearCrashVerdict with it, which is what makes this branch reachable
// through the killed_by_user precedence above.
func tmuxTerminalRepair(in StatusUpdateInput, cur statusRow) bool {
	if cur.status != "stopped" && cur.status != "error" {
		return false
	}
	if cur.agent == "shell" {
		return in.Status == "running"
	}
	return in.Status == "starting"
}

// vetoSecondCrashCollection: crash collection is first-writer-only. A racing observer still records
// what it saw, but does not replace the stored verdict or tail.
func vetoSecondCrashCollection(in StatusUpdateInput, cur statusRow) bool {
	return in.PaneExitStatus != nil && cur.paneExitStatus.Valid
}

// statusWins reports whether the update replaces the stored verdict.
func statusWins(in StatusUpdateInput, cur statusRow) bool {
	for _, veto := range statusVetoes {
		if veto(in, cur) {
			return false
		}
	}
	return true
}

// nextSeenStatusAt is the seen stamp a verdict leaves on the row. A transition
// into an attention status opens an unseen episode whatever stamp it carries,
// so a status_at equal to an older seen stamp cannot hide it. Any other
// transition keeps the row's seen answer (what the acknowledged bit did): a
// seen row stays seen at its new stamp, an unseen one stays unseen.
func nextSeenStatusAt(input StatusUpdateInput, cur statusRow, repeat bool, statusAt int64) int64 {
	switch {
	case repeat:
		return cur.seenStatusAt
	case isAttentionStatus(input.Status):
		return 0
	case cur.seenStatusAt == cur.at:
		return statusAt
	}
	return cur.seenStatusAt
}

// nextStatusSeq is status_seq after a winning write (R319, R321, R322): it
// counts the row's status transitions, so (status_at, status_seq) names one
// status episode even when two episodes share a millisecond or the wall clock
// stepped backwards. A write repeating the current status opens no episode
// (R303) and leaves it alone.
func nextStatusSeq(cur statusRow, repeat bool) int64 {
	if repeat {
		return cur.seq
	}
	return cur.seq + 1
}

// nextHookAt is hook_at after a winning write (R303, SPEC §7). Only a hook
// advances it, and it never moves backwards while the row stays hook-sourced:
// hook deliveries can arrive out of order, and a delayed repeat dated before
// a newer one must not age the newest hook evidence. A hook taking the row
// over from another source starts the clock at its own time.
func nextHookAt(in StatusUpdateInput, cur statusRow) int64 {
	switch {
	case in.Source != "hook":
		return cur.hookAt
	case cur.source == "hook":
		return max(cur.hookAt, in.At)
	}
	return in.At
}

// nextKilledByUser is killed_by_user after the write: set wins over clear.
func nextKilledByUser(input StatusUpdateInput, cur statusRow) int {
	switch {
	case input.KilledByUser:
		return 1
	case input.ClearKilledByUser:
		return 0
	}
	return cur.killedByUser
}

// writeStatusUpdate stores the winning verdict on the session row.
//
// A write that leaves the status unchanged is inert (SPEC §7, R303): it
// keeps status_at, notify_epoch, hook_fired and the seen state exactly
// as stored, and updates only the reason, the source the precedence rules let
// win (statusWins decides that exactly as for a transition) and the side
// columns the write explicitly carries (kill/crash verdict, last message). A repeat is the same status being
// reported again, not a transition, so it has no moment to date, no
// episode to open or close and nothing to mark unseen.
func writeStatusUpdate(ctx context.Context, tx *sql.Tx, input StatusUpdateInput, cur statusRow) error {
	repeat := input.Status == cur.status
	statusAt := input.At
	if repeat {
		statusAt = cur.at
	}
	notifyEpoch := cur.notifyEpoch
	if isAttentionStatus(cur.status) && !isAttentionStatus(input.Status) {
		notifyEpoch++
	}
	seenStatusAt := nextSeenStatusAt(input, cur, repeat, statusAt)
	newKilled := nextKilledByUser(input, cur)
	lastMessage := truncateUTF8(input.LastMessage, 2*1024)
	// clearCrash drops both crash columns; otherwise they keep their existing
	// pre-clear behaviour, where a tail is written only alongside the exit
	// status that explains it (the crash_tail CASE is gated on
	// input.PaneExitStatus, never on the tail string itself, which is empty
	// on every unrelated write).
	clearCrash := boolInt(input.ClearCrashVerdict)
	_, err := tx.ExecContext(ctx, `UPDATE sessions SET
		status = ?, status_reason = ?, status_source = ?, status_at = ?,
		killed_by_user = ?, seen_status_at = ?, notify_epoch = ?,
		hook_fired = CASE WHEN ? THEN NULL ELSE hook_fired END,
		pane_exit_status = CASE WHEN ? = 1 THEN NULL ELSE COALESCE(?, pane_exit_status) END,
		crash_tail = CASE WHEN ? = 1 THEN NULL WHEN ? IS NULL THEN crash_tail ELSE ? END,
		last_message = CASE WHEN ? = '' THEN last_message ELSE ? END,
		hook_at = ?, status_seq = ?,
		probe_miss_streak = 0
		WHERE id = ?`, input.Status, input.Reason, input.Source, statusAt,
		newKilled, seenStatusAt, notifyEpoch, notifyEpoch != cur.notifyEpoch, clearCrash, input.PaneExitStatus,
		clearCrash, input.PaneExitStatus, input.CrashTail, lastMessage, lastMessage, nextHookAt(input, cur), nextStatusSeq(cur, repeat), input.SessionID)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	return nil
}

// UpdateSessionStatus applies one verdict and records its source event in the
// same transaction. Losing verdicts are still events (important evidence that
// a probe ran), but cannot change the row. The immediate transaction configured
// by OpenPath makes the read/precedence/write sequence atomic across clients.
func (s *Store) UpdateSessionStatus(ctx context.Context, input StatusUpdateInput) error {
	input = normalizeStatusUpdate(input)
	if err := validateStatusUpdate(input); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session status update: %w", err)
	}
	defer rollbackTx(tx)

	cur, err := readStatusRow(ctx, tx, input.SessionID)
	if err != nil {
		return err
	}

	// A conditional transition that lost its race is a complete no-op: it must
	// not append an event claiming an action against a different current state.
	if input.ExpectedStatus != "" && cur.status != input.ExpectedStatus {
		return nil
	}

	if err := applyStatusVerdict(ctx, tx, input, cur); err != nil {
		return err
	}

	return recordStatusEventTx(ctx, tx, input)
}

// applyStatusVerdict writes a winning verdict; a verdict that lost precedence
// is still proof the pane matched a probe rule or the agent spoke, so a probe
// or hook loser ends the miss streak (R315).
func applyStatusVerdict(ctx context.Context, tx *sql.Tx, input StatusUpdateInput, cur statusRow) error {
	if statusWins(input, cur) {
		return writeStatusUpdate(ctx, tx, input, cur)
	}
	if input.Source == "probe" || input.Source == "hook" {
		return clearProbeMissStreakTx(ctx, tx, input.SessionID)
	}
	return nil
}

// clearProbeMissStreakTx zeroes sessions.probe_miss_streak (R315) with a plain
// UPDATE: the streak is a column, never an event.
func clearProbeMissStreakTx(ctx context.Context, tx *sql.Tx, sessionID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET probe_miss_streak = 0 WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("clear probe_miss_streak: %w", err)
	}
	return nil
}

// recordStatusEventTx appends the status update's source event and commits
// UpdateSessionStatus's transaction.
func recordStatusEventTx(ctx context.Context, tx *sql.Tx, input StatusUpdateInput) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, input.SessionID, input.At, input.EventKind, input.Reason, input.Payload)
	if err != nil {
		return fmt.Errorf("record session status event: %w", err)
	}
	if input.EventSeq != nil {
		if *input.EventSeq, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("read session status event seq: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session status update: %w", err)
	}
	return nil
}

// AcknowledgeSession is Y's durable write (R319): it clears the row's unseen
// marker without changing its status verdict, source, timestamp, or
// notification epoch. It is intentionally a targeted update rather than a
// status transition: pressing Y acknowledges waiting/error but does not claim
// that the user answered it. Like engagement's MarkStatusSeen it names the
// status episode the user pressed Y on (status_at AND status_seq): the key's
// write runs asynchronously, so by the time it lands the row may already be in
// a newer episode -- even one sharing the old stamp, or one whose stepped-back
// clock repeated it -- and acknowledging the older prompt must never mark that
// newer one seen. A row that has moved on, or is already seen, is left
// untouched (no write at all); only a missing row is an error.
func (s *Store) AcknowledgeSession(ctx context.Context, sessionID string, episode StatusEpisode) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if _, err := s.MarkStatusSeen(ctx, sessionID, episode); err != nil {
		return fmt.Errorf("acknowledge session: %w", err)
	}
	var exists int
	switch err := s.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, sessionID).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("session %q not found", sessionID)
	case err != nil:
		return fmt.Errorf("read acknowledged session: %w", err)
	}
	return nil
}

// MarkStatusSeen is engagement's (R321) one durable write: it sets
// seen_status_at = status_at on a row that is still in the status episode the
// caller saw (StatusEpisode: the stamp AND the transition count, because a
// stamp alone is not an identity -- two episodes can share a millisecond and
// a stepped clock can repeat an old stamp). It changes no status, source,
// reason, status_at, notify_epoch or event, and it writes nothing at all when
// the row is already seen or has moved to another episode since the caller
// looked (a newer prompt must not be marked seen by an engagement with an
// older one, however late that write runs). It reports whether a row was
// written.
func (s *Store) MarkStatusSeen(ctx context.Context, sessionID string, episode StatusEpisode) (bool, error) {
	if sessionID == "" {
		return false, errors.New("session id is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET seen_status_at = status_at
		 WHERE id = ? AND status_at = ? AND status_seq = ? AND seen_status_at <> status_at`,
		sessionID, episode.At, episode.Seq)
	if err != nil {
		return false, fmt.Errorf("mark status seen: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read marked-seen session count: %w", err)
	}
	return changed > 0, nil
}

// RecordSessionNote appends one session-scoped "note" event (task 013,
// SPEC §9.2's fail-open teardown report) without touching any other column
// on the row: unlike UpdateSessionStatus it never rewrites status, source,
// reason or any of the other fields that transition carries, because the
// archive/delete this note follows has already durably committed and a
// teardown hook's own failure must never be confused with -- or overwrite
// the record of -- why the session itself stopped. It is deliberately not
// RecordOrphanEvent: that helper always writes session_id = NULL, and this
// note belongs to a specific, still-present row.
func (s *Store) RecordSessionNote(ctx context.Context, sessionID, reason string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, 'note', ?, '')`, sessionID, at, reason); err != nil {
		return fmt.Errorf("record session note: %w", err)
	}
	return nil
}

// RecordTranscript records, as an event on the row, the transcript file an
// agent's own hook named for it (Copilot's agentStop transcriptPath). The
// caller has already established that the path is a regular file under the
// agent's root; the store keeps the evidence and never opens it.
func (s *Store) RecordTranscript(ctx context.Context, sessionID, path string, at int64) error {
	if sessionID == "" || path == "" {
		return errors.New("session id and transcript path are required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, 'transcript', 'hook', ?)`, sessionID, at, path); err != nil {
		return fmt.Errorf("record transcript: %w", err)
	}
	return nil
}

// RecordOrphanEvent preserves a hook event which could not be resolved to a
// session. NULL (not an empty id) is used so the foreign key remains honest.
func (s *Store) RecordOrphanEvent(ctx context.Context, input EventInput) error {
	if input.At == 0 {
		return errors.New("event timestamp is required")
	}
	if input.Kind == "" {
		return errors.New("event kind is required")
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (NULL, ?, ?, ?, ?)`, input.At, input.Kind, input.Reason, input.Payload); err != nil {
		return fmt.Errorf("record orphan event: %w", err)
	}
	return nil
}

func isAttentionStatus(status string) bool { return status == "waiting" || status == "error" }

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

// UnverifiedMissStreak and UnverifiedAgeFactor are R315's thresholds: a row is
// unverified after this many consecutive probe misses and this multiple of
// stale_after with no event.
const (
	UnverifiedMissStreak = 3
	UnverifiedAgeFactor  = 3
)

// Unverified reports whether a `running` row has gone unconfirmed (R315): no
// event has been recorded for it for at least UnverifiedAgeFactor x
// staleAfter (measured from the newer of LastEventAt and status_at, so a
// repeat verdict that leaves status_at alone still restarts the window) and
// its probe has missed UnverifiedMissStreak times in a row. It is derived
// for display only: it reads no stored flag, writes nothing, and changes
// neither the status, its text nor the sort position. staleAfter <= 0 means
// probing is off, so nothing is ever unverified.
func (s Session) Unverified(now time.Time, staleAfter time.Duration) bool {
	if staleAfter <= 0 || s.Status != "running" || s.ProbeMissStreak < UnverifiedMissStreak {
		return false
	}
	quietSince := max(s.StatusAt, s.LastEventAt)
	return now.UnixMilli()-quietSince >= UnverifiedAgeFactor*staleAfter.Milliseconds()
}

// RecordProbeMiss notes that a stale-eligible pane was sampled but matched
// no probe.go rule at all, without changing the row's status, status_source,
// or status_at — SPEC §7 precedence and every existing transition are
// untouched by a miss. It exists solely so the `i` detail dialog can
// distinguish "sampled, no rule matched" from "never sampled" (task 009).
//
// SPEC §7 amendment (steer 3e-001, R59): "a diagnostic sampling result is a
// column, not an event" — a probe miss overwrites sessions.last_probe_at ONLY
// and never appends to events. At the default reconcile cadence one
// unmatched session alone would otherwise write two rows a second forever,
// with no reader for the kind (the general rule: a kind is only written if
// something reads it). This intentionally uses a plain UPDATE, not
// mutateSessionWithEvent, because that helper always pairs its UPDATE with
// an events INSERT in the same transaction.
func (s *Store) RecordProbeMiss(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_probe_at = ?, probe_miss_streak = probe_miss_streak + 1 WHERE id = ?`, at, sessionID)
	if err != nil {
		return fmt.Errorf("set last_probe_at: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check set last_probe_at: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// RecordOutput sets sessions.last_output_at to at (R304). Like RecordProbeMiss
// it is a plain UPDATE of one column and never appends to events: the output
// clock advances once per bucket per session, and an event kind nothing reads
// would grow the log for as long as an agent works. It does not touch status,
// status_source or status_at.
func (s *Store) RecordOutput(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at <= 0 {
		return errors.New("output timestamp is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_output_at = ? WHERE id = ?`, at, sessionID)
	if err != nil {
		return fmt.Errorf("set last_output_at: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check set last_output_at: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// SetPermissionProfileReason stores the reason the detail pane shows on its
// `degraded:` line, with a matching event, without touching status. It is how
// a sampled pane that contradicts the launched profile is reported.
func (s *Store) SetPermissionProfileReason(ctx context.Context, sessionID, reason, source string, at int64) error {
	if sessionID == "" || reason == "" {
		return errors.New("session id and reason are required")
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "permission profile reason", "profile.elevated", source, reason, at,
		`UPDATE sessions SET permission_profile_reason = ? WHERE id = ?`, reason)
}

// SetConversationID records the conversation identity assigned to (or
// pinned for) a session, alongside an event so observers can see when and by
// what source the identity was set.
func (s *Store) SetConversationID(ctx context.Context, sessionID, conversationID, source string, at int64) error {
	if sessionID == "" || conversationID == "" {
		return errors.New("session id and conversation id are required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "conversation_id", "set_conversation_id", source, conversationID, at,
		`UPDATE sessions SET conversation_id = ? WHERE id = ?`, conversationID)
}

// SetPermissionProfile persists a new permission profile for an existing
// session. It never touches a live pane; callers must state separately that
// the change applies on the next launch/restart.
func (s *Store) SetPermissionProfile(ctx context.Context, sessionID, profile, source string, at int64) error {
	if sessionID == "" || profile == "" {
		return errors.New("session id and permission profile are required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "permission_profile", "set_permission_profile", source, profile, at,
		`UPDATE sessions SET permission_profile = ? WHERE id = ?`, profile)
}

// validateSetEnvArgs refuses a SetSessionEnvValue call missing its row, key or
// event timestamp.
func validateSetEnvArgs(sessionID, key string, at int64) error {
	if sessionID == "" || key == "" {
		return errors.New("session id and environment key are required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	return nil
}

// SetSessionEnvValue edits exactly one key in a session's own env map --
// SPEC §6.1/§6.3's highest-priority layer -- and marks the row env_dirty
// (task 021): a change has been persisted but not yet applied to the live
// pane's already-running process. Only an explicit restart (task 022's `R`)
// both re-launches the pane with it and clears env_dirty back to false;
// this method itself never touches tmux at all (Service.SetSessionEnv
// mirrors the same key into tmux's own environment table for future panes
// in the same call, as a separate, tmux-side step). The recorded event's
// payload is the key name only, matching internal/audit's own
// never-log-a-value convention (task 019/024): a value a user typed here
// may be secret-shaped, and nothing durable should ever quote it back.
func (s *Store) SetSessionEnvValue(ctx context.Context, sessionID, key, value, source string, at int64) error {
	if err := validateSetEnvArgs(sessionID, key, at); err != nil {
		return err
	}
	if source == "" {
		source = "user"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set session env: %w", err)
	}
	defer rollbackTx(tx)
	if err := writeSessionEnvKeyTx(ctx, tx, sessionID, key, value); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, ?, ?, ?, ?)`,
		sessionID, at, "set_env", source, key); err != nil {
		return fmt.Errorf("record set session %q env event: %w", sessionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set session %q env: %w", sessionID, err)
	}
	return nil
}

// readSessionEnvTx reads and decodes one row's env map inside tx; an empty
// stored value decodes to an empty (non-nil) map.
func readSessionEnvTx(ctx context.Context, tx *sql.Tx, sessionID string) (map[string]string, error) {
	var envJSON string
	if err := tx.QueryRowContext(ctx, `SELECT env FROM sessions WHERE id = ?`, sessionID).Scan(&envJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session %q not found", sessionID)
		}
		return nil, fmt.Errorf("read session %q env: %w", sessionID, err)
	}
	env := map[string]string{}
	if envJSON != "" {
		if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
			return nil, fmt.Errorf("decode session %q env: %w", sessionID, err)
		}
	}
	return env, nil
}

// writeSessionEnvKeyTx reads the row's env map inside tx, sets one key and
// writes the map back with env_dirty = 1; SetSessionEnvValue records the
// event and commits afterwards.
func writeSessionEnvKeyTx(ctx context.Context, tx *sql.Tx, sessionID, key, value string) error {
	env, err := readSessionEnvTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	env[key] = value
	newEnvJSON, err := marshalEnv(env)
	if err != nil {
		return fmt.Errorf("encode session %q env: %w", sessionID, err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET env = ?, env_dirty = 1 WHERE id = ?`, newEnvJSON, sessionID)
	if err != nil {
		return fmt.Errorf("set session %q env: %w", sessionID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check set session %q env: %w", sessionID, err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// ClearEnvDirty resets env_dirty back to false once an explicit restart
// (task 022's `R`) has relaunched the pane carrying the session's
// persisted environment, so the `env↻` pending-change badge never
// outlives the change actually reaching a live process. It never touches
// tmux or the sessions.env column itself -- only the flag.
func (s *Store) ClearEnvDirty(ctx context.Context, sessionID string, at int64) error {
	return s.clearEnvDirtyWithReason(ctx, sessionID, "restart", at)
}

// MarkEnvInjected clears env_dirty exactly like ClearEnvDirty, but records
// the apply event's reason as "inject" instead of "restart": task 023's
// inject-instead path (SPEC §6.2/§6.3, shell sessions only) applies a
// pending edit straight into a live shell's already-running process via
// `export`, without ever killing or relaunching its pane, so the badge
// must clear the same way a restart's own apply does.
func (s *Store) MarkEnvInjected(ctx context.Context, sessionID string, at int64) error {
	return s.clearEnvDirtyWithReason(ctx, sessionID, "inject", at)
}

func (s *Store) clearEnvDirtyWithReason(ctx context.Context, sessionID, reason string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin clear env_dirty: %w", err)
	}
	defer rollbackTx(tx)
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET env_dirty = 0 WHERE id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("clear env_dirty for session %q: %w", sessionID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check clear env_dirty for session %q: %w", sessionID, err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload) VALUES (?, ?, ?, ?, ?)`,
		sessionID, at, "env_applied", reason, ""); err != nil {
		return fmt.Errorf("record clear env_dirty event for session %q: %w", sessionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear env_dirty for session %q: %w", sessionID, err)
	}
	return nil
}

// DirtyEnvKeys lists the env keys changed via SetSessionEnvValue since the
// row's env_dirty flag was last cleared (ClearEnvDirty/MarkEnvInjected),
// in the order each was first changed. Task 023's inject-instead path
// uses this to export exactly the keys a pending edit actually touched
// into a live shell -- never the whole persisted env, most of which the
// pane already has correctly from when it started.
func (s *Store) DirtyEnvKeys(ctx context.Context, sessionID string) ([]string, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	var lastApplied sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(at) FROM events WHERE session_id = ? AND kind = 'env_applied'`, sessionID).Scan(&lastApplied); err != nil {
		return nil, fmt.Errorf("read last env_applied event for session %q: %w", sessionID, err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload, MIN(at) AS first_at FROM events WHERE session_id = ? AND kind = 'set_env' AND at > ? GROUP BY payload ORDER BY first_at ASC`, sessionID, lastApplied.Int64)
	if err != nil {
		return nil, fmt.Errorf("list changed env keys for session %q: %w", sessionID, err)
	}
	defer closeRows(rows)
	var keys []string
	for rows.Next() {
		var key string
		var firstAt int64
		if err := rows.Scan(&key, &firstAt); err != nil {
			return nil, fmt.Errorf("scan changed env key for session %q: %w", sessionID, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list changed env keys for session %q: %w", sessionID, err)
	}
	return keys, nil
}

// SetLaunchInputs persists a new value for all four editable launch inputs
// -- PreLaunch, PostDestroy, LaunchArgs and LoginShell -- and marks the row
// launch_dirty (task 020's own launch-side counterpart to SetSessionEnvValue
// and env_dirty): a change has been durably written but not yet applied to
// the live pane, because none of these four inputs can be changed on an
// already-running pane without relaunching it. All four columns are written
// in the same UPDATE as launch_dirty = 1 so a reader never observes one
// without the other. Only an explicit relaunch clears launch_dirty back to
// 0 (ClearLaunchDirty); this method itself never touches tmux.
func (s *Store) SetLaunchInputs(ctx context.Context, sessionID, preLaunch, postDestroy string, launchArgs []string, loginShell bool, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if source == "" {
		source = "user"
	}
	launchArgsJSON, err := marshalStrings(launchArgs)
	if err != nil {
		return fmt.Errorf("encode launch args for session %q: %w", sessionID, err)
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "launch_inputs", "set_launch_inputs", source, "", at,
		`UPDATE sessions SET pre_launch = ?, post_destroy = ?, launch_args = ?, login_shell = ?, launch_dirty = 1 WHERE id = ?`,
		nullableString(preLaunch), nullableString(postDestroy), launchArgsJSON, loginShell)
}

// ClearLaunchDirty resets launch_dirty back to false once a relaunch has
// carried the session's persisted launch inputs into a live pane, so the
// pending-change badge never outlives the change actually reaching a live
// process. It never touches tmux or any of the four launch-input columns
// themselves -- only the flag.
func (s *Store) ClearLaunchDirty(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "launch_dirty", "launch_applied", "restart", "", at,
		`UPDATE sessions SET launch_dirty = 0 WHERE id = ?`)
}

// SetSessionGroup moves a session into a different group (SPEC §11,
// R130 part 2): the `i` detail dialog's `g` picker calls this exactly the
// way RenameSession moves a session's display name -- one targeted
// UPDATE, recorded as an event in the same transaction
// (mutateSessionWithEvent), never touching any other column.
//
// groupID <= 0 moves the session to the structural default group
// (sessions.group_id = NULL, SPEC §11: "default is not a row"). A
// positive groupID is written verbatim with no existence check, mirroring
// CreateSessionInput.GroupID and schemaV7's own deliberate absence of a
// foreign key (store.go:2330's comment on schemaV7 explains why): a
// groupID naming a group deleted by another client between the picker's
// fetch and this call is not this store's problem to catch -- the row
// simply renders under default on the next load (SPEC §11's own dangling-
// group_id rule), exactly the same graceful degrade every other group_id
// write already gets.
func (s *Store) SetSessionGroup(ctx context.Context, sessionID string, groupID int64, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if source == "" {
		source = "user"
	}
	var groupIDArg any
	payload := ""
	if groupID > 0 {
		groupIDArg = groupID
		payload = strconv.FormatInt(groupID, 10)
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "group_id", "set_group", source, payload, at,
		`UPDATE sessions SET group_id = ? WHERE id = ?`, groupIDArg)
}

// SetSessionsPinned pins or unpins a batch of sidebar sessions (task 003,
// SPEC §11.3's sidebar pin `p`, phase 4f) in ONE transaction: one
// `UPDATE sessions SET pinned_at = ? WHERE id = ?` per id in `ids`, all
// committed together or, on any single id's failure, none of them applied
// at all (defer rollbackTx(tx) below undoes every earlier iteration's
// UPDATE in the same call the moment any later one errors). `at` (the
// store clock, exactly like every other mutator here) is written verbatim
// when pinned is true; unpinning always writes 0, never `at`, so pinned_at
// only ever holds a real pin timestamp or the zero "not pinned" sentinel
// CreateSession's schemaV8 default already establishes. Unlike
// mutateSessionWithEvent's single-row mutators, this records no event at
// all: a pin/unpin is UI-only sidebar-ordering state, not an auditable
// session lifecycle transition, so the events table is untouched by this
// call in every case, including a rolled-back one. An id absent from
// `sessions` (already deleted, e.g.) simply UPDATEs zero rows and is not
// an error -- callers pass ids they already believe exist, so treating a
// races-with-a-delete miss as fatal would abort every other id's pin in
// the same batch for no benefit.
func (s *Store) SetSessionsPinned(ctx context.Context, ids []string, pinned bool, at int64) error {
	if len(ids) == 0 {
		return nil
	}
	value := int64(0)
	if pinned {
		value = at
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set sessions pinned: %w", err)
	}
	defer rollbackTx(tx)
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET pinned_at = ? WHERE id = ?`, value, id); err != nil {
			return fmt.Errorf("set pinned for session %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set sessions pinned: %w", err)
	}
	return nil
}

// SetResumePin pins a session to resume a specific conversation id going
// forward (resume_state=pinned), sticky across restarts until changed again.
func (s *Store) SetResumePin(ctx context.Context, sessionID, conversationID, source string, at int64) error {
	if sessionID == "" || conversationID == "" {
		return errors.New("session id and conversation id are required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "resume_pin", "set_resume_pin", source, conversationID, at,
		`UPDATE sessions SET resume_pin = ?, resume_state = 'pinned' WHERE id = ?`, conversationID)
}

// SetResumeStateAuto clears any pin and returns the session to the default
// auto resume behavior (resume the session's own last-known conversation).
func (s *Store) SetResumeStateAuto(ctx context.Context, sessionID, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "resume_state", "set_resume_state", source, "auto", at,
		`UPDATE sessions SET resume_pin = NULL, resume_state = 'auto' WHERE id = ?`)
}

// SetResumeStateFreshOnce arms a one-shot "start fresh" launch: the very next
// resume/launch is expected to start a brand-new conversation rather than
// resuming the pinned or last-known one. Callers must pair this with
// ConsumeFreshOnce once the fresh launch has actually happened, which reverts
// resume_state to auto (never back to pinned, and never left as fresh-once).
func (s *Store) SetResumeStateFreshOnce(ctx context.Context, sessionID, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if source == "" {
		source = "user"
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "resume_state", "set_resume_state", source, "fresh-once", at,
		`UPDATE sessions SET resume_state = 'fresh-once' WHERE id = ?`)
}

// ConsumeFreshOnce reverts resume_state from fresh-once back to auto after
// the one-shot fresh launch has happened, so a subsequent resume goes back to
// the normal auto behavior rather than starting fresh again or staying
// pinned. It is a no-op (but not an error) if the session was not in
// fresh-once state, since a caller that raced another mutator should not
// clobber a newer pin.
func (s *Store) ConsumeFreshOnce(ctx context.Context, sessionID, source string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	if source == "" {
		source = "user"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin consume fresh-once: %w", err)
	}
	defer rollbackTx(tx)
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET resume_state = 'auto'
		WHERE id = ? AND resume_state = 'fresh-once'`, sessionID)
	if err != nil {
		return fmt.Errorf("consume fresh-once: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check consume fresh-once: %w", err)
	}
	if affected == 0 {
		// Either the session does not exist, or it was not fresh-once; both
		// are treated as a benign no-op rather than an error so a caller
		// consuming its own prior fresh-once request cannot be defeated by
		// a concurrent, unrelated mutation.
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, sessionID, at, "set_resume_state", source, "auto"); err != nil {
		return fmt.Errorf("record consume fresh-once event: %w", err)
	}
	return tx.Commit()
}

// RenameSession implements the `i` detail dialog's rename action (SPEC
// §11.4, PRD requirement 31, I-8): it changes ONLY the session's display
// `name` column, never its `slug`. `slug` is fixed at CreateSession time and
// is the tmux session's actual identity (`deck_<slug>`, §3.2), so a rename
// never moves, recreates, or otherwise touches a live tmux session -- deck's
// display name and tmux's session name are deliberately decoupled by this
// method, not merely by omission. It still re-validates uniqueness against
// BOTH columns exactly as CreateSession does: newName must not already be
// some other session's display name, and Slug(newName) must not collide
// with some other session's own (already assigned, immutable) slug --
// either would leave the store in a state CreateSession could never
// legally reach in the first place.
func (s *Store) RenameSession(ctx context.Context, sessionID, newName, source string, at int64) error {
	_, err := s.renameSession(ctx, sessionID, newName, source, nameSourceFor(source), false, at)
	return err
}

// AdoptSessionName is RenameSession for a name the harness chose (SPEC §8
// adoption): it records a renamed event with source (harness:<kind>) and
// writes nameSource ('harness', or 'user' when the harness says a human chose
// the name). The check that the row is still 'derived' or 'harness' runs in
// the same transaction as the write, so a user rename that lands between the
// caller's read and this call is never overwritten; adopted is false then,
// and nothing is written.
func (s *Store) AdoptSessionName(ctx context.Context, sessionID, newName, source, nameSource string, at int64) (adopted bool, err error) {
	if nameSource != "harness" && nameSource != "user" {
		return false, fmt.Errorf("adopted name source %q must be harness or user", nameSource)
	}
	return s.renameSession(ctx, sessionID, newName, source, nameSource, true, at)
}

// FreezeSessionName records that a human chose the row's current name inside
// the harness without changing the name: name_source becomes 'user' and
// nothing else (the name, the slug and the events stay as they are). It
// reports whether the row changed; a row already 'user' is left alone.
func (s *Store) FreezeSessionName(ctx context.Context, sessionID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET name_source = 'user', name_adopted = 1 WHERE id = ? AND name_source != 'user'`, sessionID)
	if err != nil {
		return false, fmt.Errorf("freeze session name: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check freeze session name: %w", err)
	}
	return affected > 0, nil
}

// renameSession is the one rename transaction. adoptableOnly makes it a no-op
// (false, nil) for a row whose name_source is 'user'.
func (s *Store) renameSession(ctx context.Context, sessionID, newName, source, nameSource string, adoptableOnly bool, at int64) (bool, error) {
	slug, err := renameSlug(sessionID, newName, at)
	if err != nil {
		return false, err
	}
	if source == "" {
		source = "user"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin rename session: %w", err)
	}
	defer rollbackTx(tx)
	if adoptableOnly {
		if frozen, err := nameFrozenTx(ctx, tx, sessionID); frozen || err != nil {
			return false, err
		}
	}
	if err := writeRenameTx(ctx, tx, sessionID, newName, slug, source, nameSource, adoptableOnly, at); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit rename session: %w", err)
	}
	return true, nil
}

// writeRenameTx claims the name, writes it and records the renamed event.
func writeRenameTx(ctx context.Context, tx *sql.Tx, sessionID, newName, slug, source, nameSource string, adopted bool, at int64) error {
	if err := claimRenameNameTx(ctx, tx, sessionID, newName, slug, at); err != nil {
		return err
	}
	if err := writeSessionNameTx(ctx, tx, sessionID, newName, nameSource, adopted); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, sessionID, at, "renamed", source, newName); err != nil {
		return fmt.Errorf("record rename session event: %w", err)
	}
	return nil
}

// nameFrozenTx reports whether the row's name is the user's (name_source
// 'user'), which a harness-chosen name never replaces.
func nameFrozenTx(ctx context.Context, tx *sql.Tx, sessionID string) (bool, error) {
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT name_source FROM sessions WHERE id = ?`, sessionID).Scan(&current); err != nil {
		return false, fmt.Errorf("read name source: %w", err)
	}
	return current == "user", nil
}

// renameSlug validates RenameSession's arguments and returns the slug the
// new name would derive.
func renameSlug(sessionID, newName string, at int64) (string, error) {
	if sessionID == "" || newName == "" {
		return "", errors.New("session id and new name are required")
	}
	slug := Slug(newName)
	if slug == "" {
		return "", fmt.Errorf("session name %q does not produce a usable slug", newName)
	}
	if at == 0 {
		return "", errors.New("event timestamp is required")
	}
	return slug, nil
}

// claimRenameNameTx is RenameSession's uniqueness pre-check inside its
// transaction: the name, then the slug, each reaping a tombstoned holder.
func claimRenameNameTx(ctx context.Context, tx *sql.Tx, sessionID, newName, slug string, at int64) error {
	// Mirrors CreateSession's own name-then-slug pre-check order and
	// reasoning (see the comment there): checking name equality first,
	// inside this same transaction, makes "already exists" deterministic
	// rather than an accident of SQLite's own UNIQUE-constraint-check order.
	//
	// SPEC.md §9.2 (R77): a name or slug held ONLY by a tombstoned row is
	// available again, and renaming onto it reaps that row -- in this same
	// transaction, via reapTombstonedHolderTx, never a second BeginTx
	// against the shared *sql.DB. sessionID is excluded from both lookups
	// so renaming a session onto its own current name/slug never finds
	// itself as a "holder", exactly as the two `AND id != ?` checks this
	// replaces already did. A live holder still refuses with the bare
	// "already exists"; an archived holder refuses too, but (task 007, R78)
	// with a message that names it and both routes out.
	if err := reapTombstonedHolderTx(ctx, tx, "name", newName, sessionID,
		fmt.Sprintf("session name %q already exists", newName),
		func(holderName string) error {
			return fmt.Errorf("the archived session %q holds this name; press U to unarchive it, or dd to delete it, to free the name", holderName)
		}, at); err != nil {
		return err
	}
	if err := reapTombstonedHolderTx(ctx, tx, "slug", slug, sessionID,
		fmt.Sprintf("session name %q collides with existing slug %q", newName, slug),
		func(holderName string) error {
			return fmt.Errorf("session name %q collides with the archived session %q's slug; press U to unarchive it, or dd to delete it, to free the slug", newName, holderName)
		}, at); err != nil {
		return err
	}
	return nil
}

// nameSourceFor maps a rename event's source to the sessions.name_source value
// it records: 'derived' and 'harness' pass through, everything else (the
// operator's own rename, whatever label it carries) is 'user'.
func nameSourceFor(source string) string {
	if source == "derived" || source == "harness" {
		return source
	}
	return "user"
}

// writeSessionNameTx writes only the name and name_source columns of one row (RenameSession
// leaves slug alone) and reports a UNIQUE clash or a missing row.
func writeSessionNameTx(ctx context.Context, tx *sql.Tx, sessionID, newName, nameSource string, adopted bool) error {
	// Note: only the `name` column is written here -- `slug` is deliberately
	// left out of this UPDATE, which is the entire mechanism by which the
	// live tmux session (named from the ORIGINAL slug) is left untouched.
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET name = ?, name_source = ?, name_adopted = ? WHERE id = ?`, newName, nameSource, adopted, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "sessions.name") || strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.name") {
			return fmt.Errorf("session name %q already exists", newName)
		}
		return fmt.Errorf("rename session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rename session: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// mutateSessionWithEvent runs a single targeted UPDATE against exactly one
// session row and records a matching event in the same transaction, so
// observers never see a changed row without its corresponding event. eventKind
// and payload describe the event; query/args describe the row mutation.
func (s *Store) mutateSessionWithEvent(ctx context.Context, sessionID, fieldName, eventKind, source, payload string, at int64, query string, args ...any) error {
	return s.mutateSessionWithEventAlso(ctx, sessionID, fieldName, eventKind, source, payload, at, nil, query, args...)
}

// mutateSessionWithEventAlso is mutateSessionWithEvent plus alsoSQL, extra
// statements (each bound to the session id alone) run in the same
// transaction once the row update has matched exactly one session, so they
// commit or roll back with it.
func (s *Store) mutateSessionWithEventAlso(ctx context.Context, sessionID, fieldName, eventKind, source, payload string, at int64, alsoSQL []string, query string, args ...any) error {
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set %s: %w", fieldName, err)
	}
	defer rollbackTx(tx)
	if err := updateOneSessionTx(ctx, tx, sessionID, fieldName, query, args); err != nil {
		return err
	}
	for _, extra := range alsoSQL {
		if _, err := tx.ExecContext(ctx, extra, sessionID); err != nil {
			return fmt.Errorf("set %s: %w", fieldName, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, sessionID, at, eventKind, source, payload); err != nil {
		return fmt.Errorf("record set %s event: %w", fieldName, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set %s: %w", fieldName, err)
	}
	return nil
}

// updateOneSessionTx runs query (args, then the session id) and requires it to
// have matched exactly one session row.
func updateOneSessionTx(ctx context.Context, tx *sql.Tx, sessionID, fieldName, query string, args []any) error {
	execArgs := append(append([]any{}, args...), sessionID)
	result, err := tx.ExecContext(ctx, query, execArgs...)
	if err != nil {
		return fmt.Errorf("set %s: %w", fieldName, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check set %s: %w", fieldName, err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return nil
}

// ListSessions returns all durable rows, including stopped rows, but
// excluding tombstoned rows (deleted_at != 0, task 104) and archived rows
// (archived_at != 0, task 111, requirement 27) -- a soft-deleted session is
// never resurrected into the default view by ListSessions itself; only
// RestoreSession (clearing deleted_at) does that, and an archived row
// likewise only returns through UnarchiveSession (clearing archived_at:
// SPEC.md:323-332 makes BOTH flags reversible, `U` for archived_at
// exactly as `u` undoes a delete), which requirement 33's `/` filter
// (task 123) is how the row is reached in order to press. The stable
// order avoids hiding resumable sessions and keeps independently
// connected clients' views deterministic.
func (s *Store) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+`
		`+sessionsFromClause+` WHERE deleted_at = 0 AND archived_at = 0 ORDER BY created_at, sessions.id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer closeRows(rows)
	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}

// ListDeletedSessions is the separate, explicit accessor for tombstoned rows
// (deleted_at != 0) that ListSessions itself never returns. Task 106's
// reaper is its only intended caller: it polls this list to find rows
// whose DeletedAt plus DECK_DELETE_GRACE_MS has elapsed and calls
// ReapSession on each. Ordered the same way ListSessions is, for the same
// determinism-under-a-frozen-clock reason.
func (s *Store) ListDeletedSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+`
		`+sessionsFromClause+` WHERE deleted_at != 0 ORDER BY created_at, sessions.id`)
	if err != nil {
		return nil, fmt.Errorf("list deleted sessions: %w", err)
	}
	defer closeRows(rows)
	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deleted session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deleted sessions: %w", err)
	}
	return sessions, nil
}

// ListArchivedSessions is the separate, explicit accessor for archived rows
// (archived_at != 0, deleted_at == 0) that ListSessions itself never
// returns (requirement 27/33, task 123). Unlike ListDeletedSessions there
// is no reaper polling this list: the `/` filter's "archived" term
// (internal/tui) is its only intended caller, and it is the only way an
// archived row is FOUND again -- not the only way it comes back.
// SPEC.md:323-332 makes archived_at as reversible as deleted_at is:
// UnarchiveSession clears it (the `U` key) exactly as RestoreSession
// undoes deleted_at, so the route back is find-it-through-this-list and
// then unarchive it. Ordered the same way ListSessions is, for the same
// determinism-under-a-frozen-clock reason.
func (s *Store) ListArchivedSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+`
		`+sessionsFromClause+` WHERE deleted_at = 0 AND archived_at != 0 ORDER BY created_at, sessions.id`)
	if err != nil {
		return nil, fmt.Errorf("list archived sessions: %w", err)
	}
	defer closeRows(rows)
	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived sessions: %w", err)
	}
	return sessions, nil
}

// ListSessionsIncludingArchived returns every row deck still retains --
// archived rows (archived_at != 0) INCLUDED, tombstoned rows
// (deleted_at != 0) excluded. It exists because ListSessions' extra
// `archived_at = 0` term is a DISPLAY rule (requirement 27: the sidebar
// hides archived rows), and a consumer that needs "every row deck still
// owns" must not inherit it. Hook resolution (internal/hookrecv,
// SPEC §8.1) is that consumer: both keys it resolves by -- the payload's
// conversation id and the row id injected into the pane environment --
// name a row that exists, so resolving them through the sidebar's query
// turned every hook from an archived row into an orphan event with
// session_id NULL (issue #8) while the agent's own transcript printed a
// resolution failure the user could not act on. The tombstone half of
// the WHERE clause stays: a reaped-pending row is deliberately
// unresolvable, and a hook naming one is still an orphan. Ordered the
// same way ListSessions is, for the same determinism reason.
func (s *Store) ListSessionsIncludingArchived(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+`
		`+sessionsFromClause+` WHERE deleted_at = 0 ORDER BY created_at, sessions.id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions including archived: %w", err)
	}
	defer closeRows(rows)
	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan retained session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retained sessions: %w", err)
	}
	return sessions, nil
}

// SoftDeleteSession tombstones a session (task 104/105's `dd`): it records
// deleted_at rather than removing the row, so RestoreSession can undo it
// within the SPEC delete-grace window (task 106) and so a tombstoned row's
// events remain queryable for as long as the row itself survives. It
// never touches a live pane; callers that must kill one first (SPEC's
// `dd` contract) do so as a separate, explicit step before calling this.
func (s *Store) SoftDeleteSession(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "deleted_at", "deleted", "user", "", at,
		`UPDATE sessions SET deleted_at = ? WHERE id = ?`, at)
}

// RestoreSession clears a tombstone set by SoftDeleteSession, returning the
// row to ListSessions' default view. It is the store half of the grace-window
// `u` undo (task 106); it never touches a live pane (SPEC's undo relaunches
// or resumes the session through the normal launch path, a separate step).
//
// R77's honesty requirement (SPEC §9.2): if the row is gone because a
// CreateSession/RenameSession reused its name while the undo toast was
// still up (reapTombstonedHolderTx/reapSessionTx, above), a bare
// mutateSessionWithEvent "not found" would be true but misleading -- the
// operator pressed `u` on what looked like a live undo, not a session
// that had never existed. reapSessionTx always leaves a surviving orphan
// 'reaped' event (session_id NULL, payload = the reaped id) *before* the
// row itself is gone, so this checks for that event on the not-found path
// and reports the real cause instead. It is a read, never a second
// mutation, so it cannot itself race the same write lock R77 already
// had to avoid.
func (s *Store) RestoreSession(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	err := s.mutateSessionWithEvent(ctx, sessionID, "deleted_at", "restored", "user", "", at,
		`UPDATE sessions SET deleted_at = 0 WHERE id = ?`)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return err
	}
	var reaped int
	if checkErr := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id IS NULL AND kind = 'reaped' AND payload = ?`, sessionID).Scan(&reaped); checkErr != nil {
		return fmt.Errorf("%w (also failed to check reap history: %w)", err, checkErr)
	}
	if reaped > 0 {
		return fmt.Errorf("session %q was reaped because its name was reused", sessionID)
	}
	return err
}

// ArchiveSession sets SPEC requirement 27's `A` retention flag (task 111):
// a durable timestamp, never a status. It never checks or requires the
// current Status column -- the "archiving requires stopped" rule
// (SPEC.md:284-286) is enforced by the caller (internal/service.Archive),
// which kills a non-stopped session first, exactly as a plain kill would,
// before this ever runs; this method itself would happily set the flag on
// any row, matching SoftDeleteSession's own no-status-opinion shape. An
// already-archived row may be archived again (idempotent, no error),
// mirroring how a second SoftDeleteSession on an already-tombstoned row is
// tolerated elsewhere in this file.
func (s *Store) ArchiveSession(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return s.mutateSessionWithEventAlso(ctx, sessionID, "archived_at", "archived", "user", "", at,
		[]string{`DELETE FROM bookmarks WHERE session_id = ?`},
		`UPDATE sessions SET archived_at = ? WHERE id = ?`, at)
}

// UnarchiveSession clears requirement 27's `A` retention flag again (the
// `U` key, SPEC.md:323-332): archived_at goes back to 0, so the row
// returns to ListSessions' default view and leaves ListArchivedSessions.
// It is the exact mirror of RestoreSession -- same mutateSessionWithEvent
// shape, one targeted UPDATE plus its own "unarchived" event in one
// transaction -- and, like RestoreSession, it never touches a live pane or
// the Status column: unarchiving only ever un-hides the row, and a row
// that was killed on its way into the archive stays stopped until `r`
// resumes it. SPEC's invariant needs both halves: a one-way archive flag
// plus a resumable archived row is what would put a live agent behind the
// filter with no way out, so resume refuses an archived row
// (service.Resume) and this is the key that clears the flag.
func (s *Store) UnarchiveSession(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	return s.mutateSessionWithEvent(ctx, sessionID, "archived_at", "unarchived", "user", "", at,
		`UPDATE sessions SET archived_at = 0 WHERE id = ?`)
}

// Event is one row of the append-only events table (SPEC's schema listing:
// audit trail, §12 search corpus, and notification source in one).
// SessionID is empty for an orphan event (RecordOrphanEvent: a hook
// payload that could not be resolved to any session, §8.1) or for a
// reap's own tombstone record (ReapSession, recorded after the session row
// it describes is gone). Reason and Payload are both nullable columns and
// read back as "" when unset, never as a Go zero-value surprise for a
// caller that never checked ok.
type Event struct {
	Seq       int64
	SessionID string
	At        int64
	Kind      string
	Reason    string
	Payload   string
}

// defaultEventLogLimit is ListEvents' own bound when a caller passes a
// non-positive limit -- the events table is append-only and can grow
// without bound over a session's lifetime, so the `E` event log view (task
// 124/I-9, SPEC requirement 32) this feeds is deliberately never
// "everything ever recorded".
const defaultEventLogLimit = 200

// ListEvents returns the most recent events across every session (and any
// orphan/reap rows with no session at all), newest first: ORDER BY at DESC
// breaks ties with seq DESC so two events sharing the same millisecond
// (routine under a frozen DECK_CLOCK) still order deterministically by
// their own insertion order rather than however SQLite happens to walk
// ties. limit caps how many rows come back; a non-positive limit falls
// back to defaultEventLogLimit rather than returning the entire table.
func (s *Store) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	atomic.AddInt64(&s.eventsListCalls, 1)
	if limit <= 0 {
		limit = defaultEventLogLimit
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, session_id, at, kind, reason, payload
		FROM events ORDER BY at DESC, seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer closeRows(rows)
	var events []Event
	for rows.Next() {
		var event Event
		var sessionID, reason, payload sql.NullString
		if err := rows.Scan(&event.Seq, &sessionID, &event.At, &event.Kind, &reason, &payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.SessionID = sessionID.String
		event.Reason = reason.String
		event.Payload = payload.String
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

// droppedHookKindSuffix is hookrecv's own supersededEventKind suffix
// ("<baseKind>.superseded") duplicated here as a literal rather than an
// import: internal/store cannot depend on internal/hookrecv (the store is
// the lower-level package hookrecv itself depends on), and the LIKE clause
// below needs the literal either way. R90/task 033: this is the one place
// that reads the vocabulary task 032's supersededEventKind writes, so the
// `i` detail dialog can say "a hook was declined here, and why" without a
// caller trawling ListEvents itself.
const droppedHookKindSuffix = ".superseded"

// droppedHookNoGenerationReason is the prefix of hookrecv's supersededReason
// for a hook whose pane carried no launch generation at all (duplicated as a
// literal for the same layering reason as droppedHookKindSuffix; hookrecv's
// own test pins the two to each other).
const droppedHookNoGenerationReason = "declined: hook carries no launch generation"

// LastAlarmingDroppedHook returns the most recent event recorded for
// sessionID that is a declined hook worth an alarm in the `i` detail dialog
// (R200): a hook write supersededLaunch (internal/hookrecv) declined for
// this row, EXCEPT the expected teardown of a replaced launch. Not alarming:
// a "session_end.superseded" whose hook named a launch generation (the
// replaced pane's own SessionEnd, which arrives on essentially every
// restart/resume), and a "session_end.superseded" whose hook carried no
// generation but which a restart (kind "restart") or a launch lease
// acquisition (kind "launch_lease_acquired", written by every restart and
// resume) preceded on the same row. Always alarming: any other base kind
// (stop, notification, ...) from a replaced generation, and a generation-less
// session_end with no restart/resume before it. The raw events are never
// altered: ListEvents (`E`) still lists every one of them.
// found is false, with a zero Event and nil error, when there is none; that
// is the ordinary case and is not an error. Ordered the same way ListEvents
// is (at DESC, seq DESC) so two declines sharing a millisecond still resolve
// to the actually-newest one.
func (s *Store) LastAlarmingDroppedHook(ctx context.Context, sessionID string) (Event, bool, error) {
	if sessionID == "" {
		return Event{}, false, errors.New("session id is required")
	}
	row := s.db.QueryRowContext(ctx, `SELECT e.seq, e.session_id, e.at, e.kind, e.reason, e.payload
		FROM events e
		WHERE e.session_id = ? AND e.kind LIKE '%' || ?
		AND NOT (e.kind = 'session_end' || ?
			AND (COALESCE(e.reason, '') NOT LIKE ? || '%'
				OR EXISTS (SELECT 1 FROM events p WHERE p.session_id = e.session_id AND p.seq < e.seq
					AND p.kind IN ('restart', 'launch_lease_acquired'))))
		ORDER BY e.at DESC, e.seq DESC LIMIT 1`,
		sessionID, droppedHookKindSuffix, droppedHookKindSuffix, droppedHookNoGenerationReason)
	var event Event
	var sqlSessionID, reason, payload sql.NullString
	if err := row.Scan(&event.Seq, &sqlSessionID, &event.At, &event.Kind, &reason, &payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Event{}, false, nil
		}
		return Event{}, false, fmt.Errorf("last dropped hook for session %q: %w", sessionID, err)
	}
	event.SessionID = sqlSessionID.String
	event.Reason = reason.String
	event.Payload = payload.String
	return event, true, nil
}

// ListEventsCallCount reports how many times ListEvents has actually run
// against this store (R61, steer 3e-001 §6.3): test-only instrumentation
// used to prove a caller (the `E` event log) reads the store once per
// open rather than once per rendered frame.
func (s *Store) ListEventsCallCount() int64 {
	return atomic.LoadInt64(&s.eventsListCalls)
}

// eventRetentionMinInterval is how often EnforceEventRetention actually
// performs a deletion pass, per SPEC §6.5/§12 and steer 3e-001 §6.4/§7: "on
// store open and thereafter at most once an hour". The throttle itself is
// persisted in ui_state (not held only in memory) so that deck's many
// short-lived hook-command invocations of store.Open, each a fresh process,
// do not each re-run a full deletion pass; only the first EnforceEventRetention
// call after each real hour boundary does real work.
const eventRetentionMinInterval = time.Hour

// eventRetentionLastRunKey is the ui_state key EnforceEventRetention uses to
// remember when it last actually ran a deletion pass (as opposed to being
// called and throttled away). Absent key means "never run", so the very
// first call --  on store open, per the contract above -- always runs.
const eventRetentionLastRunKey = "event_retention_last_run_at"

// eventRetentionBatchRows bounds how many rows a single EnforceEventRetention
// batch deletes; eventRetentionMaxBatches bounds how many batches one call
// performs. Together they cap the synchronous work one call can do (SPEC's
// "bounded batches", never an unbounded DELETE nor an automatic VACUUM) even
// if the retention window has been widened, or deck was not run for a long
// time, leaving a very large backlog. Any remainder is left for the next
// hourly pass rather than pushing the caller's latency past this bound.
const (
	eventRetentionBatchRows  = 500
	eventRetentionMaxBatches = 200
)

// EnforceEventRetention deletes events older than retentionDays, oldest
// first, in bounded batches, per SPEC §6.5/§12 and steer 3e-001 §6.4/§7 (R62).
// now is the caller's current time in the same UnixMilli units as events.at
// and sessions.last_probe_at elsewhere in this file. It is a floor on
// history, not a cap on row count: a burst of events inside the retention
// window is kept whole however large it is. This performs an actual deletion
// pass only on the first call ever (store open, no prior ui_state row) and
// thereafter at most once per eventRetentionMinInterval; every call in
// between is a cheap no-op single SELECT against ui_state. No VACUUM is ever
// run here or anywhere else in this file.
func (s *Store) EnforceEventRetention(ctx context.Context, retentionDays int, now int64) error {
	if retentionDays <= 0 {
		return errors.New("event retention window must be positive")
	}
	if now <= 0 {
		return errors.New("event retention timestamp is required")
	}
	throttled, err := s.throttleActive(ctx, eventRetentionLastRunKey, now, eventRetentionMinInterval)
	if err != nil {
		return fmt.Errorf("read event retention throttle: %w", err)
	}
	if throttled {
		return nil
	}
	cutoff := now - int64(retentionDays)*24*time.Hour.Milliseconds()
	if err := s.deleteRetiredEvents(ctx, cutoff); err != nil {
		return err
	}
	if err := s.setUIState(ctx, eventRetentionLastRunKey, strconv.FormatInt(now, 10)); err != nil {
		return fmt.Errorf("write event retention throttle: %w", err)
	}
	return nil
}

// throttleActive reports whether the pass recorded under the ui_state key
// lastRunKey ran less than interval before now. A missing or unparsable
// stamp counts as "never run" and is never throttled.
func (s *Store) throttleActive(ctx context.Context, lastRunKey string, now int64, interval time.Duration) (bool, error) {
	lastRunRaw, err := s.getUIState(ctx, lastRunKey, "0")
	if err != nil {
		return false, err
	}
	lastRun, err := strconv.ParseInt(lastRunRaw, 10, 64)
	if err != nil {
		lastRun = 0
	}
	return lastRun != 0 && now-lastRun < interval.Milliseconds(), nil
}

// deleteRetiredEvents deletes events older than cutoff, oldest first, in
// batches of eventRetentionBatchRows, at most eventRetentionMaxBatches of
// them.
func (s *Store) deleteRetiredEvents(ctx context.Context, cutoff int64) error {
	for batch := 0; batch < eventRetentionMaxBatches; batch++ {
		result, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE seq IN (
			SELECT seq FROM events WHERE at < ? ORDER BY at ASC, seq ASC LIMIT ?)`,
			cutoff, eventRetentionBatchRows)
		if err != nil {
			return fmt.Errorf("delete retired events: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check retired events deletion: %w", err)
		}
		if affected < int64(eventRetentionBatchRows) {
			break
		}
	}
	return nil
}

// tombstoneSweepMinInterval is how often SweepTombstones actually performs
// a reap pass, mirroring eventRetentionMinInterval exactly (task 010, SPEC
// §6.5/§12's "on store open and thereafter at most once an hour"): the
// throttle is persisted in ui_state, not held only in memory, for the same
// reason -- deck's many short-lived hook-command invocations of store.Open,
// each a fresh process, must not each re-run a full sweep.
const tombstoneSweepMinInterval = time.Hour

// tombstoneSweepLastRunKey is the ui_state key SweepTombstones uses to
// remember when it last actually ran a sweep pass. Absent key means
// "never run", so the very first call -- on store open, per the contract
// above -- always runs.
const tombstoneSweepLastRunKey = "tombstone_sweep_last_run_at"

// tombstoneSweepBatchRows bounds how many tombstoned rows a single
// SweepTombstones call reaps. One call performs exactly ONE batch and
// returns -- deliberately unlike EnforceEventRetention's inner batch loop
// -- because this sweep's first caller runs on store open, before the
// model exists and before the first frame is drawn: one batch is the whole
// bound on how long it can hold that path up, however large the backlog is
// (deck not run for a long time, or many abandoned dd's with no tea.Tick
// left to reap them, task 011). Any remainder is left for the next pass
// rather than pushing the caller's latency past this bound.
const tombstoneSweepBatchRows = 200

// SweepTombstones reaps every session row whose deleted_at is older than
// deleteGrace, oldest first, in bounded batches, modelled directly on
// EnforceEventRetention above (task 010, steer 3e-001 §6.4/§7's "on store
// open and thereafter at most once an hour" contract, applied here to the
// dd/undo tombstone window instead of the events table). now is the
// caller's current time in the same UnixMilli units as deleted_at
// elsewhere in this file -- it is the ONLY source of "now" this method
// ever consults; it never calls time.Now() itself, so a frozen-clock
// caller's store never reaps on wall-clock time by accident. This is a
// safety net for a row whose tea.Tick deleteGraceExpired never fires
// (process killed, crashed, or simply quit before the grace window
// elapsed) -- task 011 wires the real store-open call site; this method
// only implements the primitive and its throttle. It performs an actual
// reap pass only on the first call ever (store open, no prior ui_state
// row) and thereafter at most once per tombstoneSweepMinInterval; every
// call in between is a cheap no-op single SELECT against ui_state.
//
// A pass reaps at most tombstoneSweepBatchRows rows, oldest tombstone
// first, in a single transaction, and then returns: one batch never blocks
// the caller.
//
// The bool return (review finding 3, R79) reports whether, after this
// batch, at least one MORE row is still older than deleteGrace -- i.e.
// whether the backlog is bigger than one batch and a caller must call
// again to finish draining it. The hourly throttle stamp is deliberately
// written only once that bool comes back false: a backlog larger than one
// batch must not silently wait out a real hour, once per leftover batch,
// before it is safe to continue -- so as long as a pass leaves rows
// behind, the NEXT call (whenever it comes, even moments later) is still
// treated as "never run" and performs its own batch immediately. Only
// once a pass actually catches the backlog up to cutoff does the throttle
// engage for a genuinely fresh hour. DrainExpiredTombstones below is the
// production continuation built on this; a caller that only wants the
// traditional single bounded batch (deck's own pre-first-frame store-open
// call) simply ignores the bool.
func (s *Store) SweepTombstones(ctx context.Context, deleteGrace time.Duration, now int64) (bool, error) {
	if deleteGrace < 0 {
		return false, errors.New("delete grace window must not be negative")
	}
	if now <= 0 {
		return false, errors.New("tombstone sweep timestamp is required")
	}
	throttled, err := s.throttleActive(ctx, tombstoneSweepLastRunKey, now, tombstoneSweepMinInterval)
	if err != nil {
		return false, fmt.Errorf("read tombstone sweep throttle: %w", err)
	}
	if throttled {
		return false, nil
	}
	more, err := s.sweepTombstoneBatch(ctx, now-deleteGrace.Milliseconds(), now)
	if err != nil {
		return false, err
	}
	if more {
		return true, nil
	}
	if err := s.setUIState(ctx, tombstoneSweepLastRunKey, strconv.FormatInt(now, 10)); err != nil {
		return false, fmt.Errorf("write tombstone sweep throttle: %w", err)
	}
	return false, nil
}

// sweepTombstoneBatch reaps one batch of tombstones older than cutoff and
// reports whether at least one more such row remains.
func (s *Store) sweepTombstoneBatch(ctx context.Context, cutoff, now int64) (bool, error) {
	ids, err := s.tombstonesOlderThan(ctx, cutoff, tombstoneSweepBatchRows)
	if err != nil {
		return false, fmt.Errorf("list expired tombstones: %w", err)
	}
	if len(ids) > 0 {
		if err := s.reapTombstoneBatch(ctx, ids, now); err != nil {
			return false, err
		}
	}
	remaining, err := s.tombstonesOlderThan(ctx, cutoff, 1)
	if err != nil {
		return false, fmt.Errorf("check remaining expired tombstones: %w", err)
	}
	return len(remaining) > 0, nil
}

// DrainExpiredTombstones is the continuation SweepTombstones' own doc
// promises (review finding 3, R79): it calls SweepTombstones repeatedly,
// driven ENTIRELY by its reported remainder -- never by a count this
// function or its caller computed -- until the whole backlog older than
// deleteGrace is gone or the hourly throttle genuinely has nothing left
// to do. Each individual call still reaps at most one
// tombstoneSweepBatchRows batch; this only chains those calls back to
// back within the SAME open/startup cycle, so a backlog bigger than one
// batch no longer needs an extra real hour per leftover batch to finish
// draining. now is forwarded unchanged to every call in the chain -- this
// never calls time.Now() itself. cmd/deck's tick caller (the
// hourly-throttled tuiReconcile closure, which already runs every
// settings.Reconcile tick) is the intended production caller: deck's
// pre-first-frame store-open call keeps calling plain SweepTombstones
// instead, so that call stays bounded to exactly one batch and this drain
// happens moments later, still as part of the same startup, once the
// first tick fires.
func (s *Store) DrainExpiredTombstones(ctx context.Context, deleteGrace time.Duration, now int64) error {
	for {
		more, err := s.SweepTombstones(ctx, deleteGrace, now)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// tombstonesOlderThan returns up to limit ids of rows tombstoned strictly
// before cutoff (deleted_at != 0 AND deleted_at < cutoff), oldest first --
// SweepTombstones' own read half, kept separate from the reap so the pass
// above stays readable. The ORDER BY is what makes a multi-pass drain
// deterministic: the longest-expired tombstones always go first, so no row
// can be starved by later arrivals.
func (s *Store) tombstonesOlderThan(ctx context.Context, cutoff int64, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM sessions WHERE deleted_at != 0 AND deleted_at < ? ORDER BY deleted_at ASC, id ASC LIMIT ?`,
		cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// reapTombstoneBatch reaps every id in ids inside a single transaction,
// via the same reapSessionTx CreateSession's own tombstoned-holder reap
// (R77) and ReapSession use, so a sweep batch never opens more than one
// transaction against the shared *sql.DB at a time.
func (s *Store) reapTombstoneBatch(ctx context.Context, ids []string, at int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tombstone sweep batch: %w", err)
	}
	defer rollbackTx(tx)
	for _, id := range ids {
		if err := reapSessionTx(ctx, tx, id, at); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tombstone sweep batch: %w", err)
	}
	return nil
}

// reapSessionTx performs the actual tombstone removal -- the orphan
// 'reaped' event (session_id NULL, task 019's audit-log convention for
// events that must outlive the row they describe) recorded BEFORE the
// DELETE, then the DELETE itself, which cascades away every row's own
// events (ON DELETE CASCADE, schemaV1) -- against an ALREADY-OPEN
// transaction. It does not begin or commit anything: both ReapSession and
// CreateSession's own tombstoned-holder reap (R77, SPEC.md §9.2) call
// this so a caller never opens a second *sql.Tx against the shared
// *sql.DB while the first is still open, which would block forever on
// SQLite's single writer lock rather than the deadlock ever surfacing as
// an error. The JSONL audit log, not this events table, is where task 107
// keeps a reaped session's earlier history durably past the reap.
func reapSessionTx(ctx context.Context, tx *sql.Tx, sessionID string, at int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (NULL, ?, 'reaped', 'user', ?)`, at, sessionID); err != nil {
		return fmt.Errorf("record reap event for session %q: %w", sessionID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("reap session %q: %w", sessionID, err)
	}
	return nil
}

// reapTombstonedHolderTx is CreateSession's (and, task 004, RenameSession's)
// half of R77: it looks up whatever row currently holds `value` in `column`
// ("name" or "slug", always a literal supplied by this package, never
// caller/user input, so the fmt.Sprintf below never carries untrusted SQL)
// inside the caller's own transaction. excludeID, when non-empty, is
// excluded from the lookup -- RenameSession's own row must never be found
// as its own "holder" just because the new name/slug happens to already be
// its current one; CreateSession has no existing row yet and passes "",
// under which the exclusion is a no-op since no session ever has an empty
// id. No holder: nil, proceed. A LIVE holder (deleted_at == 0, archived_at
// == 0) is refused with conflictMsg, unchanged from before this task. An
// ARCHIVED holder (deleted_at == 0, archived_at != 0 -- R78 keeps an
// archived row's name reserved) is refused too, but with a DIFFERENT
// message: archivedMsg is handed the holder's own current name (never
// `value`, which for the slug-column call is the slug, not a name) and
// builds a message that names that holder and both routes out, `U` to
// unarchive it or `dd` to delete it -- the bare "already exists" a live
// holder still gets tells the user nothing, since no list shows an
// archived row (R78, SPEC.md §9.2). A TOMBSTONED holder is reaped via
// reapSessionTx, in the same transaction as the caller's own INSERT/UPDATE,
// so the row is gone by the time that statement runs and never trips its
// UNIQUE constraint.
func reapTombstonedHolderTx(ctx context.Context, tx *sql.Tx, column, value, excludeID, conflictMsg string, archivedMsg func(holderName string) error, at int64) error {
	var id, holderName string
	var deletedAt, archivedAt int64
	err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT id, name, deleted_at, archived_at FROM sessions WHERE %s = ? AND id != ?`, column), value, excludeID).Scan(&id, &holderName, &deletedAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check session %s %q: %w", column, value, err)
	}
	if deletedAt == 0 {
		if archivedAt != 0 {
			return archivedMsg(holderName)
		}
		return errors.New(conflictMsg)
	}
	return reapSessionTx(ctx, tx, id, at)
}

// TombstonedNameHolders reports the ids of the tombstoned rows that hold
// `name` or its §3.2 slug -- exactly the rows that a CreateSession (and,
// task 004, a RenameSession) taking that name reaps inside its own
// transaction via reapTombstonedHolderTx (R77, SPEC.md §9.2). It reaps
// nothing itself and writes nothing: it exists so the SERVICE layer can
// learn, BEFORE the create runs, whose deck-owned per-session FILES (the
// captures dir and §9.4 history file) that create is about to orphan, and
// then remove them AFTER the SQL commit. Reading the ids up front rather
// than deleting files up front is the whole point: a create that is
// refused (a live or archived holder) or that fails for any other reason
// must leave a still-restorable session's scrollback exactly as it was.
func (s *Store) TombstonedNameHolders(ctx context.Context, name string) ([]string, error) {
	if name == "" {
		return nil, errors.New("session name is required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM sessions WHERE deleted_at != 0 AND (name = ? OR slug = ?)`, name, Slug(name))
	if err != nil {
		return nil, fmt.Errorf("list tombstoned holders of name %q: %w", name, err)
	}
	defer closeRows(rows)
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tombstoned holder of name %q: %w", name, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tombstoned holders of name %q: %w", name, err)
	}
	return ids, nil
}

// SessionRowExists reports whether a row with this id is still present at
// all, tombstoned or not. The service layer uses it to confirm, after a
// create committed, that a holder it saw tombstoned beforehand really was
// reaped by that create before removing that holder's files -- so a row
// that was restored in between (its name freed some other way) never has
// its scrollback deleted underneath it.
func (s *Store) SessionRowExists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.New("session id is required")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id = ?`, id).Scan(&count); err != nil {
		return false, fmt.Errorf("check session %q exists: %w", id, err)
	}
	return count > 0, nil
}

// ReapSession permanently removes a tombstoned session once its grace
// window (task 106) has elapsed. Only a row SoftDeleteSession has already
// tombstoned may be reaped; reaping a live row is refused so a caller
// cannot skip the tombstone/undo window by mistake. The actual removal is
// reapSessionTx, shared with CreateSession's own tombstoned-holder reap
// (R77) so neither caller ever opens a second transaction against the
// same *sql.DB while one is already open.
func (s *Store) ReapSession(ctx context.Context, sessionID string, at int64) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reap session: %w", err)
	}
	defer rollbackTx(tx)
	if err := requireTombstonedTx(ctx, tx, sessionID); err != nil {
		return err
	}
	if err := reapSessionTx(ctx, tx, sessionID, at); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reap session %q: %w", sessionID, err)
	}
	return nil
}

// requireTombstonedTx refuses a session that does not exist or is not
// tombstoned, so ReapSession never removes a live row.
func requireTombstonedTx(ctx context.Context, tx *sql.Tx, sessionID string) error {
	var deletedAt int64
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM sessions WHERE id = ?`, sessionID).Scan(&deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session %q not found", sessionID)
		}
		return fmt.Errorf("read session %q for reap: %w", sessionID, err)
	}
	if deletedAt == 0 {
		return fmt.Errorf("session %q is not tombstoned, refusing to reap", sessionID)
	}
	return nil
}

func (s *Store) version() (int, error) {
	var version int
	err := s.db.QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	// A missing meta table is the supported pre-v1 fixture format.
	if err != nil && containsNoTable(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read state database schema version: %w", err)
	}
	return version, nil
}

func containsNoTable(err error) bool {
	return err != nil && (contains(err.Error(), "no such table: meta") || contains(err.Error(), "no such table: META"))
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func (s *Store) migrate(version int) error {
	if version == SchemaVersion {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin state database migration: %w", err)
	}
	defer rollbackTx(tx)
	if err := applySchemaLadder(tx, version); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', ?) ON CONFLICT(key) DO UPDATE SET version = excluded.version`, SchemaVersion); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state database migration: %w", err)
	}
	return nil
}

// applySchemaLadder runs every migration rung above the on-disk version,
// oldest first, inside the caller's transaction. The rungs are read from
// the schemaVN variables at call time (tests substitute them); a version
// outside 0..SchemaVersion-1 has no migration path.
func applySchemaLadder(tx *sql.Tx, version int) error {
	rungs := [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21}
	if version < 0 || version >= len(rungs) {
		return fmt.Errorf("no migration path from schema version %d", version)
	}
	for i := version; i < len(rungs); i++ {
		for _, statement := range rungs[i] {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("create schema v%d: %w", i+1, err)
			}
		}
	}
	return nil
}

var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, slug TEXT NOT NULL UNIQUE,
		cwd TEXT NOT NULL, agent TEXT NOT NULL, launch_args TEXT NOT NULL DEFAULT '[]',
		env TEXT NOT NULL DEFAULT '{}', env_dirty INTEGER NOT NULL DEFAULT 0,
		captured_path TEXT NOT NULL, pre_launch TEXT, login_shell INTEGER NOT NULL DEFAULT 0,
		permission_profile TEXT NOT NULL DEFAULT 'safe', permission_profile_reason TEXT, conversation_id TEXT, resume_pin TEXT,
		resume_state TEXT NOT NULL DEFAULT 'auto', status TEXT NOT NULL, status_reason TEXT,
		status_source TEXT NOT NULL, status_at INTEGER NOT NULL, killed_by_user INTEGER NOT NULL DEFAULT 0,
		pane_exit_status INTEGER, crash_tail TEXT, notify_epoch INTEGER NOT NULL DEFAULT 0,
		last_message TEXT, sensitive INTEGER NOT NULL DEFAULT 0, notify_rules TEXT,
		important INTEGER NOT NULL DEFAULT 0, workspace TEXT, snoozed_until INTEGER NOT NULL DEFAULT 0,
		acknowledged INTEGER NOT NULL DEFAULT 1, launch_lease_owner TEXT, launch_lease_until INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL, last_attached_at INTEGER NOT NULL DEFAULT 0,
		archived_at INTEGER NOT NULL DEFAULT 0, deleted_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
		at INTEGER NOT NULL, kind TEXT NOT NULL, reason TEXT, payload TEXT
	)`,
}

// schemaV2 adds ui_state (SPEC §11.2, §6.5): machine-local UI state
// (layout_mode, sidebar_width) that never lives in config.toml. It is
// applied on top of schemaV1 for a fresh database and standalone for an
// existing v1 database, and never touches the sessions table — an upgrade
// from v1 to v2 recreates no session row.
var schemaV2 = []string{
	`CREATE TABLE IF NOT EXISTS ui_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
}

// schemaV3 adds sessions.last_probe_at (task 009): the wall-clock millisecond
// timestamp of the most recent stale-eligible pane sample that matched no
// probe.go rule, recorded by RecordProbeMiss. It is a pure diagnostic column
// for the `i` detail dialog and is never read by §7 precedence or by any
// status/status_source transition.
var schemaV3 = []string{
	`ALTER TABLE sessions ADD COLUMN last_probe_at INTEGER NOT NULL DEFAULT 0`,
}

// schemaV4 adds recent_cwds (task 006, SPEC §4/§11.7: the create modal's
// directory history). used_seq is monotonic, never a clock value, so
// ordering stays assertable under a frozen DECK_CLOCK (SPEC §13.1). It is
// applied on top of schemaV1-3 for a fresh database and standalone for an
// existing v1/v2/v3 database, and never touches the sessions table -- an
// upgrade to v4 recreates no session row.
var schemaV4 = []string{
	`CREATE TABLE IF NOT EXISTS recent_cwds (path TEXT PRIMARY KEY, used_seq INTEGER NOT NULL)`,
}

// schemaV5 adds the two events indexes from SPEC.md's amended events DDL
// (steer 3e-001 / task 330): events_at backs ListEvents' newest-first read
// (§12), events_session_kind backs §6.4's env-apply reads. Index-only; it
// touches no existing row and is applied on top of schemaV1-4 for a fresh
// database and standalone for an existing v1-v4 database.
var schemaV5 = []string{
	`CREATE INDEX IF NOT EXISTS events_at ON events(at DESC, seq DESC)`,
	`CREATE INDEX IF NOT EXISTS events_session_kind ON events(session_id, kind)`,
}

// schemaV6 adds sessions.post_destroy (task 010, the per-session teardown
// hook command SPEC §6.4 documents alongside pre_launch) and
// sessions.launch_dirty (task 010's own launch-side counterpart to the
// existing env_dirty column: whether a launch-affecting field has changed
// since the pane last launched). Both are ALTER TABLE ADD COLUMN against
// the existing sessions table -- neither touches sessions' PRIMARY KEY,
// UNIQUE constraints, or any existing row's other columns -- applied on
// top of schemaV1-5 for a fresh database and standalone for an existing
// v1-v5 database. post_destroy defaults to NULL (unset, mirroring
// pre_launch's own nullable TEXT shape: no hook configured is the common
// case); launch_dirty defaults to 0 (not dirty), matching env_dirty's own
// INTEGER NOT NULL DEFAULT 0 shape in schemaV1.
var schemaV6 = []string{
	`ALTER TABLE sessions ADD COLUMN post_destroy TEXT`,
	`ALTER TABLE sessions ADD COLUMN launch_dirty INTEGER NOT NULL DEFAULT 0`,
}

// schemaV7 lands R128's manual session groups (SPEC §4/§11) in one
// migration: the groups table (name unique, case-insensitively -- "a
// rename carries membership" and a name collision must be caught
// case-insensitively so "Infra" cannot coexist with "infra"), the
// sessions.group_id column that carries membership, and the drop of
// sessions.workspace, the column group_id replaces. It is applied on top
// of schemaV1-6 for a fresh database and standalone for an existing
// v1-v6 database, inside migrate()'s single transaction: a fresh v6
// database's three statements below either all land or none do, and a
// failure partway (the atomicity test in schema_v7_migration_test.go)
// leaves the caller's v6 database exactly as it was, workspace column
// and all, because the whole migrate() call -- this schema bump
// included -- is one BEGIN/COMMIT, never per-statement commits.
//
// sessions.group_id deliberately carries NO foreign key at all -- not
// even the seemingly obvious REFERENCES groups(id) ON DELETE SET NULL.
// This store opens every connection with `PRAGMA foreign_keys=ON`
// (OpenPath above), so a real FK here would have SQLite itself null out
// group_id the instant a referenced groups row is deleted. But SPEC §11
// spells out the opposite behaviour on purpose: "a group_id that no
// longer resolves -- another client deleted it between this client's
// load and its render -- renders under default rather than vanishing"
// -- i.e. a dangling group_id is an EXPECTED transient state this
// store's own read path (scanSession's LEFT JOIN, internal/tui's group
// seam) is built to tolerate, not a corruption an FK should race to
// erase before the next reader gets a chance to notice it went stale.
// Group deletion (task 018/019, SPEC §11.5) is also already
// app-orchestrated: the settings dialog decides what happens to a
// group's members (move to default, or delete them via the batch dd
// path) BEFORE the group row itself is removed, in the same operation --
// so there is no moment in the intended, single-writer path where an
// ON DELETE SET NULL would ever have anything to do. Adding one anyway
// would only change behaviour for the one case SPEC explicitly wants
// handled at read time instead: two independent deck processes racing
// on the same state.db (task 020).
//
// groups.id is INTEGER PRIMARY KEY AUTOINCREMENT, not the plain
// INTEGER PRIMARY KEY every other rowid-aliased table in this schema
// uses: SQLite's ordinary rowid assignment (max(rowid)+1 among rows
// CURRENTLY in the table) freely reuses the id of a row that has since
// been deleted -- delete the one existing group, whose id happens to be
// the table's current max, create another, and the new row lands on the
// exact same id (proven by the id-reuse regression in
// group_id_reuse_test.go). Membership (sessions.group_id), the
// remembered create-modal group (ui_state's last_create_group) and the
// collapse set (ui_state's collapsed_groups) are all keyed on this id
// with no foreign key at all (see the comment below), so a reused id
// would silently rebind every one of those stale references onto
// whatever unrelated group next claims it, instead of the dangling
// reference degrading to default the way R128/R130 require. AUTOINCREMENT
// makes SQLite track the highest id a groups row has EVER held (via the
// hidden sqlite_sequence table) and never hand it out again, so a
// deleted group's identity is retired for good, matching sessions.id's
// own approach to durable identity (a TEXT UUID primary key, immune to
// this class of reuse by construction) for the one column here that
// cannot use a UUID and still cycle through the create modal's numeric
// picker.
var schemaV7 = []string{
	`CREATE TABLE IF NOT EXISTS groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE COLLATE NOCASE)`,
	`ALTER TABLE sessions ADD COLUMN group_id INTEGER`,
	`ALTER TABLE sessions DROP COLUMN workspace`,
}

// schemaV8 adds sessions.pinned_at (task 002, the sidebar pin `p`, phase
// 4f, GH #51): the wall-clock millisecond timestamp a session was pinned
// at, zero for a never-pinned row -- mirroring ArchivedAt/DeletedAt's own
// zero-means-unset INTEGER NOT NULL DEFAULT 0 shape rather than a
// nullable TEXT. It is a single ALTER TABLE ADD COLUMN against the
// existing sessions table, touching no other column and no other row
// value, applied on top of schemaV1-7 for a fresh database and
// standalone for an existing v1-v7 database.
var schemaV8 = []string{
	`ALTER TABLE sessions ADD COLUMN pinned_at INTEGER NOT NULL DEFAULT 0`,
}

// schemaV9 adds sessions.hook_executable (R204c, GH #56): the absolute path of
// the deck binary a session's agent was last launched with -- the executable
// its hook command (`<path> _hook`) is bound to for as long as that agent
// runs. NULL for a row no launch has recorded one for (every row created
// before this version), which reads back empty and means "unknown", never a
// stale binding. A single ALTER TABLE ADD COLUMN, touching no other column
// or row value.
var schemaV9 = []string{
	`ALTER TABLE sessions ADD COLUMN hook_executable TEXT`,
}

// schemaV10 is R228's event-hook schema (phase 4n). It retires the notify-rule
// and snooze columns that no code reads or writes (notify_rules and
// snoozed_until, both from schemaV1), drops an outbox table if a database ever
// carried one (no shipped schema creates it, hence IF EXISTS), and adds the
// three per-session event-hook columns SPEC §4 describes, all nullable so
// every existing row reads back "inherit the global setting, nothing fired":
// event_hook_enabled (tri-state: NULL inherit, 1 on, 0 off), event_hook_events
// (a JSON list replacing the global kind list, NULL inherits) and hook_fired
// (JSON (kind,reason) pairs already fired in the current notify_epoch,
// cleared whenever notify_epoch advances). No other column or row value is
// touched.
var schemaV10 = []string{
	`DROP TABLE IF EXISTS outbox`,
	`ALTER TABLE sessions DROP COLUMN notify_rules`,
	`ALTER TABLE sessions DROP COLUMN snoozed_until`,
	`ALTER TABLE sessions ADD COLUMN event_hook_enabled INTEGER`,
	`ALTER TABLE sessions ADD COLUMN event_hook_events TEXT`,
	`ALTER TABLE sessions ADD COLUMN hook_fired TEXT`,
}

// schemaV11 stores the event hook's result against the event it ran for
// (R232b, SPEC §10.3): the offered kind, the exit status, whether the bound
// expired, the capped output tail and the not-started error. All columns are
// nullable, so every existing event, and every event nothing was spawned for,
// reads back "no hook ran".
var schemaV11 = []string{
	`ALTER TABLE events ADD COLUMN hook_kind TEXT`,
	`ALTER TABLE events ADD COLUMN hook_exit INTEGER`,
	`ALTER TABLE events ADD COLUMN hook_timed_out INTEGER`,
	`ALTER TABLE events ADD COLUMN hook_output TEXT`,
	`ALTER TABLE events ADD COLUMN hook_error TEXT`,
}

// schemaV12 is R248's harness-name schema (phase 4q). name_source records who
// last wrote sessions.name: 'user' (typed in deck, and the default so every
// pre-existing row keeps a name nothing may overwrite), 'derived' (deck's own
// default from the cwd or first prompt) or 'harness' (adopted from the
// agent's own session title). pending_harness_name holds a harness title seen
// but not adopted, with pending_harness_name_at the millisecond it was seen;
// both are NULL for "nothing pending". harness_name_adopt is the per-session
// adoption override (tri-state: NULL inherit the global setting, 1 adopt,
// 0 never adopt). Every column is added by ALTER TABLE ADD COLUMN; no
// existing value is touched.
var schemaV12 = []string{
	`ALTER TABLE sessions ADD COLUMN name_source TEXT NOT NULL DEFAULT 'user' CHECK (name_source IN ('user', 'derived', 'harness'))`,
	`ALTER TABLE sessions ADD COLUMN pending_harness_name TEXT`,
	`ALTER TABLE sessions ADD COLUMN pending_harness_name_at INTEGER`,
	`ALTER TABLE sessions ADD COLUMN harness_name_adopt INTEGER CHECK (harness_name_adopt IS NULL OR harness_name_adopt IN (0, 1))`,
}

// schemaV13 is the origin of a session's name. name_adopted is 1 when the name
// came from the harness, whether adopted as name_source 'harness' or frozen as
// 'user' because a human chose it inside the harness. Launch and resume argv
// carry only a name typed in deck, so an adopted name stays data (R253). It is
// cleared by any rename typed in deck. An existing row keeps the origin schema
// 12 recorded: it is 1 when name_source is 'harness', or when the row's latest
// renamed event came from a harness ('harness' or 'harness:<kind>') and named
// the current name (an explicit harness name adopted or frozen as 'user'). Every
// other existing row reads 0.
var schemaV13 = []string{
	`ALTER TABLE sessions ADD COLUMN name_adopted INTEGER NOT NULL DEFAULT 0 CHECK (name_adopted IN (0, 1))`,
	`UPDATE sessions SET name_adopted = 1 WHERE name_source = 'harness' OR EXISTS (
		SELECT 1 FROM events e WHERE e.seq = (
			SELECT MAX(l.seq) FROM events l WHERE l.session_id = sessions.id AND l.kind = 'renamed')
		AND (e.reason = 'harness' OR e.reason LIKE 'harness:%') AND e.payload = sessions.name)`,
}

// schemaV14 adds bookmarks (R277): slot 0-9 mapped to a session id. The slot
// is the primary key, so one session holds a slot at most once over and a slot
// names one session; a session may hold several slots. Deleting a session
// removes its slots: the foreign key cascades when the row is reaped, and the
// trigger clears them when the row is tombstoned (deleted_at set), so a
// tombstoned session is never a jump target. Neither statement touches the
// sessions table's rows, so an upgrade from v13 recreates no session.
var schemaV14 = []string{
	`CREATE TABLE IF NOT EXISTS bookmarks (
		slot INTEGER PRIMARY KEY CHECK (slot BETWEEN 0 AND 9),
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS bookmarks_session ON bookmarks(session_id)`,
	`CREATE TRIGGER IF NOT EXISTS bookmarks_clear_on_tombstone AFTER UPDATE OF deleted_at ON sessions
		WHEN NEW.deleted_at != 0 BEGIN DELETE FROM bookmarks WHERE session_id = NEW.id; END`,
}

// schemaV15 adds sessions.last_output_at (R304): the wall-clock millisecond
// time of the newest transcript-region output reconcile has confirmed for the
// session, 0 until the first. It is a clock, not a status: it is written by a
// plain UPDATE (RecordOutput), never as an event row, and nothing in SPEC §7
// precedence reads it. ALTER TABLE ADD COLUMN touches no existing value, so an
// upgrade from v14 recreates no session.
var schemaV15 = []string{
	`ALTER TABLE sessions ADD COLUMN last_output_at INTEGER NOT NULL DEFAULT 0`,
}

// schemaV16 adds sessions.probe_miss_streak (R315): the number of consecutive
// stale-eligible probes that matched no rule, 0 until the first. It is a
// counter written by plain UPDATEs (RecordProbeMiss adds one; every verdict
// resets it), never an event, and nothing in SPEC §7 precedence reads it.
var schemaV16 = []string{
	`ALTER TABLE sessions ADD COLUMN probe_miss_streak INTEGER NOT NULL DEFAULT 0`,
}

// schemaV17 adds the events_session_at index (R315). R315's quiet window is
// "no event for 3 x stale_after", and status_at cannot measure that, because
// a repeat verdict records an event while leaving status_at alone (R303).
// Session.LastEventAt is therefore read as the newest events.at for the row,
// and this index makes that per-row MAX a single index probe. It adds no
// column and writes no row.
var schemaV17 = []string{
	`CREATE INDEX IF NOT EXISTS events_session_at ON events(session_id, at)`,
}

// schemaV18 is R318's seen schema (phase 4t). sessions.acknowledged (a bit)
// is dropped (the schemaV10 column-drop precedent) and sessions.seen_status_at
// added: the status_at stamp the user has seen, so a row's status is seen
// exactly when seen_status_at equals status_at. Backfill keeps what each row
// already said: an acknowledged row is seen at its current stamp, every other
// row is unseen (0).
var schemaV18 = []string{
	`ALTER TABLE sessions ADD COLUMN seen_status_at INTEGER NOT NULL DEFAULT 0`,
	`UPDATE sessions SET seen_status_at = CASE WHEN acknowledged = 1 THEN status_at ELSE 0 END`,
	`ALTER TABLE sessions DROP COLUMN acknowledged`,
}

// schemaV19 adds sessions.last_seen_at (R324): the last_output_at value the
// user has viewed, 0 until the first view. It is a clock mirror, not a wall
// time: a view copies last_output_at into it (MarkViewed), so the output-unseen
// test is a plain comparison that a stepped clock cannot upset. Backfill sets
// it to last_output_at, so an upgrade does not mark every existing session
// unseen at once.
var schemaV19 = []string{
	`ALTER TABLE sessions ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0`,
	`UPDATE sessions SET last_seen_at = last_output_at`,
}

// schemaV20 adds sessions.hook_at (R303, SPEC §7): the time of the newest hook
// verdict the store applied. The hook-over-sampling precedence reads it
// instead of status_at, because a hook repeating the current status must not
// move status_at (the transition stamp) yet is still fresh hook evidence.
// Backfill dates a hook-sourced row by its status_at, the only hook time a
// pre-v20 database recorded.
var schemaV20 = []string{
	`ALTER TABLE sessions ADD COLUMN hook_at INTEGER NOT NULL DEFAULT 0`,
	`UPDATE sessions SET hook_at = status_at WHERE status_source = 'hook'`,
}

// schemaV21 adds sessions.status_seq (R319, R321, R322): the number of status
// transitions the row has made. status_at is a wall-clock stamp and so not an
// identity (two episodes can share a millisecond, a stepped clock can repeat
// an old stamp); (status_at, status_seq) is, and every asynchronous seen write
// and cached engagement/dwell decision is bound to it. A repeat of the current
// status (R303) does not advance it, and seen_status_at == status_at stays the
// public seen predicate.
var schemaV21 = []string{
	`ALTER TABLE sessions ADD COLUMN status_seq INTEGER NOT NULL DEFAULT 0`,
}

// StatusEpisode names one status episode of a row: the status_at stamp and the
// transition count (status_seq) it was written at. Two episodes never share
// both, whatever the clock did.
type StatusEpisode struct {
	At, Seq int64
}

// Episode is the status episode the row is in now.
func (s Session) Episode() StatusEpisode {
	return StatusEpisode{At: s.StatusAt, Seq: s.StatusSeq}
}

// VerdictAt is the time the row's verdict was last vouched for, the anchor of
// every staleness decision (probe eligibility, the movement fallback, the
// store's hook-over-sampling guards): status_at, or the newer hook_at when the
// stored verdict is a hook's. A hook repeating the status therefore keeps its
// verdict fresh without ever dating the row (R303).
func (s Session) VerdictAt() int64 {
	if s.StatusSource == "hook" {
		return max(s.StatusAt, s.HookAt)
	}
	return s.StatusAt
}

// MarkViewed is the one durable write of a view (R324): it copies the row's
// last_output_at into last_seen_at, so output the user has now looked at stops
// counting as unseen. It writes nothing when the two already agree, which keeps
// a watched row from rewriting its row on every tick: the column changes only
// on a view change or when a new output bucket lands under a view. It changes
// no status, seen_status_at, status_at or event and never moves a row in any
// sort order (no order reads last_seen_at). It reports whether a row was
// written.
func (s *Store) MarkViewed(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, errors.New("session id is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_seen_at = last_output_at WHERE id = ? AND last_seen_at <> last_output_at`, sessionID)
	if err != nil {
		return false, fmt.Errorf("mark viewed: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read viewed session count: %w", err)
	}
	return changed > 0, nil
}

// Unseen reports whether the row carries the unseen marker (R324): output
// the user has not viewed (last_output_at > last_seen_at) or an attention
// status the user has not seen (waiting or error, and not StatusSeen per
// R319). A running row never shows the output half: its output is the
// expected state, and only a row that went quiet is worth a look.
func (s Session) Unseen() bool {
	if (s.Status == "waiting" || s.Status == "error") && !s.StatusSeen() {
		return true
	}
	return s.Status != "running" && s.LastOutputAt > s.LastSeenAt
}

// StatusSeen reports whether the user has seen the row's current status stamp
// (R318): equality of seen_status_at and status_at, never an ordering.
func (s Session) StatusSeen() bool { return s.SeenStatusAt == s.StatusAt }

// getUIState returns the persisted value for key, or def when no row exists
// yet. Per SPEC §11.2, ui_state is not load-bearing: a missing row degrades
// to the documented default rather than to an error.
func (s *Store) getUIState(ctx context.Context, key, def string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM ui_state WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return "", fmt.Errorf("read ui_state %s: %w", key, err)
	}
	return value, nil
}

func (s *Store) setUIState(ctx context.Context, key, value string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ui_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		return fmt.Errorf("write ui_state %s: %w", key, err)
	}
	return nil
}

// GetLayoutMode returns the persisted §11.2 layout_mode, defaulting to
// DefaultLayoutMode when nothing has been persisted yet.
func (s *Store) GetLayoutMode(ctx context.Context) (string, error) {
	return s.getUIState(ctx, "layout_mode", DefaultLayoutMode)
}

// SetLayoutMode persists the §11.2 layout_mode in state.db, never in
// config.toml.
func (s *Store) SetLayoutMode(ctx context.Context, mode string) error {
	return s.setUIState(ctx, "layout_mode", mode)
}

// GetSidebarWidth returns the persisted §11.2 sidebar_width, defaulting to
// DefaultSidebarWidth when nothing has been persisted yet or the stored
// value is not a valid integer (ui_state is not load-bearing).
func (s *Store) GetSidebarWidth(ctx context.Context) (int, error) {
	value, err := s.getUIState(ctx, "sidebar_width", strconv.Itoa(DefaultSidebarWidth))
	if err != nil {
		return 0, err
	}
	width, err := strconv.Atoi(value)
	if err != nil {
		return DefaultSidebarWidth, nil
	}
	return width, nil
}

// SetSidebarWidth persists the §11.2 sidebar_width in state.db, never in
// config.toml.
func (s *Store) SetSidebarWidth(ctx context.Context, width int) error {
	return s.setUIState(ctx, "sidebar_width", strconv.Itoa(width))
}

// lastCreateAgentUIStateKey is the ui_state key GetLastCreateAgent/
// SetLastCreateAgent use to persist SPEC.md:1364-1367's "a dialog opens on
// the choice the user last made" for the create modal's Agent field: the
// kind of the most recent session a create actually succeeded with, never
// a cycled-then-abandoned choice.
const lastCreateAgentUIStateKey = "last_create_agent"

// GetLastCreateAgent returns the persisted last-successfully-created agent
// kind, or "" when no create has ever succeeded (ui_state is not
// load-bearing: a missing row degrades to "", which the tui package treats
// as "nothing to remember" and falls back to its own built-in default --
// never a store-side default, since validating the value against the
// currently registered kinds is the caller's job, not this accessor's).
func (s *Store) GetLastCreateAgent(ctx context.Context) (string, error) {
	return s.getUIState(ctx, lastCreateAgentUIStateKey, "")
}

// SetLastCreateAgent persists the just-succeeded create modal's agent kind
// in state.db's ui_state table, never in config.toml, so a later create
// opens pre-selecting it.
func (s *Store) SetLastCreateAgent(ctx context.Context, agent string) error {
	return s.setUIState(ctx, lastCreateAgentUIStateKey, agent)
}

// collapsedGroupsUIStateKey is the ui_state key GetCollapsedGroups/
// SetCollapsedGroups use to persist SPEC §11's "collapse state persists in
// ui_state" for manual groups (R129): the set of group ids the user has
// toggled collapsed with `c` or a header click, so a group collapsed
// yesterday is still collapsed today.
const collapsedGroupsUIStateKey = "collapsed_groups"

// GetCollapsedGroups returns the persisted set of collapsed group ids as a
// membership map (collapsed[id] is true for every id in the set), or an
// empty, non-nil map when no row exists yet, or when the stored value is
// not valid JSON (ui_state is not load-bearing: nothing collapsed is the
// documented default). It does not filter out an id that no longer
// resolves to a real group -- SPEC §11 already renders a dangling group_id
// under default on the session side, and a stale collapsed-id entry for a
// deleted group is simply never looked up again, so there is nothing here
// that needs healing.
func (s *Store) GetCollapsedGroups(ctx context.Context) (map[int64]bool, error) {
	value, err := s.getUIState(ctx, collapsedGroupsUIStateKey, "")
	if err != nil {
		return nil, err
	}
	collapsed := make(map[int64]bool)
	if value == "" {
		return collapsed, nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return make(map[int64]bool), nil
	}
	for _, id := range ids {
		collapsed[id] = true
	}
	return collapsed, nil
}

// SetCollapsedGroups persists the full set of collapsed group ids in
// state.db's ui_state table, never in config.toml, replacing whatever set
// was there before. Only ids mapped to true are persisted, so callers may
// pass the same map they mutate with collapsed[id] = false to un-collapse.
func (s *Store) SetCollapsedGroups(ctx context.Context, collapsed map[int64]bool) error {
	ids := make([]int64, 0, len(collapsed))
	for id, on := range collapsed {
		if on {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	encoded, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("encode collapsed groups: %w", err)
	}
	return s.setUIState(ctx, collapsedGroupsUIStateKey, string(encoded))
}

// lastCreateGroupUIStateKey is the ui_state key GetLastCreateGroup/
// SetLastCreateGroup use to persist SPEC §11's "the create modal ... Group
// field ... default[s] to the last group created into" (R130), mirroring
// lastCreateAgentUIStateKey's own precedent for the Agent field.
const lastCreateGroupUIStateKey = "last_create_group"

// GetLastCreateGroup returns the id of the group most recently created
// into, or nil when the most recent create targeted the structural default
// group (see SetLastCreateGroup's note on groupID 0) or no create has ever
// targeted a real group at all (ui_state is
// not load-bearing: a missing or unparsable row degrades to nil, meaning
// default). It also degrades to nil when the remembered id no longer
// resolves to a real group -- SPEC §11's "a group_id that no longer
// resolves ... renders under default rather than vanishing" applies here
// too: the create modal must never pre-select a group that was deleted
// since the last create, so a dangling remembered id degrades to nil
// (default) rather than to an empty, still-distinguishable value.
func (s *Store) GetLastCreateGroup(ctx context.Context) (*int64, error) {
	value, err := s.getUIState(ctx, lastCreateGroupUIStateKey, "")
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, nil
	}
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM groups WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check last create group: %w", err)
	}
	return &id, nil
}

// SetLastCreateGroup persists the just-succeeded create modal's target
// group id in state.db's ui_state table, never in config.toml, so a later
// create opens pre-selecting it.
//
// groupID 0 is the structural default group (a session whose group_id is
// NULL), and it is worth remembering exactly like a named group: a create
// into default must make the NEXT create open on default, never on
// whatever named group happened to be remembered before it. It is recorded
// by clearing the row to the empty value, which GetLastCreateGroup already
// reads back as nil (== default), so "remembered default" and "nothing
// ever remembered" collapse into the single state the create modal already
// treats identically -- open on default, unlabelled -- instead of needing
// a third, separately-encoded value.
func (s *Store) SetLastCreateGroup(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return s.setUIState(ctx, lastCreateGroupUIStateKey, "")
	}
	return s.setUIState(ctx, lastCreateGroupUIStateKey, strconv.FormatInt(groupID, 10))
}

// RecentCwd is one row of the §11.7 directory history: a resolved absolute
// path and the monotonic sequence number it was last promoted with. Most
// recent is the highest UsedSeq, never the newest wall-clock timestamp
// (SPEC §4/§13.1: order must stay assertable under a frozen DECK_CLOCK).
type RecentCwd struct {
	Path    string
	UsedSeq int64
}

// PromoteRecentCwd moves path to the front of the §11.7 directory history,
// deduplicated by exact string equality on the caller-resolved absolute
// path, and evicts every entry beyond limit. limit is caller-supplied
// (SPEC §6.5's [ui] recent_cwd_limit, default 5) so the store carries no
// opinion of its own; limit <= 0 keeps nothing at all, per the same rule
// that decides how many rows survive. The next sequence number is always
// one more than the current maximum, computed inside the same transaction
// as the insert/update and the eviction, so a promotion is atomic and the
// order stays deterministic under a frozen DECK_CLOCK -- used_seq is never
// derived from a clock value.
func (s *Store) PromoteRecentCwd(ctx context.Context, path string, limit int) error {
	if path == "" {
		return errors.New("recent cwd path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("recent cwd path %q must be an absolute, resolved path", path)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin promote recent cwd: %w", err)
	}
	defer rollbackTx(tx)
	var maxSeq sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(used_seq) FROM recent_cwds`).Scan(&maxSeq); err != nil {
		return fmt.Errorf("read recent cwd sequence: %w", err)
	}
	nextSeq := maxSeq.Int64 + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO recent_cwds (path, used_seq) VALUES (?, ?)
		ON CONFLICT(path) DO UPDATE SET used_seq = excluded.used_seq`, path, nextSeq); err != nil {
		return fmt.Errorf("promote recent cwd %q: %w", path, err)
	}
	if err := evictRecentCwdsTx(ctx, tx, limit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit promote recent cwd: %w", err)
	}
	return nil
}

// evictRecentCwdsTx deletes every recent_cwds row beyond the limit most
// recently used; limit <= 0 keeps nothing.
func evictRecentCwdsTx(ctx context.Context, tx *sql.Tx, limit int) error {
	if limit <= 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM recent_cwds`); err != nil {
			return fmt.Errorf("evict recent cwds to limit 0: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM recent_cwds WHERE path NOT IN (
		SELECT path FROM recent_cwds ORDER BY used_seq DESC LIMIT ?)`, limit); err != nil {
		return fmt.Errorf("evict recent cwds beyond limit %d: %w", limit, err)
	}
	return nil
}

// RecentCwds returns the §11.7 directory history, most recent first (highest
// UsedSeq first). It reflects exactly what PromoteRecentCwd has left in the
// table; callers that want only the first N should slice the result rather
// than pass a second, independent limit here.
func (s *Store) RecentCwds(ctx context.Context) ([]RecentCwd, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, used_seq FROM recent_cwds ORDER BY used_seq DESC`)
	if err != nil {
		return nil, fmt.Errorf("list recent cwds: %w", err)
	}
	defer closeRows(rows)
	var out []RecentCwd
	for rows.Next() {
		var rc RecentCwd
		if err := rows.Scan(&rc.Path, &rc.UsedSeq); err != nil {
			return nil, fmt.Errorf("scan recent cwd: %w", err)
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent cwds: %w", err)
	}
	return out, nil
}

// ClearRecentCwds drops every row from recent_cwds. Requirement 17 (§11.5):
// the settings takeover offers this as an explicit action because the
// history it clears "is history and paths can be sensitive", and it must
// remain non-load-bearing -- dropping the table costs the create modal's
// §11.7 prefill/cycling/completion (a fresh call to PromoteRecentCwd starts
// the table over from empty) and never a session: no row in `sessions`,
// `events`, or anything else this store owns is touched. Unlike
// PromoteRecentCwd, there is no path validation to do and nothing to keep:
// it is unconditionally `DELETE FROM recent_cwds`, not a caller-supplied
// limit's eviction.
func (s *Store) ClearRecentCwds(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM recent_cwds`); err != nil {
		return fmt.Errorf("clear recent cwds: %w", err)
	}
	return nil
}
