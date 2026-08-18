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
	"time"

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
	db, err := sql.Open("sqlite", path)
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
	Status       string
	StatusReason string
	StatusSource string
	StatusAt     int64
	CreatedAt    int64

	LaunchArgs        []string
	Env               map[string]string
	PreLaunch         string
	LoginShell        bool
	PermissionProfile string
	ConversationID    string
	ResumePin         string
	ResumeState       string
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
	now := time.Now().UnixMilli()
	if input.StatusAt == 0 {
		input.StatusAt = now
	}
	if input.CreatedAt == 0 {
		input.CreatedAt = now
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
		 launch_args, env, pre_launch, login_shell, permission_profile, conversation_id, resume_pin, resume_state)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, input.Name, slug, input.CWD, input.Agent, input.CapturedPath,
		input.Status, input.StatusSource, input.StatusAt, input.CreatedAt,
		launchArgsJSON, envJSON, nullableString(input.PreLaunch), input.LoginShell,
		input.PermissionProfile, nullableString(input.ConversationID), nullableString(input.ResumePin), input.ResumeState)
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
		LaunchArgs: input.LaunchArgs, Env: input.Env, PreLaunch: input.PreLaunch, LoginShell: input.LoginShell,
		PermissionProfile: input.PermissionProfile, ConversationID: input.ConversationID,
		ResumePin: input.ResumePin, ResumeState: input.ResumeState,
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
	var preLaunch, conversationID, resumePin sql.NullString
	var loginShell int
	if err := row.Scan(&session.ID, &session.Name, &session.Slug, &session.CWD,
		&session.Agent, &session.Status, &session.StatusReason, &session.StatusSource,
		&session.StatusAt, &session.CreatedAt, &launchArgsJSON, &envJSON, &preLaunch,
		&loginShell, &session.PermissionProfile, &conversationID, &resumePin, &session.ResumeState); err != nil {
		return Session{}, err
	}
	if err := json.Unmarshal([]byte(launchArgsJSON), &session.LaunchArgs); err != nil {
		return Session{}, fmt.Errorf("decode launch args: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &session.Env); err != nil {
		return Session{}, fmt.Errorf("decode env: %w", err)
	}
	session.PreLaunch = preLaunch.String
	session.ConversationID = conversationID.String
	session.ResumePin = resumePin.String
	session.LoginShell = loginShell != 0
	return session, nil
}

const sessionColumns = `id, name, slug, cwd, agent, status,
		COALESCE(status_reason, ''), status_source, status_at, created_at,
		launch_args, env, pre_launch, login_shell, permission_profile, conversation_id, resume_pin, resume_state`

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

// UpdateSessionStatus changes exactly one session and records its transition in
// the append-only event log. Both writes share a transaction, so observers
// never see a changed status without its corresponding event.
func (s *Store) UpdateSessionStatus(ctx context.Context, input StatusUpdateInput) error {
	if input.SessionID == "" || input.Status == "" {
		return errors.New("session id and status are required")
	}
	if input.Source == "" {
		input.Source = "user"
	}
	if input.At == 0 {
		input.At = time.Now().UnixMilli()
	}
	if input.EventKind == "" {
		input.EventKind = input.Status
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session status update: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE sessions
		SET status = ?, status_reason = ?, status_source = ?, status_at = ?
		WHERE id = ?`, input.Status, input.Reason, input.Source, input.At, input.SessionID)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check session status update: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("session %q not found", input.SessionID)
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
		permission_profile TEXT NOT NULL DEFAULT 'safe', conversation_id TEXT, resume_pin TEXT,
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
