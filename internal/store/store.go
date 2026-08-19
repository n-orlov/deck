package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/config"
	_ "modernc.org/sqlite"
)

// SchemaVersion is the newest schema understood by this binary.
const SchemaVersion = 1

// Store is deck's sole SQLite writer API. Call Close when the client exits.
type Store struct {
	db   *sql.DB
	path string
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
	dsn := path
	if strings.Contains(dsn, "?") {
		dsn += "&_txlock=immediate"
	} else {
		dsn += "?_txlock=immediate"
	}
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
	version, err := store.version()
	if err != nil {
		db.Close()
		return nil, err
	}
	// Check this before setting journal mode or running a migration. A future
	// database is read only from this binary's point of view.
	if version > SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("state database schema version %d is newer than supported version %d; upgrade deck", version, SchemaVersion)
	}
	if err := os.Chmod(home, 0o700); err != nil {
		db.Close()
		return nil, fmt.Errorf("secure store directory: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure state database: %w", err)
	}
	if err := store.migrate(version); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("secure state database: %w", err)
	}
	return store, nil
}

// DB exposes the connection for narrowly scoped queries. Product mutations
// belong on Store methods so they remain targeted and transactional.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database location for external diagnostics.
func (s *Store) Path() string { return s.path }

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

	KilledByUser   bool
	PaneExitStatus *int
	CrashTail      string
	NotifyEpoch    int64
	LastMessage    string
	Acknowledged   bool

	LaunchArgs              []string
	Env                     map[string]string
	PreLaunch               string
	LoginShell              bool
	PermissionProfile       string
	PermissionProfileReason string
	ConversationID          string
	ResumePin               string
	ResumeState             string
}

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
	// is reserved for resume, which is the only operation allowed to make the
	// row automation-writable again.
	KilledByUser      bool
	ClearKilledByUser bool
	// Acknowledged explicitly changes the durable unseen marker. Independently,
	// every waiting/error transition resets it to false.
	Acknowledged *bool
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
	if input.ID == "" || input.Name == "" || input.CWD == "" || input.Agent == "" || input.CapturedPath == "" {
		return Session{}, errors.New("session id, name, cwd, agent, and captured path are required")
	}
	slug := Slug(input.Name)
	if slug == "" {
		return Session{}, fmt.Errorf("session name %q does not produce a usable slug", input.Name)
	}
	if input.Status == "" {
		input.Status = "starting"
	}
	if input.StatusSource == "" {
		input.StatusSource = "user"
	}
	if input.StatusAt == 0 || input.CreatedAt == 0 {
		return Session{}, errors.New("session status_at and created_at timestamps are required")
	}
	if input.ResumeState == "" {
		input.ResumeState = "auto"
	}
	if input.PermissionProfile == "" {
		input.PermissionProfile = "safe"
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
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions
		(id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at,
		 launch_args, env, pre_launch, login_shell, permission_profile, permission_profile_reason, conversation_id, resume_pin, resume_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, slug, input.CWD, input.Agent, input.CapturedPath,
		input.Status, input.StatusSource, input.StatusAt, input.CreatedAt,
		launchArgsJSON, envJSON, nullableString(input.PreLaunch), input.LoginShell,
		input.PermissionProfile, nullableString(input.PermissionProfileReason), nullableString(input.ConversationID), nullableString(input.ResumePin), input.ResumeState)
	if err != nil {
		if strings.Contains(err.Error(), "sessions.name") || strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.name") {
			return Session{}, fmt.Errorf("session name %q already exists", input.Name)
		}
		if strings.Contains(err.Error(), "sessions.slug") || strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.slug") {
			return Session{}, fmt.Errorf("session name %q collides with existing slug %q", input.Name, slug)
		}
		return Session{}, fmt.Errorf("insert session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("commit create session: %w", err)
	}
	return Session{
		ID: input.ID, Name: input.Name, Slug: slug, CWD: input.CWD, Agent: input.Agent,
		Status: input.Status, StatusSource: input.StatusSource, StatusAt: input.StatusAt, CreatedAt: input.CreatedAt,
		Acknowledged: true,
		LaunchArgs:   input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, LoginShell: input.LoginShell,
		PermissionProfile: input.PermissionProfile, PermissionProfileReason: input.PermissionProfileReason,
		ConversationID: input.ConversationID,
		ResumePin:      input.ResumePin, ResumeState: input.ResumeState,
	}, nil
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

// scanSession reads the extended Phase 1 create fields shared by ListSessions
// and GetSession.
func scanSession(row interface {
	Scan(dest ...any) error
}) (Session, error) {
	var session Session
	var launchArgsJSON, envJSON string
	var preLaunch, permissionProfileReason, conversationID, resumePin, crashTail, lastMessage sql.NullString
	var loginShell, killedByUser, acknowledged int
	var paneExitStatus sql.NullInt64
	if err := row.Scan(&session.ID, &session.Name, &session.Slug, &session.CWD,
		&session.Agent, &session.CapturedPath, &session.Status, &session.StatusReason, &session.StatusSource,
		&session.StatusAt, &session.CreatedAt, &killedByUser, &paneExitStatus, &crashTail,
		&session.NotifyEpoch, &lastMessage, &acknowledged, &launchArgsJSON, &envJSON, &preLaunch,
		&loginShell, &session.PermissionProfile, &permissionProfileReason, &conversationID, &resumePin, &session.ResumeState); err != nil {
		return Session{}, err
	}
	if err := json.Unmarshal([]byte(launchArgsJSON), &session.LaunchArgs); err != nil {
		return Session{}, fmt.Errorf("decode launch args: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &session.Env); err != nil {
		return Session{}, fmt.Errorf("decode env: %w", err)
	}
	session.PreLaunch = preLaunch.String
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
	session.Acknowledged = acknowledged != 0
	return session, nil
}

const sessionColumns = `id, name, slug, cwd, agent, captured_path, status,
		COALESCE(status_reason, ''), status_source, status_at, created_at,
		killed_by_user, pane_exit_status, crash_tail, notify_epoch, last_message, acknowledged,
		launch_args, env, pre_launch, login_shell, permission_profile, permission_profile_reason, conversation_id, resume_pin, resume_state`

// GetSession returns exactly one session by id, including every Phase 1
// create field.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	session, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, fmt.Errorf("session %q not found", id)
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return session, nil
}

// UpdateSessionStatus applies one verdict and records its source event in the
// same transaction. Losing verdicts are still events (important evidence that
// a probe ran), but cannot change the row. The immediate transaction configured
// by OpenPath makes the read/precedence/write sequence atomic across clients.
func (s *Store) UpdateSessionStatus(ctx context.Context, input StatusUpdateInput) error {
	if input.SessionID == "" || input.Status == "" {
		return errors.New("session id and status are required")
	}
	if input.Source == "" {
		input.Source = "user"
	}
	if input.At == 0 {
		return errors.New("session status timestamp is required")
	}
	if input.Source == "probe" && input.StaleAfter <= 0 {
		return errors.New("probe stale_after is required")
	}
	if input.KilledByUser && (input.Source != "user" || input.Status != "stopped") {
		return errors.New("killed_by_user requires a user-sourced stopped transition")
	}
	if input.EventKind == "" {
		input.EventKind = input.Status
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session status update: %w", err)
	}
	defer tx.Rollback()

	var currentStatus, currentSource, agent string
	var currentAt, notifyEpoch int64
	var killedByUser, acknowledged int
	var paneExitStatus sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT status, status_source, status_at, agent,
		killed_by_user, acknowledged, notify_epoch, pane_exit_status
		FROM sessions WHERE id = ?`, input.SessionID).Scan(
		&currentStatus, &currentSource, &currentAt, &agent, &killedByUser,
		&acknowledged, &notifyEpoch, &paneExitStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session %q not found", input.SessionID)
		}
		return fmt.Errorf("read session status: %w", err)
	}

	// A conditional transition that lost its race is a complete no-op: it must
	// not append an event claiming an action against a different current state.
	if input.ExpectedStatus != "" && currentStatus != input.ExpectedStatus {
		return nil
	}

	apply := killedByUser == 0 || input.ClearKilledByUser || input.KilledByUser
	if apply && input.Source == "probe" && currentSource == "hook" && input.At-currentAt < input.StaleAfter {
		apply = false
	}
	// tmux supplies terminal liveness, plus the one explicit shell promotion;
	// it cannot invent an agent's working state.
	tmuxLaunchObservation := input.Status == "starting" && currentStatus == "starting" && currentSource == "user"
	tmuxShellPromotion := input.Status == "running" && agent == "shell" && currentStatus == "starting"
	if apply && input.Source == "tmux" && input.Status != "stopped" && input.Status != "error" && !tmuxShellPromotion && !tmuxLaunchObservation {
		apply = false
	}
	// Crash collection is first-writer-only. A racing observer still records
	// what it saw, but does not replace the stored verdict or tail.
	if apply && input.PaneExitStatus != nil && paneExitStatus.Valid {
		apply = false
	}

	if apply {
		leavingAttention := isAttentionStatus(currentStatus) && !isAttentionStatus(input.Status)
		if leavingAttention {
			notifyEpoch++
		}
		if isAttentionStatus(input.Status) {
			acknowledged = 0
		} else if input.Acknowledged != nil {
			acknowledged = boolInt(*input.Acknowledged)
		}
		newKilled := killedByUser
		if input.ClearKilledByUser {
			newKilled = 0
		}
		if input.KilledByUser {
			newKilled = 1
		}
		lastMessage := truncateUTF8(input.LastMessage, 2*1024)
		_, err = tx.ExecContext(ctx, `UPDATE sessions SET
			status = ?, status_reason = ?, status_source = ?, status_at = ?,
			killed_by_user = ?, acknowledged = ?, notify_epoch = ?,
			pane_exit_status = COALESCE(?, pane_exit_status),
			crash_tail = CASE WHEN ? IS NULL THEN crash_tail ELSE ? END,
			last_message = CASE WHEN ? = '' THEN last_message ELSE ? END
			WHERE id = ?`, input.Status, input.Reason, input.Source, input.At,
			newKilled, acknowledged, notifyEpoch, input.PaneExitStatus,
			input.PaneExitStatus, input.CrashTail, lastMessage, lastMessage, input.SessionID)
		if err != nil {
			return fmt.Errorf("update session status: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, input.SessionID, input.At, input.EventKind, input.Reason, input.Payload); err != nil {
		return fmt.Errorf("record session status event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session status update: %w", err)
	}
	return nil
}

// AttachWaitingSession atomically records that a deck-mediated attachment
// answered the currently observed waiting episode. If the row stopped waiting
// after the UI loaded it, ExpectedStatus turns the stale keypress into a no-op.
func (s *Store) AttachWaitingSession(ctx context.Context, sessionID string, at int64) error {
	acknowledged := true
	return s.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: sessionID, Status: "running", Source: "user", At: at,
		EventKind: "attached", Acknowledged: &acknowledged, ExpectedStatus: "waiting",
	})
}

// AcknowledgeSession durably clears the selected row's unseen marker without
// changing its status verdict, source, timestamp, or notification epoch. It is
// intentionally a targeted update rather than a status transition: pressing Y
// acknowledges waiting/error but does not claim that the user answered it.
func (s *Store) AcknowledgeSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET acknowledged = 1 WHERE id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("acknowledge session: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("read acknowledged session count: %w", err)
	} else if changed == 0 {
		return fmt.Errorf("session %q not found", sessionID)
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
	defer tx.Rollback()
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

// mutateSessionWithEvent runs a single targeted UPDATE against exactly one
// session row and records a matching event in the same transaction, so
// observers never see a changed row without its corresponding event. eventKind
// and payload describe the event; query/args describe the row mutation.
func (s *Store) mutateSessionWithEvent(ctx context.Context, sessionID, fieldName, eventKind, source, payload string, at int64, query string, args ...any) error {
	if at == 0 {
		return errors.New("event timestamp is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set %s: %w", fieldName, err)
	}
	defer tx.Rollback()
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload)
		VALUES (?, ?, ?, ?, ?)`, sessionID, at, eventKind, source, payload); err != nil {
		return fmt.Errorf("record set %s event: %w", fieldName, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set %s: %w", fieldName, err)
	}
	return nil
}

// ListSessions returns all durable rows, including stopped rows. The stable
// order avoids hiding resumable sessions and keeps independently connected
// clients' views deterministic.
func (s *Store) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+`
		FROM sessions ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
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
	defer tx.Rollback()
	if version == 0 {
		for _, statement := range schemaV1 {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("create schema v1: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', ?) ON CONFLICT(key) DO UPDATE SET version = excluded.version`, SchemaVersion); err != nil {
			return fmt.Errorf("record schema version: %w", err)
		}
	} else {
		return fmt.Errorf("no migration path from schema version %d", version)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state database migration: %w", err)
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
