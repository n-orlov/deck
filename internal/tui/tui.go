// Package tui implements deck's interactive terminal interface.
package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// Model is the base session-list screen. Later modal and action work extends
// this model rather than providing a separate command-line interface.
type Model struct {
	store       *store.Store
	settings    config.Settings
	sessions    []store.Session
	startupNote string
	help        bool
	creating    bool
	detail      bool
	// helpScroll/detailScroll are task 078's height-bounding fix (requirement
	// 39 residual): the help overlay and `i` detail view are the only
	// widgets on screen while open, so unlike every bounded §11.4 dialog
	// their content can exceed the frame budget (helpText alone is 273
	// lines at 80x24). Rather than truncate, framedDialogScrollable clips to
	// a scrollable window; these hold the window's top line, reset to 0
	// every time the overlay opens (never sticking across a close/reopen,
	// like envReveal). See eventLogScroll below for the event log's own.
	helpScroll   int
	detailScroll int
	// createScroll is task 016's own instance of the same helpScroll/
	// detailScroll pattern above: the create modal's field set can wrap
	// well past the frame budget (framedDialogScrollable's own doc
	// comment measured it at 29 lines untouched, before this task's own
	// theming/candidate-list growth), so PgUp/PgDn (updateCreate) scroll
	// this window instead of the modal ever truncating its own submit
	// line away. Reset to 0 every time `n` opens the modal, same as the
	// other two never sticking across a close/reopen.
	createScroll     int
	createName       string
	createCWD        string
	createAgent      string
	createProfile    string
	createLaunchArgs string
	createEnv        string
	createPreLaunch  string
	createLoginShell bool
	// createProfileTouched is true once the user has cycled the Permission
	// profile field (field 3) itself in the currently open create modal. It
	// gates cycleCreateField's Agent case (field 2): while false, the value
	// showing in createProfile is only ever "whatever defaultCreateProfile
	// picked for the current agent" (steer 017 item 2 / yolo_default), so an
	// agent change must re-run that default for the new agent; once true,
	// the user's own explicit choice is preserved across an agent change
	// (falling back to options[0] only if it becomes unavailable). Reset to
	// false every time the modal opens fresh, alongside createProfile.
	createProfileTouched bool
	// createYoloConfirmed once tracked the explicit "y" confirm the yolo
	// double-gate required before create could submit with profile=="yolo"
	// (SPEC §5 pre-steer-017). Steer 017 item 2 removed that confirm --
	// allow_yolo alone gates yolo's availability now -- so this field is
	// gone; see docs/reports/phase3d-214-yolo-degate.md for the removal.
	// createCWDPrefilled is true while m.createCWD still holds an untouched
	// prefill deck itself chose (either the most recent §11.7 recent_cwds
	// entry, or with no history the directory deck was started in) rather
	// than anything the user typed. The first keystroke in the cwd field
	// -- rune or backspace -- clears the prefill wholesale before acting,
	// rather than editing it in place (task 008).
	createCWDPrefilled bool
	// createCWDLastUsed is true only when createCWDPrefilled's value came
	// from §11.7 recent_cwds history (as opposed to the no-history
	// directory-deck-started-in fallback), driving createFieldRows' "last
	// used" label. Cleared together with createCWDPrefilled on first edit.
	createCWDLastUsed bool
	// createCWDRecents is the §11.7 recent_cwds snapshot the cwd field is
	// currently cycling through (task 009), fetched once when up/down
	// first starts a cycle rather than re-queried on every keypress, so
	// the list a user is cycling through cannot change under them mid-
	// cycle. nil while not cycling.
	createCWDRecents []store.RecentCwd
	// createCWDRecentIndex is the 0-based position within createCWDRecents
	// the field currently shows (rendered 1-based as "recent N/M"), or -1
	// while not cycling at all -- neither an untouched prefill nor a
	// cycled recent entry, just whatever the user has typed (task 009).
	createCWDRecentIndex int
	// createCWDPreCycleValue/Prefilled/LastUsed snapshot the cwd field's
	// state from the moment before the first "up" started a cycle, so
	// pressing "down" back past the most recent entry restores exactly
	// what was there -- the untouched §11.7 prefill and its "last used"
	// label, or whatever the user had already typed -- rather than
	// leaving the field on recents[0] or blanking it.
	createCWDPreCycleValue     string
	createCWDPreCyclePrefilled bool
	createCWDPreCycleLastUsed  bool
	// createCWDCandidates is task 012's bash-completion-contract listing
	// branch: non-nil only while tab has reached the longest common prefix
	// among the segment's directory matches and at least two still remain
	// (it cannot advance the text any further), so the user picks one
	// explicitly instead of a right-arrow ghost guessing among them (task
	// 011's reasoning against ghosting an ambiguous match applies here
	// too). nil whenever the list is closed -- not open, or just closed by
	// any edit, field change, selection or esc.
	createCWDCandidates []string
	// createCWDCandidateIndex is the 0-based highlighted entry within
	// createCWDCandidates, moved by up/down while the list is open (which
	// suspends the field's own §11.7 recent_cwds cycling for as long as
	// the list is open, exactly as it is already suspended while an
	// ambiguous-match count or a ghost is shown instead).
	createCWDCandidateIndex int
	createField             int
	createError             string
	create                  func(context.Context, service.ShellCreateInput) (store.Session, error)
	createAgentSession      func(context.Context, service.AgentCreateInput) (store.Session, error)
	attach                  func(context.Context, string) (*exec.Cmd, error)
	prepareAttach           func(context.Context, string) error
	kill                    func(context.Context, store.Session) error
	acknowledge             func(context.Context, string) error
	reconcile               func(context.Context) error
	resume                  func(context.Context, string) (store.Session, service.ResumeOutcome, error)
	// restart is task 022's `R` (SPEC §6.2/§6.3): kills the selected
	// session's live pane if one exists and relaunches it with the
	// adapter's resume argv (same conversation id, never a fresh one),
	// clearing env_dirty on success -- the only path that ever applies a
	// pending `e`-editor edit to an already-running process. nil means
	// restarting is unavailable.
	restart func(context.Context, string) (store.Session, service.ResumeOutcome, error)
	// inject is task 023's "inject instead" alternative to restart, offered
	// only for a shell session: it exports the keys changed since env_dirty
	// was last cleared straight into the live, already-running pane's shell
	// (never killing or relaunching it) and clears env_dirty the same way a
	// successful restart does. nil means injecting is unavailable.
	inject func(context.Context, string) (store.Session, []string, error)
	// restartChoosing is true while the `R` restart/inject-instead choice
	// (task 023, shell sessions only) is open; restartChoiceValue is the
	// locally-held candidate ("restart" or "inject"), defaulting to
	// "restart" so an already-existing R workflow needs only one extra
	// Enter to keep behaving exactly as before task 023.
	restartChoosing    bool
	restartChoiceValue string
	restartChoiceNote  string
	// deleteSvc is task 105's `dd` submit path: it kills the selected
	// session's live pane if one exists (never touching its cwd or
	// conversation id/transcript) and then tombstones the row
	// (store.SoftDeleteSession) so it disappears from ListSessions
	// immediately, restorable within task 106's grace window. nil means
	// deleting is unavailable and submitting states so rather than
	// silently doing nothing.
	deleteSvc func(context.Context, store.Session) error
	// restoreSvc is task 106's `u` undo for a completed dd: it clears the
	// tombstone (store.RestoreSession) so the row returns to ListSessions.
	// nil means restoring is unavailable and u is a no-op, exactly like
	// undoSessionID being empty makes u a no-op for the kill-undo path.
	restoreSvc func(context.Context, string) (store.Session, error)
	// reapSvc is task 106's DECK_DELETE_GRACE_MS expiry: it permanently
	// removes a tombstoned row that was never restored
	// (store.ReapSession). nil means the grace-window tick simply clears
	// the toast without reaping -- see deleteGraceExpired.
	reapSvc func(context.Context, string) error
	// purgeSvc is task 110's non-default "purge conversation" choice inside
	// the dd confirm dialog: it deletes exactly the path already resolved
	// by the dialog itself via the session's own adapter's declared
	// TranscriptPaths (task 109) -- it never resolves a path on its own and
	// is never called with an empty one. nil means purging is unavailable;
	// submitting with purge chosen then reports so rather than silently
	// deleting nothing while claiming success.
	purgeSvc func(context.Context, string) error
	// archiveSvc is task 111's `A` submit path (SPEC requirement 27): it
	// kills the selected session's live pane first if one exists (offering
	// "kill and archive" as one action rather than refusing), then sets
	// archived_at (store.ArchiveSession) -- a flag, never a status, so the
	// row keeps whatever Status it ends up with. nil means archiving is
	// unavailable and submitting states so rather than silently doing
	// nothing.
	archiveSvc func(context.Context, store.Session) error
	// unarchiveSvc is R71's `U` (SPEC.md:323-332, issue #8): the reverse of
	// archiveSvc above, clearing archived_at (store.UnarchiveSession via
	// service.Unarchive) so the row returns to ListSessions' default view.
	// It never resumes or relaunches anything -- a row killed on its way
	// into the archive comes back stopped, exactly as restoreSvc's undo of
	// a dd never un-kills the pane it killed. nil means unarchiving is
	// unavailable and `U` says so rather than silently doing nothing.
	unarchiveSvc func(context.Context, string) (store.Session, error)
	// pendingDelete is true for exactly one keypress after a lone `d`
	// (SPEC's dd chord): a second `d` opens deleteConfirming; ANY other
	// key (including Esc) clears pendingDelete without performing any
	// destructive action -- the pending indicator itself never touches
	// the store.
	pendingDelete bool
	// deleteConfirming is true while the second `d`'s confirm dialog is
	// open; deleteNote surfaces a failed submit (mirrors pinNote/
	// profileSwitchNote's existing shape) without closing the dialog.
	deleteConfirming bool
	deleteNote       string
	// deletePurgeValue is task 110's non-default "purge conversation"
	// choice inside the dd confirm dialog (never offered anywhere else,
	// never default): left/right cycles deletePurgeOptions, reset to
	// "keep" every time the dialog opens. deletePurgePath/deletePurgeOK
	// are resolved once, at that same moment, via the selected session's
	// own adapter's declared TranscriptPaths (task 109) -- never guessed,
	// never re-derived from anything the dialog renders.
	deletePurgeValue string
	deletePurgePath  string
	deletePurgeOK    bool
	// archiveConfirming is R72's `A` confirm dialog (issue #10,
	// SPEC.md:752): `A` writes NOTHING on the keypress -- it only opens
	// this dialog, and the dialog's own Enter is the first thing that ever
	// reaches archiveSvc, so the kill a live row's archive performs is
	// never a single unconfirmed keystroke (`a`, attach, is one Shift
	// away). archiveNote surfaces a failed submit without closing the
	// dialog, exactly as deleteNote does above. Modelled on
	// deleteConfirming throughout: same dialog contract
	// (applyDialogContract), same suppression of the bare-letter keymap and
	// of the mouse while it is open.
	archiveConfirming bool
	archiveNote       string
	profileSwitch     func(context.Context, string, string) (store.Session, error)
	selected          int
	// pendingSelectSessionID is requirement 52's one-shot "select the
	// session I just created" intent: submitCreate's shellCreated success
	// path (below) records the new session's id here rather than acting
	// immediately, since the id does not exist in m.sessions until the
	// sessionsLoaded that follows lands. The FIRST sessionsLoaded whose
	// (filtered) m.sessions contains this id selects that row by id --
	// never by index, since attention order can place a brand-new session
	// anywhere -- scrolls it into view, and clears this field so a later
	// sessionsLoaded never re-steals a selection the user has since moved
	// (one-shot). A filter query that excludes the new session leaves both
	// the selection and the query untouched and simply leaves this field
	// set, waiting for a load where the id is visible; if the id never
	// appears at all (session immediately gone), the field is harmlessly
	// left set forever rather than panicking or forcing a selection.
	pendingSelectSessionID string
	// startCWD is the directory deck itself was started in (os.Getwd() at
	// New(), best-effort -- "" on error), used to prefill the create
	// modal's cwd field when §11.7's recent_cwds history is empty.
	startCWD        string
	collapsedGroups map[string]bool
	attachError     string
	resumeNote      string
	// undoSessionID/undoSessionName track the session killed by the most
	// recent x, so u can undo it (requirement 22) within DECK_UNDO_MS
	// without depending on it still being the current selection -- "undo"
	// undoes the last kill, not "whatever row happens to be selected".
	// undoGeneration invalidates a stale undoExpired tick left over from an
	// earlier kill/undo cycle once a newer one has started or been undone.
	undoSessionID   string
	undoSessionName string
	undoGeneration  int
	// deleteUndoSessionID/deleteUndoSessionName/deleteUndoGeneration mirror
	// undoSessionID/undoSessionName/undoGeneration exactly (task 106), but
	// track a successful dd delete's own DECK_DELETE_GRACE_MS window rather
	// than x's DECK_UNDO_MS one. They are kept independent trackers, never
	// merged with the trio above: dd's internal kill step never emits
	// sessionKilled, so the two undo windows never contend for the same
	// session at once, and u below checks the kill-undo trio first, falling
	// back to this one only when it is empty.
	deleteUndoSessionID   string
	deleteUndoSessionName string
	deleteUndoGeneration  int
	// marked is task 112's `m` mark set (requirement 28), keyed by session
	// id rather than any index or visual position -- a re-sort
	// (attention.go, run on every loadSessions) or a re-group
	// (collapsedGroups, group.go) never moves the underlying data this map
	// keys off, so the set survives either without any special-case code.
	// x and dd consult it FIRST (via markedSessions(), which resolves it
	// through visualOrder() in the sidebar's own painted order, never
	// index arithmetic): non-empty means "act on the whole batch", empty
	// means the pre-existing single-selected-row behaviour. It clears on
	// that action (kill fires immediately; delete clears it the instant
	// the confirm dialog is submitted, before the async kill/tombstone
	// step even runs) and on a plain Esc at the top level.
	marked map[string]bool
	// batchUndoSessionIDs/batchUndoGeneration mirror undoSessionID/
	// undoGeneration exactly, but hold every session ID a marked-set `x`
	// just killed, so a single `u` undoes the WHOLE batch in one action
	// rather than leaving N separate single-session windows the operator
	// would have to undo one press at a time. Never merged with
	// undoSessionID: a plain unmarked x always uses that trio instead, and
	// u below checks undoSessionID first, falling back to this one only
	// when it is empty.
	batchUndoSessionIDs []string
	batchUndoGeneration int
	// batchDeleteUndoSessionIDs/batchDeleteUndoGeneration mirror
	// deleteUndoSessionID/deleteUndoGeneration exactly, for a marked-set
	// dd's own DECK_DELETE_GRACE_MS window, covering every session that
	// dd's bulk submit tombstoned in one shared window rather than N.
	batchDeleteUndoSessionIDs []string
	batchDeleteUndoGeneration int
	// archiveUndoSessionID/archiveUndoSessionName/archiveUndoGeneration are
	// R72's THIRD undo trio (issue #10, SPEC.md:752 "On success a toast says
	// what happened, with `u` to undo"), mirroring the kill trio
	// (undoSessionID, above) and the delete trio (deleteUndoSessionID, above)
	// field for field and generation-tie for generation-tie. Kept a separate
	// tracker rather than merged into either: an archive's undo is
	// UnarchiveSession (R71's `U` service, clearing archived_at), not a
	// resume and not a tombstone restore, so a merged trio would have to
	// guess which reversal a given id wants. It runs on DECK_UNDO_MS, x's
	// window rather than dd's grace window, because nothing is reaped when it
	// expires -- expiry only takes the toast away, exactly as x's does.
	// archiveUndoSessionName is recorded for symmetry with its two siblings
	// and is deliberately NOT rendered in the toast (see
	// archiveUndoNoteLines).
	archiveUndoSessionID   string
	archiveUndoSessionName string
	archiveUndoGeneration  int
	// archiveUndoKilled records whether the archive this window undoes also
	// killed a live pane (the row's status was not "stopped" when the confirm
	// was submitted), so the toast can say what actually happened without
	// re-reading a row that has since left the default list.
	archiveUndoKilled  bool
	profileSwitching   bool
	profileSwitchValue string
	profileSwitchNote  string
	// profileSwitchYoloOK once tracked P's own yolo confirm keystroke,
	// mirroring createYoloConfirmed above; removed by the same steer 017
	// item 2 change.
	resumeMode func(context.Context, string, string) (store.Session, error)
	pinning    bool
	pinValue   string
	pinNote    string
	// renamer is task 013's `i`-dialog-only rename action (SPEC §11.4, PRD
	// requirement 31, I-8): it persists a new display name for the
	// selected session and never touches its live tmux session -- deck's
	// display name and tmux's session name are decoupled by
	// service.Rename/store.RenameSession itself (never writes `slug`), not
	// merely by this dialog choosing not to ask. nil means renaming is
	// unavailable and submitting states so rather than silently doing
	// nothing.
	renamer func(context.Context, string, string) (store.Session, error)
	// renaming is true while the rename sub-dialog (reachable ONLY from
	// inside the `i` detail dialog, never as a top-level key) is open;
	// m.detail stays true underneath it the whole time, so cancelling or
	// submitting a rename returns to detailView, not the main list.
	// renameValue is the locally-held candidate name, prefilled with the
	// session's current name; renamePrefilled marks it untouched, so the
	// very first typed rune or backspace replaces it wholesale exactly like
	// createView's cwd field and the env editor's value field.
	renaming        bool
	renameValue     string
	renamePrefilled bool
	renameNote      string
	// envEditing is task 020's `e` env editor (SPEC §6.1/§6.3): a listing
	// of the selected session's effective environment, one row per key,
	// naming which layer (server env, captured_path, config [env], session
	// env) supplied the value actually in force. envCursor selects a row;
	// enter on it opens envEditKey/envEditValue (task 021's write path --
	// non-empty envEditKey means a value is being typed right now). Esc
	// while typing cancels only that edit; esc otherwise closes the whole
	// dialog, changing nothing further. setSessionEnv persists a committed
	// edit (env_dirty, tmux mirror); nil means editing is unavailable and
	// submitting states so rather than silently doing nothing.
	envEditing               bool
	envCursor                int
	envEditKey, envEditValue string
	// envEditPrefilled tracks task 021's cwd-field-style prefill: enter
	// preloads envEditValue with the row's current value, and the very
	// FIRST typed rune or backspace replaces it wholesale rather than
	// editing within it (see updateEnvDialog).
	envEditPrefilled bool
	envNote          string
	// envReveal is task 010's per-view explicit reveal toggle (SPEC §6.4,
	// requirement 21): false (the default every time the dialog opens, per
	// "r" below) masks every secret-shaped row's value via maskEnvValue;
	// "r" while browsing (not mid-edit) flips it back off, never sticking
	// across a close/reopen of the dialog.
	envReveal     bool
	setSessionEnv func(context.Context, string, string, string) (store.Session, error)
	// eventLogOpen is task 124's `E` event log (SPEC §12/requirement 32,
	// I-9): a read-only, newest-first listing of store.Event rows across
	// every session, each payload run through maskEventPayload -- the same
	// task 010 isSecretShapedKey/maskedSecretPlaceholder predicate the `e`
	// env editor uses -- so a payload that happens to carry a secret-shaped
	// key's value is never shown in the clear here either. There is
	// nothing to submit or cycle (like detailView/helpView); Esc is its
	// only interaction. eventLogScroll is task 078's height-bounding fix
	// (requirement 39 residual, see helpScroll's own comment above), reset
	// to 0 every time `E` opens the log.
	eventLogOpen   bool
	eventLogScroll int
	// eventLogRows/eventLogErr are R61's (steer 3e-001 §6.3) in-memory copy
	// of loadEventLog's ListEvents result: populated exactly once, by the
	// tea.Cmd the "E" key handler dispatches when the dialog opens, and
	// read by eventLogBody/View() -- never re-fetched from m.store while
	// the dialog stays open, no matter how many reconcileTick/previewTick
	// messages land in the meantime. Both reset to zero value the same
	// place eventLogScroll resets, so a reopen never shows a stale error
	// or a stale row set from a previous visit.
	eventLogRows []store.Event
	eventLogErr  error
	// filtering is task 123's `/` list filter (SPEC §11.3/requirement 33,
	// I-10): true while the filter's own text field has keyboard focus
	// (see updateFilter). filterQuery is the live, incrementally-applied
	// query text. Unlike every full-screen dialog above, filtering never
	// takes over View() -- the sidebar keeps rendering the (now filtered)
	// session list underneath it (filterStatusLine states the filter is
	// in force, per SPEC requirement 33's own wording, so a hidden row is
	// never mistaken for a deleted one), and it stays applied after Enter
	// closes the text field, clearing only on Esc.
	filtering   bool
	filterQuery string
	// baseSessions is exactly what sessionsLoaded's ListSessions() call
	// returned (SPEC's default view: excludes both tombstoned and
	// archived rows) -- the one thing that handler ever assigns directly.
	// m.sessions, by contrast, is the DISPLAYED list: baseSessions
	// unfiltered when no query is in force, or filteredSessions()'s
	// baseSessions-plus-archivedSessions match set while one is. Kept as
	// two fields, rather than deriving one from the other on the fly at
	// every read site, because every existing selection/navigation/render
	// primitive already reads m.sessions directly (dozens of call sites);
	// recomputing the displayed list at the few places that change what
	// it should contain is far less invasive than teaching all of them
	// about a filter.
	baseSessions []store.Session
	// archivedSessions caches requirement 33's ONLY route back to an
	// archived row (Store.ListArchivedSessions): fetched fresh every time
	// `/` opens (loadArchivedSessions), so the filter's archived-side
	// search pool is at most one keypress stale. Empty and unread
	// whenever no filter query is in force.
	archivedSessions []store.Session
	// settingsOpen is task 013's `,` full-screen takeover (SPEC §11.5): a
	// category list and the selected category's field list, both walking
	// config.Schema rather than a hand-written field set. It is not a
	// §11.4 dialog (no framedDialog, no shared dialog-contract retrofit --
	// see task 029) and replaces the whole frame the same way m.creating
	// etc already do, so it is checked in the same tea.KeyMsg/View()
	// early-return chain as those, never layered atop mainView.
	settingsOpen bool
	// settingsCategoryIndex/settingsFieldIndex select the left list's
	// category and, within it, the right list's field. Both default to 0
	// ("General", its first field) on open; task 014 adds the tab/left/
	// right/up/down navigation that moves them and the `/` search that
	// can jump either. This task only renders whichever index the state
	// holds, never hand-picks what to render.
	settingsCategoryIndex int
	settingsFieldIndex    int
	// settingsFocus is task 014's tab/left/right target: settingsFocus
	// Categories while the category list drives up/down, settingsFocus
	// Fields once tab/left/right has moved it to the field list — see
	// SPEC §11.5: "tab/left/right switch between the category list and the
	// field list, up/down move within the focused list."
	settingsFocus int
	// settingsSearchActive/settingsSearchQuery/settingsSearchIndex are
	// task 014's `/` fuzzy search: settingsSearchActive is true while the
	// query is being typed and results browsed, settingsSearchQuery holds
	// the typed text, settingsSearchIndex selects among the matches
	// settingsSearchMatches(settingsSearchQuery) returns (in schema order,
	// searched across every category, per §11.5's "search every field by
	// label and description").
	settingsSearchActive bool
	settingsSearchQuery  string
	settingsSearchIndex  int
	// settingsEdits is task 015's staged working copy of every schema field's
	// value: a config.FileConfig seeded from m.settings when the takeover
	// opens (settingsEditsFromSettings) and mutated in place as toggle/
	// integer/enum fields are edited. Task 016 hands this to
	// config.WriteConfigFile on save; nothing here writes config.toml or
	// changes a live pane on its own -- editing only ever changes this
	// staged copy, never m.settings itself, so a field labelled
	// restart-to-apply genuinely cannot affect an already-running pane
	// through this path alone.
	settingsEdits config.FileConfig
	// settingsSavedEdits is task 016's "what is actually on disk right
	// now" snapshot: seeded from the same settingsEditsFromSettings call
	// that seeds settingsEdits when `,` opens, and replaced with a copy of
	// settingsEdits only after a successful config.WriteConfigFile. esc
	// compares settingsEdits against THIS, not against m.settings itself
	// (m.settings is only ever refreshed by a restart -- see field kind's
	// own "restart-to-apply" scope, req 19 -- so it would still disagree
	// with settingsEdits right after a save that changed nothing else,
	// wrongly re-prompting to discard a change that is already safely on
	// disk).
	settingsSavedEdits config.FileConfig
	// settingsDiscardConfirm is true while esc's "you have unsaved
	// changes" prompt (SPEC requirement 14/20) is showing: y/enter
	// discards (closes the takeover, leaving config.toml exactly as
	// settingsSavedEdits already has it -- never written to), n/esc
	// dismisses the prompt and returns to editing.
	settingsDiscardConfirm bool
	// settingsNote surfaces ctrl+s's outcome (task 012's atomic writer can
	// fail -- e.g. an unwritable config directory -- and that failure must
	// be visible, not swallowed) and the discard prompt's own text.
	// Mirrors profileSwitchNote/pinNote's existing shape in this package.
	settingsNote string
	// settingsEnvOpen/settingsEnvIndex/settingsEnvEditing/
	// settingsEnvEditingKeyPart/settingsEnvEditKey/settingsEnvEditValue/
	// settingsEnvEditOriginalKey are task 003's [env] entry editor (SPEC
	// requirement 17: the global [env] table is genuinely editable in the
	// takeover, not display-only). settingsEnvOpen is true while the
	// entries list (one row per settingsEdits.Env key, plus a trailing
	// "add entry" row) has taken over the field panel; settingsEnvIndex
	// selects a row in that list. settingsEnvEditing is true while a
	// single entry's key or value is being typed; settingsEnvEditingKeyPart
	// says which of the two free-text buffers (settingsEnvEditKey/
	// settingsEnvEditValue) is currently receiving typed runes.
	// settingsEnvEditOriginalKey holds the key being edited (empty when
	// adding a new entry), so committing a renamed key removes the old one
	// rather than leaving both. All of this only ever mutates
	// settingsEdits.Env, the same staged copy every other field kind edits
	// -- ctrl+s/esc still govern when (or whether) it reaches config.toml.
	settingsEnvOpen            bool
	settingsEnvIndex           int
	settingsEnvEditing         bool
	settingsEnvEditingKeyPart  bool
	settingsEnvEditKey         string
	settingsEnvEditValue       string
	settingsEnvEditOriginalKey string
	// settingsEnvReveal is task 010's per-view explicit reveal toggle (SPEC
	// §6.4, requirement 21): false (reset every time the entries list is
	// opened) masks every secret-shaped entry's value via maskEnvValue;
	// "r" while browsing the entries list (not mid-edit) flips it.
	settingsEnvReveal bool
	// themePicking is task 025's `t` picker (SPEC §11.6, requirement 27): it
	// does NOT replace the whole frame the way m.creating/m.settingsOpen do
	// -- the point of the picker is that the REAL session list stays on
	// screen and is coloured with whatever theme is currently highlighted
	// (activeTheme() consults themePickerValue while this is true), so
	// mainView keeps rendering and simply grows a themePickerLines() banner
	// (mirroring themeBanner/attachErrorLines' own "extra reserved rows"
	// shape) rather than gaining a second, separate full-screen view.
	themePicking bool
	// themePickerValue is the theme NAME currently highlighted in the
	// picker (never the *theme.Theme itself, so "not found" -- an empty
	// list, or a name that raced a user-theme file being deleted mid-pick
	// -- is representable and handled rather than a nil pointer). esc never
	// writes this anywhere durable; only themePickerConfirm (enter) does,
	// and only after resolving it back to a concrete theme.
	themePickerValue string
	// themePickerNote surfaces enter's outcome (a failed config.toml write,
	// or an empty list making enter a no-op), mirroring profileSwitchNote/
	// pinNote/settingsNote's identical shape elsewhere in this package.
	themePickerNote string
	width           int
	height          int
	agents          *agent.Registry
	// layoutMode and sidebarWidth are the §11.2 layout pin and the
	// persisted sidebar width, both persisted to state.db's ui_state table
	// by persistLayoutMode/persistSidebarWidth (task 016). "" and 0 both
	// mean "use the default", which ComputeLayout already treats as
	// auto/35 respectively.
	layoutMode   string
	sidebarWidth int
	// preCollapseLayoutMode and layoutCycleActive track requirement 33's
	// "click the collapsed strip -> restores the previous non-collapsed
	// mode": preCollapseLayoutMode is the pin the user actually had in
	// force before they started the *current* `|` excursion (not merely
	// the mode one hop back -- side-by-side -> stacked -> collapsed still
	// remembers side-by-side, the mode the excursion began from, not the
	// stacked stop it passed through on the way). layoutCycleActive is
	// true for as long as that excursion is still under way (it clears
	// once `|` wraps back to auto, or the strip is clicked, so the next
	// press anchors fresh from wherever the pin then stands). Both are
	// only read/written by cycleLayoutMode and restoreFromCollapsedStrip
	// -- unset (""/false) means "nothing recorded yet, restore to auto",
	// the same default every other unset pin uses.
	preCollapseLayoutMode string
	layoutCycleActive     bool
	// previewCapture is the read-only preview capture engine (SPEC
	// requirements 21, 22): given the selected row's slug, it returns
	// exactly one tmux.PreviewCapture per call, never attaching a client,
	// spawning a control-mode server, using pipe-pane, or resizing a pane.
	// nil (the zero value, and every constructor below New) leaves the
	// preview exactly as inert as before this task landed.
	previewCapture func(context.Context, string) (tmux.PreviewCapture, error)
	// previewSessionID, previewLive and previewBytes hold the most recent
	// successful capture, keyed by the session it was captured for so a
	// stale reply arriving after the selection moved on is never rendered
	// against the wrong row. Rendering these (crop, geometry line, cell-aware
	// truncation) is tasks 018-021; this task only maintains them.
	previewSessionID string
	previewLive      bool
	previewBytes     []byte
	// previewPaneWidth/previewPaneHeight are the real tmux pane geometry at
	// the moment previewBytes was captured (SPEC requirement 23's "45x22 of
	// 120x40" line, task 018) — independent of previewBytes' own line count,
	// since tmux trims trailing blank lines/columns from a capture.
	previewPaneWidth  int
	previewPaneHeight int
	// previewFitSessionID names the session ID whose window passive fit
	// (SPEC §11, steer 018 item 4) has already settled against -- the
	// coalescing mechanism previewFit uses to fit at most once per
	// SETTLED selection rather than once per row walked while holding an
	// arrow key: previewTick fires every DECK_PREVIEW_MS regardless of how
	// many times m.selected changed since the last tick, and previewFit
	// only issues a fit when the currently selected session's ID differs
	// from this field, which is updated (to the attempted session's ID,
	// whether or not the fit itself succeeded) only once the attempt
	// completes. Deliberately keyed to SELECTION identity, not to the
	// preview panel's own geometry: a sidebar-width, layout-mode or outer-
	// terminal change that leaves the selection unchanged does not retrigger
	// a fit (steer 018 §1b's amendment to the passive-preview no-resize
	// guarantee is scoped to fitting-on-navigation only, per task 215/
	// docs/reports/phase3d-215-preview-fit-on-nav.md) -- best-effort, so a
	// session left stale by one of those axes is corrected the next time
	// its own selection settles again, never continuously chased. Reset to
	// "" by exitInteractive so the session interactive mode just left (whose
	// geometry was restored to its PRE-entry size, not the panel's) is
	// re-evaluated on the very next tick even though its ID has not
	// changed.
	previewFitSessionID string
	// previewFitInFlight names the session ID of the ONE passive fit
	// currently outstanding -- set when previewFit SCHEDULES its command
	// (not when that command runs), cleared when the matching
	// previewFitDone lands (task R63). previewFitSessionID alone cannot
	// coalesce overlapping attempts: it is only written once an attempt
	// COMPLETES, so two previewTicks closer together than one
	// PreviewPane+FitWindowToPane round trip (DECK_PREVIEW_MS is 300ms by
	// default, and a tmux round trip on a loaded host can exceed that)
	// both saw the selection as unsettled and both issued a fit for the
	// SAME session -- two resize-window calls, hence a second SIGWINCH the
	// scenarios that count them (features/preview.feature's "exactly 1")
	// never asked for. While this is non-empty previewFit issues nothing
	// at all, for any session: at most one passive fit is ever in flight,
	// so a later selection simply waits for the next tick after the
	// outstanding one reports. Safe against wedging because every return
	// path inside previewFit's closure owes -- and delivers -- a
	// previewFitDone; deliberately NOT cleared by exitInteractive, whose
	// clearing of previewFitSessionID must not license a second concurrent
	// fit for a session whose first attempt is still outstanding.
	previewFitInFlight string
	// sidebarScroll is the wheel-scroll offset into the sidebar's own
	// content lines (SPEC §11.8, task 028): it moves which lines the
	// panel shows without ever touching m.selected, and is clamped at
	// render/scroll time (never negative, never past the point where the
	// last line would leave the panel's bottom empty).
	sidebarScroll int
	// draggingSeam is true between a mouse press on the side-by-side
	// seam column and its matching release (SPEC §11.8): while true, a
	// motion event live-adjusts sidebarWidth the same way `<`/`>` do.
	draggingSeam bool
	// tmuxClient is the raw tmux.Client §11.9 interactive mode (task 061,
	// PRD Part II) uses directly, unlike every dependency above, which
	// wraps tmux behind a narrower func field instead (attach, kill,
	// resume, ...): window geometry, ownership, dispatcher construction
	// and the grid transport all take a live Client value as their own
	// parameter, so there is no narrower shape to wrap it in. The zero
	// Client (every constructor above, and every pre-task-061 unit test)
	// has an empty Socket; enterInteractive checks that explicitly and
	// degrades to "unavailable" before ever invoking tmux, exactly like
	// every nil func-field dependency already does.
	tmuxClient tmux.Client
	// interactive is true while §11.9's interactive mode owns the
	// keyboard (PRD II-41): the sidebar keeps rendering, but every
	// keystroke forwards to the live pane instead of driving list
	// navigation, until Ctrl+Q leaves (updateInteractive).
	interactive bool
	// interactiveWindowTarget is the deck_<slug> session name interactive
	// mode claimed ownership of and fit -- every geometry/ownership call
	// (CaptureWindowGeometry, FitWindowToPane, ClaimWindowOwnership,
	// RestoreWindowGeometry) addresses this window target, never the pane
	// id (which SendNamedKey/SendLiteral address instead, via
	// interactiveDispatcher).
	interactiveWindowTarget string
	// interactiveGeometry is the window's own geometry captured before
	// entering (PRD II-7), restored byte-exact on exit (PRD II-9/12).
	interactiveGeometry tmux.WindowGeometry
	// interactiveOwnership is the claimed @deck_isize_owner handle (PRD
	// II-14), released on exit.
	interactiveOwnership *tmux.WindowOwnership
	// interactiveGrid is the live transport (task 042 onward): one
	// long-lived x/vt emulator fed by pipe-pane, closed on exit.
	interactiveGrid *interactive.Session
	// interactiveDispatcher sends every forwarded keystroke (PRD II-28
	// onward): built against the pane id, re-verifying identity before
	// every send.
	interactiveDispatcher *tmux.Dispatcher
	// interactiveScrollOffset is PRD II-51's bounded scrollback position:
	// 0 is the live bottom (interactiveBodyLines shows exactly what
	// Grid().Render() itself would, unchanged from before task 068), and a
	// positive value is how many lines back into the grid's own bounded
	// scrollback (interactive.ScrollbackMaxLines) the view currently sits.
	// Reset to 0 on every entry (enterInteractive), every exit
	// (exitInteractive) and every keystroke forwarded to the target
	// (updateInteractive) -- scrolled-back history is read-only, so typing
	// or leaving snaps straight back to the live view.
	interactiveScrollOffset int
	// interactiveSelecting/interactiveSelectDragged/interactiveSelectAnchor*/
	// interactiveSelectCurrent* back the drag-to-copy selection SPEC §11.8
	// describes ("a drag beginning inside the preview selects, and
	// releasing copies") and steer 017 item 3/task 216 scope to §11.9's
	// interactive mode: interactiveSelecting is true from the press that
	// began inside the interactive preview's own content box (previewCellAt,
	// resolved through the SAME layout/hit-test math every other mouse
	// gesture in this package uses, never a second geometry computation)
	// until the matching release; interactiveSelectDragged only becomes
	// true once a MOTION event actually arrived while selecting, so a
	// plain click (press+release, no motion) commits nothing -- "a click
	// over the preview does nothing" (SPEC §11.8) stays true, and only an
	// actual drag is the stated exception. The anchor/current pair is kept
	// in previewCellAt's VIEW-relative (col, row) space (0 at the top-left
	// of the content box as currently rendered) rather than the grid's own
	// absolute row space, because that is the only space a hit-tested mouse
	// cell can be expressed in; commitInteractiveSelection converts both
	// through interactiveGrid.AbsoluteRow (keyed to the SAME
	// interactiveScrollOffset the drag was performed against) right before
	// extracting text, so a resize or a scroll racing the drag can never
	// silently select the wrong cells.
	interactiveSelecting        bool
	interactiveSelectDragged    bool
	interactiveSelectAnchorCol  int
	interactiveSelectAnchorRow  int
	interactiveSelectCurrentCol int
	interactiveSelectCurrentRow int
}

// WithTmuxClient attaches the tmux.Client §11.9 interactive mode (task
// 061 onward) uses directly. A Model built without it (every constructor
// above this method, and every pre-task-061 unit test) has the zero
// Client, so Enter simply cannot enter interactive mode -- see
// tmuxClient's own doc comment.
func (m Model) WithTmuxClient(client tmux.Client) Model {
	m.tmuxClient = client
	return m
}

// defaultAgentRegistry returns the stock shell/claude/pi registry used when a
// caller does not supply one, so every existing constructor keeps working
// unchanged.
func defaultAgentRegistry() *agent.Registry {
	r := agent.NewRegistry()
	r.Register(agent.NewShell())
	r.Register(agent.NewClaude())
	r.Register(agent.NewPi())
	return r
}

// defaultCreateAgent picks which registered kind the create modal opens on.
// "shell" is the only adapter that currently supports real session creation
// (see the createAgent != "shell" guard below), so it is preferred whenever
// present; otherwise the first kind in the registry's stable order is used.
// This keeps the modal's opening selection independent of where "shell"
// happens to sort alphabetically among registered kinds.
func defaultCreateAgent(kinds []string) string {
	for _, k := range kinds {
		if k == "shell" {
			return k
		}
	}
	if len(kinds) > 0 {
		return kinds[0]
	}
	return ""
}

// registry returns m.agents, falling back to defaultAgentRegistry() when the
// model was built without one (e.g. via New or any constructor that predates
// registry support).
func (m Model) registry() *agent.Registry {
	if m.agents != nil {
		return m.agents
	}
	return defaultAgentRegistry()
}

type sessionsLoaded struct {
	sessions []store.Session
	err      error
}

// Tick types are deliberately separate: reconciliation refreshes durable rows,
// while preview is a future-facing wake-up cadence that must not manufacture an
// out-of-scope preview pane. Animation is likewise optional and never runs
// when DECK_ANIM=0, keeping deterministic frames quiet.
type reconcileTick time.Time
type previewTick time.Time
type animationTick time.Time

type shellCreated struct {
	session store.Session
	err     error
}

type attachFinished struct{ err error }

// sessionKilled carries the killed session back (not just the error) so a
// successful x can start requirement 22's undo toast/window without a
// second store round-trip to learn which session and name it was.
type sessionKilled struct {
	session store.Session
	err     error
}

// undoExpired fires DECK_UNDO_MS after a successful kill; generation ties it
// to the specific undo window it was scheduled for, so an undo (u) or a
// later kill that has already moved on ignores a stale, already-superseded
// tick instead of clearing a newer toast out from under it.
type undoExpired int

type sessionAcknowledged struct{ err error }

// sessionDeleted carries task 105's dd submit result back: kill the live
// pane (if any) and tombstone the row (store.SoftDeleteSession). A
// successful delete reloads the session list so the now-tombstoned row
// disappears from the sidebar immediately rather than waiting for the
// next reconcile tick. session is carried back (task 106) so a successful
// delete can start the deleteUndoSessionID/Name toast and its
// DECK_DELETE_GRACE_MS window without a second store round-trip, exactly
// as sessionKilled already does for x's undo toast.
type sessionDeleted struct {
	session store.Session
	err     error
	// purgeErr is task 110's non-default purge choice's own outcome,
	// reported separately from err: a purge is only ever attempted after
	// the delete itself has already succeeded (err == nil), so a purge
	// failure never leaves the tombstone half-applied or the dialog stuck
	// open re-litigating a delete that already went through -- it
	// surfaces as a plain error banner instead (see the sessionDeleted
	// case in Update).
	purgeErr error
}

// sessionArchived carries task 111's `A` submit result back (SPEC
// requirement 27): kill the live pane first if one exists (offering
// "kill and archive" as one action rather than refusing), then set
// archived_at. A successful archive reloads the session list so the
// now-archived row disappears from the sidebar immediately, exactly as
// sessionDeleted already does for dd. session is the row as the confirm
// dialog named it -- i.e. BEFORE the archive's own kill step -- which is
// what lets the success branch start R72's undo window (SPEC.md:752's
// "a toast says what happened, with `u` to undo") and say whether a live
// agent was killed, without a second store round-trip.
type sessionArchived struct {
	session store.Session
	err     error
}

// archiveUndoExpired fires DECK_UNDO_MS after a successful `A` submit,
// mirroring undoExpired's generation-tying shape exactly (R72): a stale
// tick left over from an earlier archive/undo cycle that a newer archive or
// an intervening u has already superseded is ignored rather than clearing a
// newer toast. Nothing is reaped when it fires -- unlike
// deleteGraceExpired, an expired archive window leaves the archived row
// exactly where it is and only takes the toast away.
type archiveUndoExpired int

// deleteGraceExpired fires DECK_DELETE_GRACE_MS after a successful dd
// delete, mirroring undoExpired's generation-tying shape exactly (task
// 106): a stale tick left over from an earlier delete/undo cycle that a
// newer delete or an intervening u has already superseded is ignored
// rather than reaping the wrong row or clearing a newer toast.
type deleteGraceExpired int

// sessionsBulkKilled carries task 112's marked-set `x` result back: every
// marked, non-stopped session's own kill outcome, in markedSessions()'s
// own (visual) order. A successful entry starts ONE shared undo window
// covering the whole batch (batchUndoSessionIDs), never N separate
// single-session windows.
type sessionsBulkKilled struct {
	sessions []store.Session
	errs     []error
}

// batchUndoExpired mirrors undoExpired exactly, but for the batch window
// sessionsBulkKilled started.
type batchUndoExpired int

// sessionsBulkResumed carries a batch `u` undo's resume outcome back for
// every session batchUndoSessionIDs named.
type sessionsBulkResumed struct {
	errs []error
}

// sessionsBulkDeleted mirrors sessionsBulkKilled exactly, for a marked-set
// dd's bulk confirm submit: every marked session's own delete outcome, in
// markedSessions()'s own order. A successful entry starts ONE shared
// DECK_DELETE_GRACE_MS window (batchDeleteUndoSessionIDs) covering the
// whole batch. Purge is not offered for a bulk delete (task 110's purge
// choice resolves one session's one declared transcript path at a time;
// see docs/reports/phase3-findings.md), so there is no purgeErr here.
type sessionsBulkDeleted struct {
	sessions []store.Session
	errs     []error
}

// batchDeleteGraceExpired mirrors deleteGraceExpired exactly, but for the
// batch window sessionsBulkDeleted started.
type batchDeleteGraceExpired int

// sessionsBulkRestored carries a batch `u` undo's restore outcome back for
// every session batchDeleteUndoSessionIDs named.
type sessionsBulkRestored struct {
	errs []error
}

// sessionsBulkReaped carries every session batchDeleteGraceExpired reaped
// (or failed to reap) back, mirroring sessionReaped's shape for the batch
// case.
type sessionsBulkReaped struct {
	errs []error
}

// sessionRestored carries task 106's `u` undo-of-a-delete result back:
// store.RestoreSession clears the tombstone, and a successful restore
// reloads the session list so the row reappears in the sidebar
// immediately, mirroring sessionResumed's reload-on-success shape.
type sessionRestored struct {
	session store.Session
	err     error
}

// sessionUnarchived carries R71's `U` result back (SPEC.md:323-332, issue
// #8): store.UnarchiveSession cleared archived_at, and a successful
// unarchive reloads BOTH lists -- the default one the row has just
// rejoined and the `/` filter's archived-side pool it has just left -- so
// neither view keeps claiming the row is archived.
type sessionUnarchived struct {
	session store.Session
	err     error
}

// sessionReaped carries task 106's grace-window expiry result back: a
// failure is surfaced (the row was already gone from the default view, so
// there is nothing to reload) but is not silently swallowed.
type sessionReaped struct{ err error }

// previewCaptured reports one previewCapture tick's result for the session
// selected at the moment the capture was issued (SPEC requirements 21, 22).
// err is only ever a real tmux/transport failure; a session with no live
// pane is reported as capture.Live == false with err == nil, never as an
// error, so a stopped/starting/archived selection does not spam attachError
// every tick.
type previewCaptured struct {
	sessionID string
	capture   tmux.PreviewCapture
	err       error
}

// uiStatePersisted reports the outcome of persisting layout_mode or
// sidebar_width to state.db's ui_state table after a `|`/`<`/`>` keypress
// (task 016, SPEC §11.2). It never touches config.toml.
type uiStatePersisted struct{ err error }

// previewFitDone reports that one previewFit attempt for sessionID has
// completed (SPEC §11, steer 018 item 4) -- whether or not the underlying
// FitWindowToPane call actually issued a resize, and swallowing any error
// it returned: passive fit is stated as best-effort in the spec itself, so
// there is no error state to surface to the user, only a settled/
// unsettled one, which previewFitSessionID (set from sessionID here) is.
type previewFitDone struct{ sessionID string }

type sessionResumed struct {
	session store.Session
	outcome service.ResumeOutcome
	err     error
}

// sessionRestarted carries the result of task 022's `R` restart: a
// service.ResumeOutcome exactly like sessionResumed's, since a restart is
// implemented as kill-then-resume and shares the same outcome vocabulary
// (started, starting elsewhere, already running, not leasable).
type sessionRestarted struct {
	session store.Session
	outcome service.ResumeOutcome
	err     error
}

// envInjected carries the result of task 023's inject-instead alternative
// to Restart: the keys that were actually exported into the live shell
// pane (nil/empty when there was nothing dirty to inject, which is not an
// error), or the error that left the pane, env_dirty and the store exactly
// as they were before.
type envInjected struct {
	session store.Session
	keys    []string
	err     error
}

type profileSwitched struct {
	session store.Session
	err     error
}

type resumeModeChanged struct {
	session store.Session
	err     error
}

// sessionRenamed is the reply to a committed rename (task 013, SPEC §11.4,
// PRD requirement 31/I-8): Service.Rename's persisted result, or the error
// that kept the store's name column exactly as it was before -- the tmux
// session itself is never part of this round trip either way.
type sessionRenamed struct {
	session store.Session
	err     error
}

// envEdited is the reply to a committed `e` env-editor edit (task 021):
// Service.SetSessionEnv's persisted-and-mirrored result, or the error that
// kept the store/tmux state exactly as it was before the edit.
type envEdited struct {
	session store.Session
	err     error
}

// New creates a list model. tmux failures are intentionally retained as a
// rendered health state: users must be able to read and quit it.
func New(db *store.Store, settings config.Settings, tmuxNote string) Model {
	m := Model{store: db, settings: settings, startupNote: tmuxNote, createCWDRecentIndex: -1}
	if wd, err := os.Getwd(); err == nil {
		m.startCWD = wd
	}
	if db != nil {
		m.acknowledge = db.AcknowledgeSession
		m.prepareAttach = func(ctx context.Context, sessionID string) error {
			if settings.Clock == nil {
				return fmt.Errorf("attach status clock is unavailable")
			}
			return db.RecordAttachment(ctx, sessionID, settings.Clock.Now().UnixMilli())
		}
		// Task 016: layout_mode and sidebar_width (SPEC §11.2) live only in
		// state.db's ui_state table, read once here so a restarted client
		// (a fresh New(db, ...) call, exactly as acknowledge_test.go's
		// "restarted" model simulates it) renders the persisted pin/width
		// rather than falling back to the in-memory zero values. A read
		// failure is not load-bearing: ui_state degrades to the Go zero
		// value (""/0), which ComputeLayout already treats as auto/default.
		ctx := context.Background()
		if mode, err := db.GetLayoutMode(ctx); err == nil {
			m.layoutMode = mode
		}
		if width, err := db.GetSidebarWidth(ctx); err == nil {
			m.sidebarWidth = width
		}
	}
	return m
}

// NewWithShellCreator creates a list model that can create plain shell sessions.
// Keeping creation behind this small function lets the UI remain a renderer and
// makes failures, including slug collisions, visible rather than fatal.
func NewWithShellCreator(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error)) Model {
	return NewWithShellCreatorAndAttacher(db, settings, tmuxNote, creator, nil)
}

// NewWithShellCreatorAndAttacher creates a model with the two Phase 0 shell
// actions. The attacher returns an exec command so Bubble Tea can temporarily
// release the terminal while tmux owns it, then redraw the list on detach.
func NewWithShellCreatorAndAttacher(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error)) Model {
	return NewWithShellCreatorAttacherAndKiller(db, settings, tmuxNote, creator, attacher, nil)
}

// NewWithShellCreatorAttacherAndKiller creates a model with all implemented
// Phase 0 shell actions. Killing only tears down deck's private tmux session;
// the caller's cwd and its durable session row remain intact and resumable.
func NewWithShellCreatorAttacherAndKiller(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error) Model {
	return NewWithShellCreatorAttacherKillerAndReconciler(db, settings, tmuxNote, creator, attacher, killer, nil)
}

// NewWithShellCreatorAttacherKillerAndReconciler adds the released liveness
// pass to the list refresh. Running it immediately before loading rows means
// an external tmux disappearance reaches the visible client in one configured
// reconciliation cadence, rather than waiting a second refresh interval.
func NewWithShellCreatorAttacherKillerAndReconciler(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error) Model {
	return NewWithShellCreatorAttacherKillerReconcilerAndResumer(db, settings, tmuxNote, creator, attacher, killer, reconciler, nil)
}

// NewWithShellCreatorAttacherKillerReconcilerAndResumer adds the `r` resume
// action (SPEC §8/§9.3). A resumed row is left `starting`; the TUI never
// renders `running` for it, a caller that lost the launch-lease race
// (service.ResumeStartingElsewhere) is shown "starting elsewhere", and a
// caller whose tmux session already exists (service.ResumeAlreadyRunning,
// requirement 46) is shown "already running" — neither is an error.
func NewWithShellCreatorAttacherKillerReconcilerAndResumer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, nil)
}

// NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher adds the `P`
// profile-switch action (SPEC §5/§8, task 020). Switching a session's
// permission profile only ever persists the new value; it never touches a
// live pane, and the TUI states plainly that the change applies on the
// session's next launch/restart rather than implying the running agent's
// mode changed.
func NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, nil)
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer adds
// the `p` pin/start-fresh dialog (SPEC §8/§9.3, task 021). resumeModer picks
// resume_state ("pinned" pins to the session's own current conversation id,
// sticky across a deck restart; "fresh-once" arms exactly one fresh
// conversation launch and reverts to "auto" once that launch has actually
// happened; "auto" clears any pin). None of these ever touch a live pane;
// they only take effect on the session's next resume.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error)) Model {
	return NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, nil)
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator
// adds real coding-agent creation (task 022) to the create modal's Enter
// handler. Until agentCreator is wired, choosing claude/pi in the create
// modal keeps the pre-task-022 "not available yet" refusal (agentCreator ==
// nil); once wired, Enter on a non-shell Agent field calls
// service.CreateAgent with every other create-modal field (permission
// profile, launch_args, env, pre_launch, login_shell) exactly as typed.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error)) Model {
	m := New(db, settings, tmuxNote)
	m.create, m.attach, m.kill, m.reconcile, m.resume, m.profileSwitch, m.resumeMode, m.createAgentSession = creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry
// is identical to
// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator
// but additionally accepts the agent registry that drives the create
// modal's Agent field and capability lookups (SPEC requirement: adding an
// adapter must not require touching internal/tui). When registry is nil the
// model falls back to defaultAgentRegistry() (shell/claude/pi), so this is a
// pure superset of the existing constructor.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator)
	m.agents = registry
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryAndPreviewCapturer
// additionally wires the read-only preview capture engine (SPEC
// requirements 21, 22, task 017). previewCapturer is called with the
// selected row's slug once per DECK_PREVIEW_MS tick; tmux.Client.CapturePreview
// is the released implementation — a single capture-pane per tick, no
// client attach, no control-mode server, no pipe-pane, no resize. nil keeps
// the preview exactly as inert as every constructor above this one leaves
// it.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryAndPreviewCapturer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry)
	m.previewCapture = previewCapturer
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerAndEnvSetter
// adds the `e` env editor's write path (task 021, SPEC §6.1/§6.3):
// envSetter persists one session-env key/value edit (env_dirty) and
// mirrors it into the live tmux pane's environment table for future panes
// only, never the pane's already-running process. Until envSetter is
// wired, committing an edit in the `e` dialog fails with "editing the
// environment is unavailable" rather than silently doing nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerAndEnvSetter(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryAndPreviewCapturer(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer)
	m.setSessionEnv = envSetter
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterAndRestarter
// adds the `R` restart action (task 022, SPEC §6.2/§6.3): restarter kills
// the selected session's live pane if one exists and relaunches it with
// the adapter's resume argv (the SAME conversation id -- never a fresh
// one), carrying whatever environment is currently persisted on the row,
// including a pending `e`-editor edit; on success it also clears
// env_dirty (the `env↻` badge), since only this explicit, user-driven
// restart ever applies a pending edit to an already-running process. Until
// restarter is wired, `R` fails with "restarting is unavailable" rather
// than silently doing nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterAndRestarter(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerAndEnvSetter(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter)
	m.restart = restarter
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterAndInjector
// adds task 023's `R` inject-instead alternative for shell sessions:
// injector exports the env keys changed since env_dirty was last cleared
// directly into the selected session's live, already-running shell pane
// (never killing or relaunching it), then clears env_dirty exactly as a
// successful restart does. Until injector is wired, choosing "inject" in
// the `R` choice dialog fails with "injecting the environment is
// unavailable" rather than silently doing nothing; the dialog itself is
// only ever offered for a shell session (Restart, above, is unaffected).
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterAndInjector(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterAndRestarter(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter)
	m.inject = injector
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorAndDeleter
// adds task 105's `dd` delete action (SPEC §9/§11.4): deleter kills the
// selected session's live pane if one exists and tombstones the row
// (store.SoftDeleteSession) so it disappears from ListSessions
// immediately, restorable within task 106's grace window. It never
// touches the session's cwd or its conversation id/transcript. Until
// deleter is wired, submitting the confirm dialog fails with "deleting is
// unavailable" rather than silently doing nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorAndDeleter(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterAndInjector(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector)
	m.deleteSvc = deleter
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerAndReaper
// adds task 106's grace-window half of `dd` (SPEC §9.2, requirements 2/23):
// restorer clears a tombstone (store.RestoreSession) so u can undo a
// completed delete within DECK_DELETE_GRACE_MS, and reaper permanently
// removes a tombstoned row (store.ReapSession) once that window's own
// tea.Tick fires without an intervening u. Both windows are scheduled
// against a real tea.Tick, not m.settings.Clock, so they keep advancing
// even while DECK_CLOCK is frozen -- exactly like DECK_UNDO_MS's own
// undoExpired tick already does for x (task 102). Until restorer/reaper
// are wired, u is a no-op for a deleted session and the grace-window tick
// clears the toast without ever reaping the row.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerAndReaper(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error, restorer func(context.Context, string) (store.Session, error), reaper func(context.Context, string) error) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorAndDeleter(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector, deleter)
	m.restoreSvc = restorer
	m.reapSvc = reaper
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperAndPurger
// adds task 110's non-default "purge conversation" choice inside the dd
// confirm dialog (SPEC.md:684-691, requirement 26): purger deletes exactly
// the path the dialog itself already resolved via the session's own
// adapter's declared TranscriptPaths (task 109) -- it is never called with
// a guessed or inferred path, and never with an empty one. Until purger is
// wired, choosing "purge" and submitting leaves the tombstone applied (as
// dd always has) but reports "purging the transcript is unavailable"
// rather than silently deleting nothing while claiming success.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperAndPurger(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error, restorer func(context.Context, string) (store.Session, error), reaper func(context.Context, string) error, purger func(context.Context, string) error) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerAndReaper(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector, deleter, restorer, reaper)
	m.purgeSvc = purger
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerAndArchiver
// adds task 111's `A` (SPEC requirement 27): archiver kills the selected
// session's live pane first if one exists (offering "kill and archive" as
// one action rather than refusing) and then sets archived_at
// (store.ArchiveSession) -- a flag, never a status. Until archiver is
// wired, A reports "archiving is unavailable" rather than silently doing
// nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerAndArchiver(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error, restorer func(context.Context, string) (store.Session, error), reaper func(context.Context, string) error, purger func(context.Context, string) error, archiver func(context.Context, store.Session) error) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperAndPurger(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector, deleter, restorer, reaper, purger)
	m.archiveSvc = archiver
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer
// adds task 013's rename action (SPEC §11.4, PRD requirement 31, I-8),
// reachable ONLY from inside the `i` detail dialog, never as a top-level
// key: renamer persists a new display name (service.Rename/
// store.RenameSession) and never touches the session's live tmux session
// -- deck's display name and tmux's own session name are decoupled by
// design, and the dialog states so on screen. Until renamer is wired, the
// rename sub-dialog reports "renaming is unavailable" rather than silently
// doing nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error, restorer func(context.Context, string) (store.Session, error), reaper func(context.Context, string) error, purger func(context.Context, string) error, archiver func(context.Context, store.Session) error, renamer func(context.Context, string, string) (store.Session, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerAndArchiver(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector, deleter, restorer, reaper, purger, archiver)
	m.renamer = renamer
	return m
}

// NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverRenamerAndUnarchiver
// adds R71's `U` (SPEC.md:323-332, issue #8): unarchiver clears archived_at
// (service.Unarchive/store.UnarchiveSession) so a row hidden by `A`
// returns to the default list. It is the promised way out that resume's
// own archived-row refusal names, and it is reached through requirement
// 33's `/` filter, the only place an archived row is displayed at all.
// Until unarchiver is wired, `U` reports "unarchiving is unavailable"
// rather than silently doing nothing.
func NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverRenamerAndUnarchiver(db *store.Store, settings config.Settings, tmuxNote string, creator func(context.Context, service.ShellCreateInput) (store.Session, error), attacher func(context.Context, string) (*exec.Cmd, error), killer func(context.Context, store.Session) error, reconciler func(context.Context) error, resumer func(context.Context, string) (store.Session, service.ResumeOutcome, error), profileSwitcher func(context.Context, string, string) (store.Session, error), resumeModer func(context.Context, string, string) (store.Session, error), agentCreator func(context.Context, service.AgentCreateInput) (store.Session, error), registry *agent.Registry, previewCapturer func(context.Context, string) (tmux.PreviewCapture, error), envSetter func(context.Context, string, string, string) (store.Session, error), restarter func(context.Context, string) (store.Session, service.ResumeOutcome, error), injector func(context.Context, string) (store.Session, []string, error), deleter func(context.Context, store.Session) error, restorer func(context.Context, string) (store.Session, error), reaper func(context.Context, string) error, purger func(context.Context, string) error, archiver func(context.Context, store.Session) error, renamer func(context.Context, string, string) (store.Session, error), unarchiver func(context.Context, string) (store.Session, error)) Model {
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorRegistryPreviewCapturerEnvSetterRestarterInjectorDeleterRestorerReaperPurgerArchiverAndRenamer(db, settings, tmuxNote, creator, attacher, killer, reconciler, resumer, profileSwitcher, resumeModer, agentCreator, registry, previewCapturer, envSetter, restarter, injector, deleter, restorer, reaper, purger, archiver, renamer)
	m.unarchiveSvc = unarchiver
	return m
}

func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{
		m.loadSessions,
		tea.Tick(m.settings.Reconcile, func(t time.Time) tea.Msg { return reconcileTick(t) }),
		tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return previewTick(t) }),
	}
	if m.settings.Animation {
		commands = append(commands, tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return animationTick(t) }))
	}
	return tea.Batch(commands...)
}

func (m Model) loadSessions() tea.Msg {
	if m.store == nil {
		return sessionsLoaded{}
	}
	rows, err := m.store.ListSessions(context.Background())
	return sessionsLoaded{sessions: rows, err: err}
}

// archivedSessionsLoaded carries task 123's fresh ListArchivedSessions
// result back into Update (SPEC requirement 33/I-10): loadArchivedSessions
// is issued every time `/` opens, so the filter's archived-side search
// pool is at most one keypress stale.
type archivedSessionsLoaded struct {
	sessions []store.Session
	err      error
}

// loadArchivedSessions is the `/` filter's own fetch of requirement 33's
// ONLY route back to an archived row: store.ListArchivedSessions, mirroring
// loadSessions' own no-store-attached degrade (an empty result, never a
// panic).
func (m Model) loadArchivedSessions() tea.Msg {
	if m.store == nil {
		return archivedSessionsLoaded{}
	}
	rows, err := m.store.ListArchivedSessions(context.Background())
	return archivedSessionsLoaded{sessions: rows, err: err}
}

// capturePreview issues exactly one read-only capture-pane for the
// currently selected row (SPEC requirement 21, task 017), or nil when there
// is no engine wired, no row to capture, or the preview panel is not shown
// this frame (requirement 27: no capture tick runs while the preview is
// below its floor and the sidebar has taken its space — task 021). The
// command captures the slug selected at the moment the tick fired, not
// whatever is selected when the reply arrives, so a selection change
// mid-flight cannot mislabel a frame.
func (m Model) capturePreview() tea.Cmd {
	if m.previewCapture == nil || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return nil
	}
	if !m.computeLayout().PreviewShown {
		return nil
	}
	session := m.sessions[m.selected]
	capture := m.previewCapture
	return func() tea.Msg {
		result, err := capture(context.Background(), session.Slug)
		return previewCaptured{sessionID: session.ID, capture: result, err: err}
	}
}

// previewFit is steer 018 item 4's passive-preview fit (SPEC §11: "The
// preview fits the selected session's window to the panel"), issued
// alongside capturePreview on every previewTick. It returns nil -- issuing
// no tmux call at all -- whenever any of the properties SPEC §11 states
// are not met:
//
//   - [ui] preview_fit is off, there is no tmux client wired, no row is
//     selected, or the preview panel is not shown this frame (the same
//     guards capturePreview itself already applies);
//   - m.interactive is true: interactive mode OWNS the window's geometry
//     (claimed, recorded, restored on exit) and runs its own
//     FitWindowToPane at entry -- passive fit must never race that, or fit
//     a window a moment before/after ClaimWindowOwnership/
//     RestoreWindowGeometry touches the very same target;
//   - a passive fit is already in flight (m.previewFitInFlight non-empty):
//     previewFitSessionID is only written when an attempt COMPLETES, so
//     without this second half of the guard two ticks inside one tmux
//     round trip both issue a fit for the same still-unsettled selection
//     and the window is resized twice (task R63);
//   - the selected session's ID already matches m.previewFitSessionID:
//     this selection has already settled and been fit (or found not
//     applicable), so nothing re-issues a fit merely because the panel's
//     OWN geometry changed under a sidebar-width/layout-mode/terminal-
//     resize gesture with the selection unchanged (steer 018 §1b's
//     amendment to the passive-preview no-resize guarantee is scoped to
//     fitting-on-navigation only -- see previewFitSessionID's own doc
//     comment and docs/reports/phase3d-215-preview-fit-on-nav.md);
//   - the preview content box is below interactiveMinInnerRows (SPEC
//     §11's "skipped below §11.9's 7-inner-row floor") or has no width:
//     a box that small is left cropped, exactly like interactive mode's
//     own entry refusal for the same floor, and never causes deck to enter
//     interactive mode as a side effect of navigating (this function never
//     starts a transport or claims ownership, only resize-window).
//
// Unlike enterInteractive's own attached-client refusal (case 1), passive
// fit does NOT check SessionAttachedCount at all: SPEC §11.9 states that
// refusal exists because interactive mode "needs a held size for its grid
// to stay correct", a property passive fit does not have -- it is
// best-effort, and "any attaching client re-expresses its own size under
// window-size latest and simply wins" is the stated, accepted outcome, not
// a case to refuse.
//
// The receiver is a POINTER because scheduling a fit is itself a state
// change: previewFitInFlight has to be marked before the command is handed
// to the event loop, or the very next tick can slip past the guard.
func (m *Model) previewFit() tea.Cmd {
	if !m.settings.PreviewFit || m.interactive || m.tmuxClient.Socket == "" {
		return nil
	}
	if len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return nil
	}
	if !m.computeLayout().PreviewShown {
		return nil
	}
	session := m.sessions[m.selected]
	if session.ID == m.previewFitSessionID || m.previewFitInFlight != "" {
		return nil
	}
	width, height := m.previewContentSize()
	if width <= 0 || height < interactiveMinInnerRows {
		return nil
	}
	client := m.tmuxClient
	slug := session.Slug
	sessionID := session.ID
	// Marked here, at SCHEDULING time, so a tick that fires while this
	// command is still running finds the guard closed. EVERY early return
	// inside the closure below therefore OWES a previewFitDone for this
	// same sessionID -- that message is the only thing that clears the
	// marker, and a path that returns anything else (or nil) wedges passive
	// fit for the rest of the session's lifetime.
	m.previewFitInFlight = sessionID
	return func() tea.Msg {
		ctx := context.Background()
		pane, ok, err := client.PreviewPane(ctx, slug)
		if err != nil || !ok {
			// No live pane (stopped/starting/archived/reaped) or a read
			// error: nothing to fit, and nothing to retry until the
			// selection changes again -- the same best-effort treatment
			// capturePreview's own previewCaptured{err} gives a transport
			// failure (it does not spam attachError every tick either).
			return previewFitDone{sessionID: sessionID}
		}
		windowTarget, err := tmux.SessionName(slug)
		if err != nil {
			return previewFitDone{sessionID: sessionID}
		}
		// FitWindowToPane already no-ops (0 resize-window calls) when the
		// pane already matches width/height, so a settled selection whose
		// window some other action (an attaching client, a prior fit) has
		// already brought to the panel's own size costs nothing beyond the
		// one display-message read that discovers that.
		_, _ = client.FitWindowToPane(ctx, windowTarget, pane.ID, width, height)
		return previewFitDone{sessionID: sessionID}
	}
}

// Task 012: one eligibility predicate per action, each taking the
// candidate session and nothing else, so the key cases below (and any
// batch/marked-set variant of the same action) share exactly one place
// that decides whether the action applies to a given row instead of
// deciding it inline in the case body. Where the switch case currently
// has no per-session refusal (Y/A/dd today act on any selected row; the
// service, not this switch, is the authority), the predicate is
// unconditionally true -- it still exists so a future restriction has
// one place to land rather than being added ad hoc to the case body.

// canAcknowledge reports whether Y may act on session. AcknowledgeSession
// carries no per-session restriction, so this is always true.
func canAcknowledge(session store.Session) bool {
	return true
}

// canKill reports whether x may act on session. Used by the batch (marked
// set) path, which silently skips an already-stopped row rather than
// erroring (a batch action naming rows that need nothing done would be
// noise). The single-row path deliberately does NOT call this: it defers
// the already-stopped refusal to the service's own verdict instead of a
// locally read Status, because remain-on-exit can leave a stopped row
// with a live tmux pane still needing a kill (#6) -- see the comment on
// case "x" below.
func canKill(session store.Session) bool {
	return session.Status != "stopped"
}

// canArchive reports whether A may act on session. Archiving accepts any
// row (a live one is killed first, in the same action), so this is
// always true.
func canArchive(session store.Session) bool {
	return true
}

// canUnarchive reports whether U may act on session: only a row that is
// actually archived.
func canUnarchive(session store.Session) bool {
	return session.ArchivedAt != 0
}

// footerArchiveEligible reports whether the footer's curated A/U slot
// (task 014, SPEC §11.3: "the eligible one of A/U") should show A for
// session: only a row that is not already archived, mirroring
// canUnarchive's complement so the two never both show for the same row.
// It is footer-only -- canArchive above answers a different question (may
// the A key act on this row at all, which it always does) and stays
// unconditionally true for the key handler; §11.3's "A on a row that is
// already archived" refusal is a footer-display rule, not a change to
// what pressing A itself does.
func footerArchiveEligible(session store.Session) bool {
	return session.ArchivedAt == 0
}

// canResume reports whether r may act on session: only a stopped row.
func canResume(session store.Session) bool {
	return session.Status == "stopped"
}

// canRestart reports whether R may act on session: any row that is not
// already stopped (r resumes a stopped one instead).
func canRestart(session store.Session) bool {
	return session.Status != "stopped"
}

// canDelete reports whether dd may act on session. Deleting accepts any
// row, live or archived (the live pane is killed first, in the same
// action), so this is always true.
func canDelete(session store.Session) bool {
	return true
}

// canReachPane reports whether the two pane-reaching keys -- `↵` (enter
// interactive mode) and `a` (attach) -- may act on session. Both handlers
// (enterInteractive, attachSelected) refuse a stopped row with the same
// "resume it first" message, because a stopped session has no pane to
// reach at all, so that one session-level rule lives here and both of
// them plus the footer (task 013) read it from the same place. The
// refusals that are NOT properties of the selection -- no tmux socket, a
// preview box below interactiveMinInnerRows, a bystander already attached
// to the window -- deliberately stay in enterInteractive: they are
// properties of the terminal and of the tmux server, and §11.3's
// eligibility question is asked of the selection.
func canReachPane(session store.Session) bool {
	return session.Status != "stopped"
}

// canShowDetail reports whether i may act on session: the detail dialog
// describes any row there is, live, stopped or archived, so its handler's
// only refusal is an empty list -- which footerRowEligible answers before
// this is ever consulted.
func canShowDetail(session store.Session) bool {
	return true
}

// canSwitchProfile reports whether P may act on session: only an agent
// that has a permission profile at all (SPEC §5/§8 -- a shell session has
// none, and case "P" says exactly that when asked). It hangs off Model
// rather than being a bare function because the answer comes from
// m.agentCapabilities, which is configuration, not row state.
func (m Model) canSwitchProfile(session store.Session) bool {
	_, applicable := m.agentCapabilities(session.Agent)
	return applicable
}

// canPinResume reports whether p may act on session: only an agent whose
// adapter assigns a conversation id, since pinning is a statement about
// which conversation the next launch resumes.
func (m Model) canPinResume(session store.Session) bool {
	caps, applicable := m.agentCapabilities(session.Agent)
	return applicable && caps.AssignsConversationID
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	i1Trace("enter", message, m.selected)
	i1TraceSessions(m)
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// PRD Part II requirement 48/task 204 (review finding F3's second
		// half): the floor check in enterInteractive only ever ran AT
		// ENTRY. previewTitle/previewContentSize recompute the preview
		// box's inner size on every render, so a terminal shrink WHILE
		// already interactive was never re-checked against
		// interactiveMinInnerRows at all -- deck stayed interactive and
		// simply rendered into a box below the measured floor, the same
		// degrade-instead-of-refuse defect 201/203 fixed for entry.
		// Checked here, right after m.width/m.height take the new size and
		// before anything else looks at them, using the exact same
		// previewContentSize arithmetic and interactiveMinInnerRows
		// constant enterInteractive's own floor check uses, so the two can
		// never disagree about what counts as below the floor.
		if m.interactive {
			if width, height := m.previewContentSize(); width <= 0 || height < interactiveMinInnerRows {
				next, cmd := m.exitInteractive()
				m = next.(Model)
				m.attachError = fmt.Sprintf("Left interactive mode: preview panel shrank to %d inner rows, fewer than the %d-row floor; press a to attach instead", height, interactiveMinInnerRows)
				return m, cmd
			}
		}
	case sessionsLoaded:
		if msg.err != nil {
			m.startupNote = "Cannot read sessions: " + msg.err.Error()
		} else {
			// SPEC requirements 28/29/30 (task 023's sort, task 024's
			// grouping): every load renders in attention order, not
			// store order, so the sidebar's group order itself follows
			// each group's most urgent member (see SPEC §11's
			// illustration: "service-a" leads with two waiting rows,
			// "infra" follows with only an error) exactly as
			// groupSessions' own "first appearance in m.sessions"
			// bucketing already promises once m.sessions is in this
			// order.
			var selectedID string
			if m.selected >= 0 && m.selected < len(m.sessions) {
				selectedID = m.sessions[m.selected].ID
			}
			// sortSessionsByAttentionStable, not the plain
			// sortSessionsByAttention: a genuine tie on both rank and
			// StatusAt (task 005/I-1's finding -- two co-created sessions
			// promoted running in the same reconcile pass round to the
			// same millisecond) must not silently swap two rows' relative
			// order out from under an in-flight k/m/j idiom just because
			// their random UUIDs happen to compare the "wrong" way; see
			// sortSessionsByAttentionStable's own doc comment
			// (internal/tui/attention.go) and
			// docs/reports/phase3d-i1-rootcause.md.
			//
			// Task 123/I-10: sorted into baseSessions, never m.sessions
			// directly, so a filter query in force survives the periodic
			// reconcile tick's own reload instead of being silently
			// clobbered by it the moment ListSessions' own (archive-free)
			// result lands.
			//
			// Task 305 (R53): attentionOrder is computed unconditionally
			// -- not only when the configured order IS attention -- because
			// it also supplies workspace GROUP order for a non-attention
			// render (see reorderPreservingGrouping's doc comment): SPEC
			// requirement 30's "group order follows each group's most
			// urgent member" holds regardless of which order the ROWS
			// within a group render in (task 309's own success criteria:
			// "group order itself unchanged from today's behavior").
			order, _ := m.effectiveSortOrder()
			attentionOrder := sortSessionsByAttentionStable(m.baseSessions, msg.sessions)
			switch {
			case order == SortOrderAttention:
				m.baseSessions = attentionOrder
			case m.groupingEnabled():
				m.baseSessions = reorderPreservingGrouping(attentionOrder, sortSessionsByOrder(m.baseSessions, msg.sessions, order))
			default:
				m.baseSessions = sortSessionsByOrder(m.baseSessions, msg.sessions, order)
			}
			m.sessions = m.filteredSessions()
			if idx := indexOfSessionID(m.sessions, selectedID); idx >= 0 {
				m.selected = idx
			} else if m.selected >= len(m.sessions) {
				m.selected = max(0, len(m.sessions)-1)
			}
			// Requirement 52: the one-shot new-session intent (see
			// pendingSelectSessionID's doc comment) overrides the
			// preserved-selection result above whenever the id it is
			// waiting for has actually arrived in the FILTERED list --
			// never m.baseSessions -- so a live filter query that hides
			// the new session leaves the selection (and the query)
			// untouched instead of yanking the view to a row the filter
			// itself is hiding.
			if m.pendingSelectSessionID != "" {
				if idx := indexOfSessionID(m.sessions, m.pendingSelectSessionID); idx >= 0 {
					m.selected = idx
					m.pendingSelectSessionID = ""
					m.scrollSessionIntoView(idx)
				}
			}
		}
	case archivedSessionsLoaded:
		// Task 123/I-10: refreshes the filter's archived-side search pool.
		// A failed fetch leaves the previous pool in place rather than
		// wiping it to empty, matching sessionsLoaded's own
		// error-preserves-the-last-good-frame convention elsewhere in
		// this switch.
		if msg.err == nil {
			m.archivedSessions = msg.sessions
		}
		m.sessions = m.filteredSessions()
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
	case eventLogLoaded:
		// R61 (steer 3e-001 §6.3): the ONE place loadEventLog's result is
		// consumed. m.eventLogRows/m.eventLogErr are what eventLogBody
		// renders; a stale reply arriving after Esc already closed the
		// dialog is harmless (the fields are simply unused until the next
		// "E" reopens and resets them again).
		m.eventLogRows = msg.events
		m.eventLogErr = msg.err
	case shellCreated:
		if msg.err != nil {
			m.createError = msg.err.Error()
			return m, nil
		}
		m.creating, m.createError = false, ""
		// Requirement 52: record the freshly created session's id as a
		// one-shot select-this-row intent (see pendingSelectSessionID's
		// doc comment) -- it does not exist in m.sessions yet, so the
		// selection itself is applied once loadSessions' own
		// sessionsLoaded actually contains it, never here.
		m.pendingSelectSessionID = msg.session.ID
		return m, m.loadSessions
	case attachFinished:
		if msg.err != nil {
			m.attachError = "Cannot attach: " + msg.err.Error()
		}
		// Steer 005: tea.ExecProcess (attachSelected, above) brackets the real
		// tmux client's attach with bubbletea's own ReleaseTerminal/
		// RestoreTerminal (ExecProcess doc, tea.go:184-188), which restores only
		// altScreenWasActive/bpWasActive/reportFocus -- there is no
		// mouseWasActive field anywhere in bubbletea v1, so mouse reporting
		// enabled once at startup via tea.WithMouseCellMotion() (cmd/deck/main.go)
		// never comes back after tmux's own DECRST-on-detach turns it off. Emit
		// the same tea.EnableMouseCellMotion the startup ProgramOption would have
		// enabled, gated by the SAME m.settings.Mouse condition so a user who
		// asked for mouse off ([ui] mouse=false / DECK_MOUSE=0) does not get it
		// silently switched back on by one attach/detach cycle.
		if m.settings.Mouse {
			return m, tea.Batch(m.loadSessions, tea.EnableMouseCellMotion)
		}
		return m, m.loadSessions
	case sessionKilled:
		if msg.err != nil {
			m.attachError = "Cannot kill: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		m.undoSessionID = msg.session.ID
		m.undoSessionName = msg.session.Name
		m.undoGeneration++
		generation := m.undoGeneration
		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.Undo, func(t time.Time) tea.Msg { return undoExpired(generation) }))
	case undoExpired:
		if int(msg) == m.undoGeneration {
			m.undoSessionID, m.undoSessionName = "", ""
		}
		return m, nil
	case sessionAcknowledged:
		if msg.err != nil {
			m.attachError = "Cannot acknowledge: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		return m, m.loadSessions
	case sessionArchived:
		if msg.err != nil {
			// R72: a failed submit stays in the dialog and says why, exactly as
			// sessionDeleted's own failure does with deleteNote -- closing the
			// dialog on failure would leave the operator with a vanished dialog
			// and an unarchived row.
			m.archiveNote = "Cannot archive: " + msg.err.Error()
			m.attachError = "Cannot archive: " + msg.err.Error()
			return m, nil
		}
		m.archiveConfirming = false
		m.archiveNote = ""
		m.attachError = ""
		// R72 (SPEC.md:752): a successful archive says what happened and offers
		// `u`, on its own DECK_UNDO_MS window. msg.session is the row the dialog
		// named, captured before the archive's kill step, so its status here is
		// the pre-archive one -- "not stopped" is exactly the case where the
		// archive also killed a live agent, which is what the toast reports.
		m.archiveUndoSessionID = msg.session.ID
		m.archiveUndoSessionName = msg.session.Name
		m.archiveUndoKilled = msg.session.Status != "stopped"
		m.archiveUndoGeneration++
		archiveGeneration := m.archiveUndoGeneration
		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.Undo, func(t time.Time) tea.Msg { return archiveUndoExpired(archiveGeneration) }))
	case archiveUndoExpired:
		if int(msg) == m.archiveUndoGeneration {
			m.archiveUndoSessionID, m.archiveUndoSessionName = "", ""
			m.archiveUndoKilled = false
		}
		return m, nil
	case sessionDeleted:
		if msg.err != nil {
			m.deleteNote = "Cannot delete: " + msg.err.Error()
			return m, nil
		}
		m.deleteConfirming = false
		m.deleteNote = ""
		m.deletePurgeValue = ""
		m.deletePurgePath = ""
		m.deletePurgeOK = false
		if msg.purgeErr != nil {
			m.attachError = "Deleted, but purge failed: " + msg.purgeErr.Error()
		} else {
			m.attachError = ""
		}
		m.deleteUndoSessionID = msg.session.ID
		m.deleteUndoSessionName = msg.session.Name
		m.deleteUndoGeneration++
		generation := m.deleteUndoGeneration
		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.DeleteGrace, func(t time.Time) tea.Msg { return deleteGraceExpired(generation) }))
	case deleteGraceExpired:
		if int(msg) != m.deleteUndoGeneration || m.deleteUndoSessionID == "" {
			return m, nil
		}
		sessionID := m.deleteUndoSessionID
		m.deleteUndoSessionID, m.deleteUndoSessionName = "", ""
		if m.reapSvc == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			return sessionReaped{err: m.reapSvc(context.Background(), sessionID)}
		}
	case sessionRestored:
		if msg.err != nil {
			m.attachError = "Cannot restore: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		return m, m.loadSessions
	case sessionUnarchived:
		if msg.err != nil {
			m.attachError = "Cannot unarchive: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		// Both loads, not just loadSessions: the row is in m.sessions only
		// because m.archivedSessions still holds it (requirement 33's
		// filter pool), so refreshing the default list alone would leave a
		// stale archived copy behind it -- filteredSessions de-duplicates
		// by id, keeping the fresh baseSessions row, and the archived pool
		// drops it on its own reload.
		return m, tea.Batch(m.loadSessions, m.loadArchivedSessions)
	case sessionReaped:
		if msg.err != nil {
			m.attachError = "Cannot reap: " + msg.err.Error()
		}
		return m, nil
	case sessionsBulkKilled:
		var succeeded []string
		var firstErr error
		for i, s := range msg.sessions {
			if msg.errs[i] != nil {
				if firstErr == nil {
					firstErr = msg.errs[i]
				}
				continue
			}
			succeeded = append(succeeded, s.ID)
		}
		if firstErr != nil {
			m.attachError = "Cannot kill: " + firstErr.Error()
		} else {
			m.attachError = ""
		}
		if len(succeeded) == 0 {
			return m, m.loadSessions
		}
		m.batchUndoSessionIDs = succeeded
		m.batchUndoGeneration++
		generation := m.batchUndoGeneration
		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.Undo, func(t time.Time) tea.Msg { return batchUndoExpired(generation) }))
	case batchUndoExpired:
		if int(msg) == m.batchUndoGeneration {
			m.batchUndoSessionIDs = nil
		}
		return m, nil
	case sessionsBulkResumed:
		for _, err := range msg.errs {
			if err != nil {
				m.attachError = "Cannot resume: " + err.Error()
				return m, m.loadSessions
			}
		}
		m.attachError = ""
		return m, m.loadSessions
	case sessionsBulkDeleted:
		var succeeded []string
		var firstErr error
		for i, s := range msg.sessions {
			if msg.errs[i] != nil {
				if firstErr == nil {
					firstErr = msg.errs[i]
				}
				continue
			}
			succeeded = append(succeeded, s.ID)
		}
		m.deleteConfirming = false
		m.deleteNote = ""
		if firstErr != nil {
			m.attachError = "Cannot delete: " + firstErr.Error()
		} else {
			m.attachError = ""
		}
		if len(succeeded) == 0 {
			return m, m.loadSessions
		}
		m.batchDeleteUndoSessionIDs = succeeded
		m.batchDeleteUndoGeneration++
		generation := m.batchDeleteUndoGeneration
		return m, tea.Batch(m.loadSessions, tea.Tick(m.settings.DeleteGrace, func(t time.Time) tea.Msg { return batchDeleteGraceExpired(generation) }))
	case batchDeleteGraceExpired:
		if int(msg) != m.batchDeleteUndoGeneration || len(m.batchDeleteUndoSessionIDs) == 0 {
			return m, nil
		}
		ids := m.batchDeleteUndoSessionIDs
		m.batchDeleteUndoSessionIDs = nil
		if m.reapSvc == nil {
			return m, nil
		}
		reapSvc := m.reapSvc
		return m, func() tea.Msg {
			result := sessionsBulkReaped{}
			for _, id := range ids {
				result.errs = append(result.errs, reapSvc(context.Background(), id))
			}
			return result
		}
	case sessionsBulkReaped:
		for _, err := range msg.errs {
			if err != nil {
				m.attachError = "Cannot reap: " + err.Error()
				return m, nil
			}
		}
		return m, nil
	case sessionsBulkRestored:
		for _, err := range msg.errs {
			if err != nil {
				m.attachError = "Cannot restore: " + err.Error()
				return m, m.loadSessions
			}
		}
		m.attachError = ""
		return m, m.loadSessions
	case uiStatePersisted:
		// A failed write to ui_state is not load-bearing (SPEC §11.2): the
		// pin/width already changed in memory and keeps rendering; only the
		// error note surfaces so a persistent failure is still visible.
		if msg.err != nil {
			m.attachError = "Cannot persist layout: " + msg.err.Error()
		}
		return m, nil
	case sessionResumed:
		if msg.err != nil {
			m.attachError = "Cannot resume: " + msg.err.Error()
			m.resumeNote = ""
			return m, nil
		}
		m.attachError = ""
		if msg.outcome == service.ResumeStartingElsewhere {
			m.resumeNote = "starting elsewhere"
			return m, nil
		}
		if msg.outcome == service.ResumeAlreadyRunning {
			// Requirement 46: deck already owns this pane. Adopting it as an
			// honest no-op means refreshing the row from whatever the service
			// returned (untouched) rather than pretending a launch happened.
			m.resumeNote = "already running"
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
		m.resumeNote = ""
		if msg.outcome == service.ResumeNotLeasable {
			// The resume command was dispatched from a stale stopped frame.
			// Render the durable status/reason returned by the service rather
			// than describing it as a launch in another client.
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
		return m, m.loadSessions
	case sessionRestarted:
		if msg.err != nil {
			m.attachError = "Cannot restart: " + msg.err.Error()
			return m, nil
		}
		m.attachError = ""
		if msg.outcome == service.ResumeStartingElsewhere {
			m.attachError = "Cannot restart: a launch for this session is already starting elsewhere"
			return m, nil
		}
		if msg.outcome == service.ResumeAlreadyRunning {
			// Requirement 46's already-running honest no-op applies here too:
			// a concurrent client may have already relaunched the pane between
			// this restart's kill and its own resume attempt.
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
		if msg.outcome == service.ResumeNotLeasable {
			for i := range m.sessions {
				if m.sessions[i].ID == msg.session.ID {
					m.sessions[i] = msg.session
					break
				}
			}
			return m, nil
		}
		// A successful restart also closes task 023's restart/inject-instead
		// choice dialog, if that is how this restart was chosen -- a no-op
		// when R restarted directly (non-shell session, no dialog ever
		// opened).
		m.restartChoosing = false
		m.restartChoiceNote = ""
		return m, m.loadSessions
	case envInjected:
		// Task 023's inject-instead: unlike Restart, nothing was killed or
		// relaunched, so a failure leaves the choice dialog open with a note
		// (mirroring profileSwitched/resumeModeChanged) rather than the
		// attachError banner sessionRestarted uses -- the dialog is still the
		// right place to retry or switch to "restart" instead.
		if msg.err != nil {
			m.restartChoiceNote = "Cannot inject: " + msg.err.Error()
			return m, nil
		}
		m.restartChoosing = false
		m.restartChoiceNote = ""
		return m, m.loadSessions
	case profileSwitched:
		if msg.err != nil {
			m.profileSwitchNote = "Cannot change permission profile: " + msg.err.Error()
			return m, nil
		}
		m.profileSwitching = false
		m.profileSwitchNote = ""
		return m, m.loadSessions
	case resumeModeChanged:
		if msg.err != nil {
			m.pinNote = "Cannot change resume mode: " + msg.err.Error()
			return m, nil
		}
		m.pinning = false
		m.pinNote = ""
		return m, m.loadSessions
	case sessionRenamed:
		// Mirrors profileSwitched/resumeModeChanged exactly: a successful
		// rename closes the rename sub-dialog (m.detail, underneath it,
		// stays true -- rename is an action inside detail, so submitting
		// it returns to detailView showing the new name, never all the way
		// out to the main list).
		if msg.err != nil {
			m.renameNote = "Cannot rename: " + msg.err.Error()
			return m, nil
		}
		m.renaming = false
		m.renameNote = ""
		return m, m.loadSessions
	case envEdited:
		// Unlike profileSwitched/resumeModeChanged, a committed edit does
		// NOT close the dialog: SPEC §6.1/§6.3's env editor is a listing of
		// several keys, and a user editing one is very likely about to edit
		// another in the same visit. Only esc (already wired in
		// updateEnvDialog) closes it.
		if msg.err != nil {
			m.envNote = "Cannot edit environment: " + msg.err.Error()
			return m, nil
		}
		m.envNote = ""
		return m, m.loadSessions
	case reconcileTick:
		loadAfterReconcile := m.loadSessions
		if m.reconcile != nil {
			loadAfterReconcile = func() tea.Msg {
				if err := m.reconcile(context.Background()); err != nil {
					return sessionsLoaded{err: err}
				}
				return m.loadSessions()
			}
		}
		return m, tea.Batch(loadAfterReconcile, tea.Tick(m.settings.Reconcile, func(t time.Time) tea.Msg { return reconcileTick(t) }))
	case previewTick:
		// The capture engine (task 017, SPEC requirements 21, 22) samples the
		// selected row's live pane once per tick; rendering it (crop, geometry
		// line, placeholders) is tasks 018-021, so DECK_PREVIEW_MS's cadence is
		// already honoured end-to-end even though the panel still shows its
		// pre-capture placeholder.
		cmds := []tea.Cmd{tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return previewTick(t) })}
		if cmd := m.capturePreview(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// steer 018 item 4: the passive-preview fit is coalesced against this
		// SAME tick, never issued once per row walked while holding an arrow
		// key (previewFit's own guards decide whether this tick's selection
		// still needs one at all).
		if cmd := m.previewFit(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case previewFitDone:
		m.previewFitSessionID = msg.sessionID
		// Cleared unconditionally, not only when it matches the session
		// reported: at most one fit is ever outstanding (previewFit refuses
		// to schedule a second while previewFitInFlight is set), and an
		// unconditional clear cannot wedge the mechanism even if some future
		// path ever delivered a previewFitDone the marker did not name. The
		// reported session may well no longer be selected (the user kept
		// navigating while the fit ran) -- that is exactly the case the next
		// tick must be free to fit.
		m.previewFitInFlight = ""
		return m, nil
	case previewCaptured:
		// A session with no live pane reports capture.Live == false and a nil
		// err (see tmux.CapturePreview); only a genuine tmux/transport failure
		// reaches err, and even that is not load-bearing — the previous frame
		// (or, before the first successful capture, the placeholder) keeps
		// rendering rather than the tick disrupting the view.
		if msg.err == nil {
			m.previewSessionID = msg.sessionID
			m.previewLive = msg.capture.Live
			m.previewBytes = msg.capture.Bytes
			m.previewPaneWidth = msg.capture.Width
			m.previewPaneHeight = msg.capture.Height
		}
		return m, nil
	case animationTick:
		if !m.settings.Animation {
			return m, nil
		}
		return m, tea.Tick(m.settings.Preview, func(t time.Time) tea.Msg { return animationTick(t) })
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			// Bubble Tea's own PTY reader coalesces multiple keystrokes that
			// land in the same read into a single KeyMsg whose Runes holds
			// every character (e.g. two quick presses of 'j' and 'x' can
			// arrive as one KeyMsg{Runes: "jx"}). Every case below (and every
			// dialog's own update* method) matches msg.String() against a
			// single key's own string ("j", "x", ...); a coalesced "jx" would
			// match NONE of them and the whole event would be silently
			// dropped, leaving neither rune to act (task 118, requirement 51).
			// Rather than teach every case and every dialog about multi-rune
			// strings, split the coalesced KeyMsg back into one single-rune
			// tea.KeyMsg per character and dispatch them through Update in
			// order, exactly as if they had arrived as separate keystrokes. A
			// single keypress (len(Runes)==1) is untouched and falls straight
			// through to the handling below, unchanged.
			//
			// A bracketed-paste KeyMsg (msg.Paste) is deliberately exempted:
			// Key.String() already wraps a paste's runes in "[...]" so it can
			// never match a single-letter shortcut by accident (bubbletea's own
			// key.go). Splitting pasted text into individual keystrokes would
			// turn a paste of, say, "dd" into an actual delete chord -- the
			// deliberate decision (docs/reports/phase3-findings.md, task 118)
			// is that a paste into the list is ignored outright, never
			// dispatched rune by rune.
			var cmds []tea.Cmd
			next := tea.Model(m)
			for _, r := range msg.Runes {
				var cmd tea.Cmd
				next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
				cmds = append(cmds, cmd)
			}
			return next, tea.Batch(cmds...)
		}
		if m.interactive {
			return m.updateInteractive(msg)
		}
		if m.creating {
			return m.updateCreate(msg)
		}
		if m.profileSwitching {
			return m.updateProfileSwitch(msg)
		}
		if m.pinning {
			return m.updatePinDialog(msg)
		}
		if m.envEditing {
			return m.updateEnvDialog(msg)
		}
		if m.restartChoosing {
			return m.updateRestartChoice(msg)
		}
		if m.deleteConfirming {
			return m.updateDeleteConfirm(msg)
		}
		// R72 (issue #10): the archive confirm intercepts every key while it
		// is open, exactly as deleteConfirming above does -- which is what
		// makes a second `A` inside the dialog a no-op rather than a
		// re-entrant archive, and what keeps `x`/`dd`/`r` from acting on the
		// row the dialog is asking about.
		if m.archiveConfirming {
			return m.updateArchiveConfirm(msg)
		}
		if m.settingsOpen {
			return m.updateSettings(msg)
		}
		if m.themePicking {
			return m.updateThemePicker(msg)
		}
		if m.help {
			return m.updateHelpView(msg)
		}
		if m.renaming {
			return m.updateRenameDialog(msg)
		}
		if m.detail {
			return m.updateDetailView(msg)
		}
		if m.eventLogOpen {
			return m.updateEventLog(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		// pendingDelete intercepts the very next key after a lone `d`
		// (SPEC's dd chord): a second `d` opens the confirm dialog; every
		// other key -- Esc included -- clears the pending indicator and is
		// otherwise swallowed, so "d followed by any other key performs no
		// destructive action" holds without also having to reason about
		// whatever that other key would normally have done.
		if m.pendingDelete {
			m.pendingDelete = false
			if msg.String() == "d" && len(m.sessions) > 0 && canDelete(m.sessions[m.selected]) {
				m.deleteConfirming = true
				m.deleteNote = ""
				if len(m.marked) > 0 {
					// Task 112: purge resolves one session's one declared
					// transcript path at a time (task 109/110) -- not
					// offered at all for a bulk delete, so nothing here
					// resolves a path.
					m.deletePurgeValue = ""
					m.deletePurgePath = ""
					m.deletePurgeOK = false
				} else {
					m.deletePurgeValue = "keep"
					m.deletePurgePath, m.deletePurgeOK = m.transcriptPathFor(m.sessions[m.selected])
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			// This switch is only ever reached with m.help == false (task 078:
			// updateHelpView, dispatched above, now intercepts every key,
			// including a second "?"/esc, while help is open), so this only
			// ever opens it; helpScroll resets so a reopen never starts
			// scrolled from wherever a previous visit left off.
			m.help = true
			m.helpScroll = 0
		case "esc":
			// detailView has no fields to submit or cycle, so the only §11.4
			// contract key it binds is esc — shared here through the same
			// applyDialogContract implementation createView, profileSwitchView
			// and pinView defer to, rather than a sixth hand-written cancel.
			// Task 112: a plain top-level Esc also clears the mark set ("the
			// marks clear on the action and on esc"). Neither m.help nor
			// m.detail is ever true here (task 013's updateDetailView and
			// task 078's updateHelpView each intercept every key, including
			// esc, while their own overlay is open) -- this branch cannot
			// close either today, but keeps clearing both anyway so a caller
			// that somehow reaches it with one already true is not left
			// stuck open.
			_, _ = applyDialogContract(msg, dialogContract{Cancel: func() {
				m.help = false
				m.detail = false
				m.marked = nil
			}})
		case "i":
			// m.detail is never true here (task 013's updateDetailView
			// intercepts every key, including a second "i", while it is), and
			// m.help is never true here either (task 078's updateHelpView
			// intercepts every key first), so this only ever opens it;
			// detailScroll resets for the same reason helpScroll does above.
			if len(m.sessions) > 0 {
				m.detail = true
				m.detailScroll = 0
			}
		case ",":
			if !m.help {
				m.settingsOpen = true
				m.settingsCategoryIndex = 0
				m.settingsFieldIndex = 0
				m.settingsFocus = settingsFocusCategories
				m.settingsSearchActive = false
				m.settingsSearchQuery = ""
				m.settingsSearchIndex = 0
				m.settingsEdits = settingsEditsFromSettings(m.settings)
				m.settingsSavedEdits = settingsEditsFromSettings(m.settings)
				m.settingsDiscardConfirm = false
				m.settingsNote = ""
				m.settingsEnvOpen = false
				m.settingsEnvEditing = false
				m.settingsEnvIndex = 0
			}
		case "t":
			if !m.help {
				m = m.openThemePicker()
			}
		case "n":
			if !m.help {
				m.creating, m.createError, m.createField = true, "", 0
				m.createScroll = 0
				m.createName = ""
				m.createCWD, m.createCWDLastUsed = m.prefillCreateCWD()
				m.createCWDPrefilled = true
				m.createCWDRecents, m.createCWDRecentIndex = nil, -1
				m.createCWDPreCycleValue, m.createCWDPreCyclePrefilled, m.createCWDPreCycleLastUsed = "", false, false
				m.closeCreateCWDCandidates()
				m.createAgent = defaultCreateAgent(m.registry().Kinds())
				m.createProfile = m.defaultCreateProfile(m.createAgent)
				m.createProfileTouched = false
				m.createLaunchArgs, m.createEnv, m.createPreLaunch, m.createLoginShell = "", "", "", false
			}
		case "up", "k":
			if next, ok := m.prevVisibleSelection(m.selected); ok {
				m.selected = next
			}
		case "down", "j":
			if next, ok := m.nextVisibleSelection(m.selected); ok {
				m.selected = next
			}
		case "pgup":
			// ·11.3 requirement 19: PgUp/PgDn always drive the list, since the
			// sidebar is the only focusable region and there is no tab panel
			// cycle to move the page keys onto instead. pageSelection walks
			// VISUAL rows (002-steering.md), not raw m.sessions index
			// arithmetic, so a page of hidden/non-adjacent rows can't skew it.
			m.selected = m.pageSelection(-m.sidebarRowsPerPage())
		case "pgdown":
			m.selected = m.pageSelection(m.sidebarRowsPerPage())
		case "Y":
			if m.acknowledge == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if !canAcknowledge(session) {
				return m, nil
			}
			sessionID := session.ID
			return m, func() tea.Msg {
				return sessionAcknowledged{err: m.acknowledge(context.Background(), sessionID)}
			}
		case "x":
			if m.kill == nil || len(m.sessions) == 0 {
				return m, nil
			}
			if len(m.marked) > 0 {
				// Task 112: a non-empty mark set means x acts on the WHOLE
				// batch instead of just the selected row. An already-stopped
				// marked row is silently skipped (never an error, unlike the
				// single-row refusal below) since a batch action naming
				// rows that need nothing done would be noise, not a
				// destructive-action guard. Marks clear immediately -- "the
				// action" here IS pressing x, not waiting for the kills to
				// finish.
				sessions := m.markedSessions()
				m.marked = nil
				if len(sessions) == 0 {
					return m, nil
				}
				kill := m.kill
				return m, func() tea.Msg {
					result := sessionsBulkKilled{}
					for _, s := range sessions {
						if !canKill(s) {
							continue
						}
						result.sessions = append(result.sessions, s)
						result.errs = append(result.errs, kill(context.Background(), s))
					}
					return result
				}
			}
			session := m.sessions[m.selected]
			// The already-stopped refusal is the SERVICE's verdict, not a
			// status read here: under `remain-on-exit failed` a row can read
			// stopped while tmux still retains its dead pane, and refusing
			// on status alone left that session's name held with no in-app
			// way to free it (#6). service.Kill refuses only when the row is
			// stopped AND no tmux session exists, and its error message is
			// the same wording this key used to render locally, so a
			// genuinely stopped row still shows "Cannot kill: session is
			// already stopped" (via the sessionKilled branch).
			return m, func() tea.Msg {
				return sessionKilled{session: session, err: m.kill(context.Background(), session)}
			}
		case "m":
			// Task 112, requirement 28: m toggles a mark on the selected row
			// keyed by session id (never a visual index), so the set
			// survives a re-sort or re-group untouched -- markedSessions()
			// re-resolves it through visualOrder() fresh every time it is
			// consulted rather than caching anything positional.
			if !m.help && len(m.sessions) > 0 {
				id := m.sessions[m.selected].ID
				if m.marked == nil {
					m.marked = map[string]bool{}
				}
				if m.marked[id] {
					delete(m.marked, id)
				} else {
					m.marked[id] = true
				}
			}
		case "A":
			// R72 (issue #10), SPEC.md:752: `A` writes NOTHING on the keypress.
			// It only opens the confirm dialog below, whose Enter then performs
			// SPEC requirement 27's archive: on a stopped row that only sets
			// archived_at; on any other row archiveSvc
			// (internal/service.Service.Archive) kills the live pane first and
			// archives in the same action ("kill and archive", §4's invariant)
			// rather than refusing the keypress the way x refuses an
			// already-stopped row -- that decision stays archiveSvc's, never
			// this switch's, and the dialog says so in as many words before
			// anything is written. `A` is deliberately NOT rebound and grows no
			// chord here (the operator declined both): the confirm alone is what
			// stops `A` -- one Shift away from `a`, attach -- from killing a live
			// agent on a single keystroke.
			if m.archiveSvc == nil || len(m.sessions) == 0 {
				if len(m.sessions) > 0 {
					m.attachError = "Archiving is unavailable"
				}
				return m, nil
			}
			if !canArchive(m.sessions[m.selected]) {
				return m, nil
			}
			m.archiveConfirming = true
			m.archiveNote = ""
			return m, nil
		case "U":
			// R71 (issue #8), SPEC.md:323-332: `A` is reversible, so `U`
			// clears archived_at on the selected row. It acts on m.sessions --
			// the DISPLAYED list -- which is exactly what makes it reachable
			// from inside requirement 33's `/` filter results: an archived row
			// is absent from the default list entirely and only ever appears
			// while a query is in force (filteredSessions widens the pool to
			// m.archivedSessions), and after Enter closes the text field the
			// freed keymap -- this switch -- acts on that narrowed list. A row
			// that is not archived is refused here rather than being handed to
			// the store, so `U` can never record an "unarchived" event for a
			// row that was never archived.
			if m.unarchiveSvc == nil || len(m.sessions) == 0 {
				if len(m.sessions) > 0 {
					m.attachError = "Unarchiving is unavailable"
				}
				return m, nil
			}
			session := m.sessions[m.selected]
			if !canUnarchive(session) {
				m.attachError = "Cannot unarchive: session is not archived"
				return m, nil
			}
			return m, func() tea.Msg {
				unarchived, err := m.unarchiveSvc(context.Background(), session.ID)
				return sessionUnarchived{session: unarchived, err: err}
			}
		case "d":
			// First half of task 105's dd chord: a visible pending indicator,
			// changing nothing in the store. The very next key (handled by
			// the m.pendingDelete intercept above, on the NEXT tea.KeyMsg) is
			// either another `d` (opens the confirm dialog) or clears this
			// with no destructive action.
			if len(m.sessions) > 0 {
				m.pendingDelete = true
			}
		case "u":
			// Requirement 22: u undoes the most recent x, not whatever row
			// happens to be selected right now -- undoSessionID is only ever
			// set by a successful kill and cleared by either this key or the
			// DECK_UNDO_MS expiry tick, so an expired or never-killed state
			// makes u a no-op exactly as the requirement states. Requirement
			// 23 (task 106): once the kill-undo trio is empty, u falls back
			// to undoing the most recent dd delete within its own
			// DECK_DELETE_GRACE_MS window, tracked by the separate
			// deleteUndoSessionID trio (never merged with the one above).
			if m.undoSessionID != "" {
				if m.resume == nil {
					return m, nil
				}
				sessionID := m.undoSessionID
				m.undoSessionID, m.undoSessionName = "", ""
				m.undoGeneration++
				return m, func() tea.Msg {
					resumed, outcome, err := m.resume(context.Background(), sessionID)
					return sessionResumed{session: resumed, outcome: outcome, err: err}
				}
			}
			// Task 112: the batch-kill undo window is checked next, still
			// ahead of the delete-undo trio below -- a batch x's undo is
			// "undo the most recent x" too, exactly like the single-session
			// case just above, just covering N sessions with one keypress.
			if len(m.batchUndoSessionIDs) > 0 {
				if m.resume == nil {
					return m, nil
				}
				ids := m.batchUndoSessionIDs
				m.batchUndoSessionIDs = nil
				m.batchUndoGeneration++
				resume := m.resume
				return m, func() tea.Msg {
					result := sessionsBulkResumed{}
					for _, id := range ids {
						_, _, err := resume(context.Background(), id)
						result.errs = append(result.errs, err)
					}
					return result
				}
			}
			if m.deleteUndoSessionID != "" {
				if m.restoreSvc == nil {
					return m, nil
				}
				sessionID := m.deleteUndoSessionID
				m.deleteUndoSessionID, m.deleteUndoSessionName = "", ""
				m.deleteUndoGeneration++
				return m, func() tea.Msg {
					restored, err := m.restoreSvc(context.Background(), sessionID)
					return sessionRestored{session: restored, err: err}
				}
			}
			// Task 112: the batch-delete undo window mirrors the single dd
			// undo case directly above, one shared window restoring every
			// session a marked-set dd tombstoned.
			if len(m.batchDeleteUndoSessionIDs) > 0 {
				if m.restoreSvc == nil {
					return m, nil
				}
				ids := m.batchDeleteUndoSessionIDs
				m.batchDeleteUndoSessionIDs = nil
				m.batchDeleteUndoGeneration++
				restoreSvc := m.restoreSvc
				return m, func() tea.Msg {
					result := sessionsBulkRestored{}
					for _, id := range ids {
						_, err := restoreSvc(context.Background(), id)
						result.errs = append(result.errs, err)
					}
					return result
				}
			}
			// R72 (issue #10, SPEC.md:752): the archive window is checked LAST,
			// behind all four kill/delete trios above, so adding it cannot change
			// what `u` does for any pre-existing window -- an archive undo only
			// ever runs when no kill and no delete undo is outstanding. Its
			// reversal is unarchiveSvc (R71's store.UnarchiveSession), the same
			// service `U` uses, so the row returns to the default list reading
			// stopped/resumable; `A`'s kill is deliberately NOT resumed here (the
			// toast says so in as many words), since undoing a hide must never
			// silently relaunch an agent.
			if m.archiveUndoSessionID != "" {
				if m.unarchiveSvc == nil {
					return m, nil
				}
				sessionID := m.archiveUndoSessionID
				m.archiveUndoSessionID, m.archiveUndoSessionName = "", ""
				m.archiveUndoKilled = false
				m.archiveUndoGeneration++
				return m, func() tea.Msg {
					unarchived, err := m.unarchiveSvc(context.Background(), sessionID)
					return sessionUnarchived{session: unarchived, err: err}
				}
			}
			return m, nil
		case "r":
			if m.resume == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if !canResume(session) {
				m.attachError = "Cannot resume: session is not stopped"
				return m, nil
			}
			sessionID := session.ID
			return m, func() tea.Msg {
				resumed, outcome, err := m.resume(context.Background(), sessionID)
				return sessionResumed{session: resumed, outcome: outcome, err: err}
			}
		case "R":
			if m.restart == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if !canRestart(session) {
				m.attachError = "Cannot restart: session is not running (use r to resume it)"
				return m, nil
			}
			if session.Agent == "shell" {
				// Task 023: a shell session gets the restart/inject-instead
				// choice instead of restarting immediately -- restarting a
				// plain shell loses whatever state (cwd, history, running
				// commands) that shell had, which inject-instead avoids.
				m.restartChoosing = true
				m.restartChoiceValue = "restart"
				m.restartChoiceNote = ""
				return m, nil
			}
			sessionID := session.ID
			return m, func() tea.Msg {
				restarted, outcome, err := m.restart(context.Background(), sessionID)
				return sessionRestarted{session: restarted, outcome: outcome, err: err}
			}
		case "P":
			if m.profileSwitch == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if !m.canSwitchProfile(session) {
				m.attachError = "Cannot change permission profile: " + session.Agent + " has no permission profile"
				return m, nil
			}
			m.profileSwitching = true
			m.profileSwitchValue = session.PermissionProfile
			m.profileSwitchNote = ""
			return m, nil
		case "p":
			if m.resumeMode == nil || len(m.sessions) == 0 {
				return m, nil
			}
			session := m.sessions[m.selected]
			if !m.canPinResume(session) {
				m.attachError = "Cannot change resume mode: " + session.Agent + " has no conversation id to pin or restart fresh"
				return m, nil
			}
			m.pinning = true
			m.pinValue = session.ResumeState
			if m.pinValue == "" {
				m.pinValue = "auto"
			}
			m.pinNote = ""
			return m, nil
		case "e":
			// SPEC §6.1/§6.3, task 020: opens for any selected session (unlike
			// `P`/`p`, which gate on adapter capabilities) since every session,
			// including a plain shell one, has an effective environment worth
			// showing -- there is no "not applicable here" case to refuse.
			if !m.help && len(m.sessions) > 0 {
				m.envEditing = true
				m.envCursor = 0
				m.envEditKey, m.envEditValue, m.envNote = "", "", ""
				m.envEditPrefilled = false
				m.envReveal = false
			}
		case "E":
			// SPEC §12/requirement 32, task 124: unlike `e`, this is global
			// -- not gated on a selected session -- since the event log
			// lists every session's events, not one row's own environment.
			// eventLogScroll resets (task 078) so a reopen never starts
			// scrolled from wherever a previous visit left off. R61 (steer
			// 3e-001 §6.3): the store read itself is dispatched here, once,
			// as a tea.Cmd (loadEventLog) rather than performed inline in
			// View() -- eventLogRows/eventLogErr also reset so a reopen
			// never renders the previous visit's rows before the fresh
			// fetch lands.
			m.eventLogOpen = true
			m.eventLogScroll = 0
			m.eventLogRows = nil
			m.eventLogErr = nil
			return m, m.loadEventLog
		case "/":
			// SPEC.md:984/requirement 33, task 123: global like `E` above --
			// not gated on a selected session, since an empty list is still
			// worth filtering into (e.g. to prove nothing archived matches).
			// Reopening with an existing query keeps it rather than clearing
			// it, mirroring the rename dialog's own prefill convention, so a
			// second `/` refines an already-applied filter instead of
			// discarding it.
			if !m.help {
				m.filtering = true
				return m, m.loadArchivedSessions
			}
		case " ":
			// SPEC requirements 31, 32: move to the next session needing
			// attention, wrapping, via the one shared NeedsAttention answer
			// (internal/tui/attention.go). Nothing needing attention (ok
			// false) leaves selection — and every session's status —
			// untouched: this must never behave like §7's attach, which
			// clears "waiting" on the attached session.
			if !m.help && len(m.sessions) > 0 {
				if next, ok := m.nextAttentionSelection(m.selected); ok {
					m.selected = next
				}
			}
		case "c":
			// SPEC §11.8 gap (requirement 30's collapsible headers had no key):
			// toggle the selected row's own workspace group collapsed/expanded,
			// via the identical helper task 028's mouse header click will call
			// (toggleGroupCollapse), so neither path is ever the only way to
			// reach this capability. A no-op, like every other bare-letter
			// binding, while help or the `i` detail overlay covers the sidebar,
			// or when there is no row to resolve a group from.
			//
			// Task 119: this was originally bound to `g`, which collides with
			// SPEC.md:952's own keymap entry "g/G top/bottom" -- `g`/`G` were
			// never actually wired to anything, so every keypress of `g` was
			// silently doing collapse instead of the documented top/bottom jump.
			// `c` (collapse) does not appear anywhere in SPEC §11's keymap list.
			// Requirement 35: with grouping off there is no group for `c`
			// to collapse -- the binding is a no-op rather than silently
			// populating m.collapsedGroups bookkeeping nothing will ever
			// read (collapse state must be absent, not merely inert).
			if !m.help && !m.detail && m.groupingEnabled() && len(m.sessions) > 0 {
				m.toggleGroupCollapse(sessionWorkspace(m.sessions[m.selected]))
			}
		case "g":
			// SPEC.md:952 "g/G top/bottom": jump to the first visible row in
			// visual order (mirrors ↑/↓'s own visualOrder-based navigation, so
			// a collapsed group's hidden rows are skipped exactly like a single
			// ↑/↓ press would skip them).
			if !m.help && !m.detail && len(m.sessions) > 0 {
				if visible := m.visibleSessionIndices(); len(visible) > 0 {
					m.selected = visible[0]
				}
			}
		case "G":
			// SPEC.md:952 "g/G top/bottom": jump to the last visible row.
			if !m.help && !m.detail && len(m.sessions) > 0 {
				if visible := m.visibleSessionIndices(); len(visible) > 0 {
					m.selected = visible[len(visible)-1]
				}
			}
		case "|":
			if !m.help {
				return m.cycleLayoutMode()
			}
		case "<":
			if !m.help {
				m.sidebarWidth = m.adjustSidebarWidth(-1)
				return m, m.persistSidebarWidth()
			}
		case ">":
			if !m.help {
				m.sidebarWidth = m.adjustSidebarWidth(1)
				return m, m.persistSidebarWidth()
			}
		case "enter":
			return m.enterInteractive()
		case "a":
			return m.attachSelected()
		}
	case tea.MouseMsg:
		// [ui] mouse / DECK_MOUSE (requirement 3, 37): bubbletea's own input
		// reader decodes an SGR/X10 mouse report from raw input bytes
		// unconditionally, regardless of whether tea.WithMouseCellMotion
		// was passed at startup (that option only controls whether the
		// *enable* escape sequence is written to the terminal in the first
		// place) -- a real, compliant terminal simply never emits a mouse
		// report deck did not ask for, but this second, product-side gate
		// makes the opt-out authoritative even if one arrives anyway (a
		// terminal that ignores the missing enable sequence, or a replayed
		// byte stream), rather than relying solely on a well-behaved
		// terminal's cooperation.
		if !m.settings.Mouse {
			return m, nil
		}
		// PRD II-51: the wheel scrolls the interactive grid's own bounded
		// scrollback while interactive mode owns the keyboard, the one
		// mouse gesture interactive mode accepts at all -- every other
		// gesture (press/drag/release/double-click) stays a no-op below,
		// exactly as the whole of interactive mode already was before this
		// (a click over a live pane makes no more sense here than it does
		// over the passive preview in list mode, which also ignores
		// clicks).
		if m.interactive {
			if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
				if hit := m.hitTest(msg.X, msg.Y); hit.panel == hitPanelPreview {
					delta := interactiveWheelStepLines
					if msg.Button == tea.MouseButtonWheelDown {
						delta = -delta
					}
					return m.scrollInteractiveByLines(delta)
				}
				return m, nil
			}
			// Task 313/R54, SPEC §11.8: hit-test a left PRESS first, before
			// ever assuming it is task 216's drag-to-copy gesture -- a press
			// that resolves to a sidebar row re-targets interactive mode onto
			// that session (leaving the current one, restoring its window
			// geometry byte-exact, then entering the new one; a press on the
			// row that is ALREADY the interactive target is a no-op: no
			// leave, no re-enter, no resize). A press over the preview or the
			// seam falls straight through, unchanged, to the drag-to-copy
			// path below.
			if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
				if hit := m.hitTest(msg.X, msg.Y); hit.panel == hitPanelSidebar && hit.target == hitTargetRow {
					return m.retargetInteractiveSidebarClick(hit.sessionIndex)
				}
			}
			// Steer 017 item 3/task 216, SPEC §11.8: a left-button drag
			// beginning inside the interactive preview's own content box
			// selects text; releasing after a genuine drag copies it. Every
			// OTHER gesture (a plain click, any button but left, anything
			// outside the content box) stays the no-op interactive mode
			// already made every non-wheel gesture before this.
			if msg.Button == tea.MouseButtonLeft {
				switch msg.Action {
				case tea.MouseActionPress:
					if updated, ok := m.beginInteractiveSelection(msg.X, msg.Y); ok {
						return updated, nil
					}
					return m, nil
				case tea.MouseActionMotion:
					if m.interactiveSelecting {
						return m.updateInteractiveSelection(msg.X, msg.Y), nil
					}
					return m, nil
				case tea.MouseActionRelease:
					if m.interactiveSelecting {
						return m.commitInteractiveSelection(), nil
					}
					return m, nil
				}
			}
			return m, nil
		}
		// R73 (issue #7): a wheel notch over one of the three scrollable
		// overlays scrolls THAT overlay's own viewport, by the same one-line
		// step up/down and j/k bind. This is tested BEFORE the blanket
		// suppression below -- "scrollable overlay AND wheel event" and
		// nothing else -- so the action-suppressing rule underneath stays
		// exactly as strict as it was for every other gesture: a click, a
		// drag, a release or any other button still returns early for all
		// fifteen overlay flags, and a wheel notch over one of the twelve
		// unscrollable overlays still does nothing either (scrollWheelOverlay
		// reports false and this falls through to that same return).
		//
		// Why no hit test on msg.X/msg.Y: an overlay is modal -- it owns the
		// keyboard outright and nothing underneath it is reachable while it is
		// up -- so there is no second thing a wheel notch could have been
		// meant for, exactly as PgUp/PgDn need no pointer to decide what they
		// page. (Interactive mode's wheel, handled above, DOES hit-test,
		// because there the sidebar next to the pane is genuinely live.)
		//
		// SPEC.md:1250 is not violated: it forbids the mouse *cancelling or
		// confirming* a dialog and forbids reaching a dialog action by mouse
		// alone. Moving a read-only viewport cancels nothing, confirms
		// nothing, takes no focus and moves no selection -- the same test
		// §11.8 already applies to drag-to-select over the preview ("selecting
		// text is reading rather than acting").
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			dir := 1
			if msg.Button == tea.MouseButtonWheelUp {
				dir = -1
			}
			if scrolled, ok := m.scrollWheelOverlay(dir); ok {
				return scrolled, nil
			}
		}
		// SPEC §11.4/§11.8: the mouse can neither cancel nor confirm a dialog,
		// and no dialog action is reachable by mouse alone, so every overlay
		// that already makes the bare-letter keymap a no-op ignores the mouse
		// exactly the same way.
		if m.help || m.creating || m.profileSwitching || m.pinning || m.detail || m.renaming || m.themePicking || m.settingsOpen || m.settingsDiscardConfirm || m.envEditing || m.restartChoosing || m.deleteConfirming || m.archiveConfirming || m.eventLogOpen || m.filtering || m.interactive {
			return m, nil
		}
		return m.handleMouse(msg)
	}
	if dir, ok := shiftPageScrollDir(message); ok {
		return m.scrollInteractiveByPage(dir)
	}
	return m, nil
}

// attachSelected is `a`'s job (SPEC §11.9, task 061): the full attach that
// hands the terminal away entirely. Since task 064/II-45 the sidebar
// double-click no longer duplicates this -- it enters interactive mode
// instead (see clickSidebarRow/enterInteractive) -- so attachSelected is
// reachable only by the `a` key.
func (m Model) attachSelected() (tea.Model, tea.Cmd) {
	if m.attach == nil || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return m, nil
	}
	session := m.sessions[m.selected]
	if !canReachPane(session) {
		m.attachError = "Cannot attach: session is stopped; resume it first"
		return m, nil
	}
	command, err := m.attach(context.Background(), session.Slug)
	if err != nil {
		m.attachError = "Cannot attach: " + err.Error()
		return m, nil
	}
	// Consult the durable row on every attachment. The list frame may lag a
	// hook that just made it waiting or error; the store transaction applies
	// only that row's current status and leaves raced resolutions untouched.
	if m.prepareAttach != nil {
		if err := m.prepareAttach(context.Background(), session.ID); err != nil {
			m.attachError = "Cannot attach: " + err.Error()
			return m, nil
		}
	}
	m.attachError = ""
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return attachFinished{err: err} })
}

// cycleLayoutMode is `|`'s own path (SPEC §11.2/§11.8 requirement 9): auto
// -> side-by-side -> stacked -> collapsed -> auto. The collapsed strip's
// click does *not* call this any more (see restoreFromCollapsedStrip) --
// requirement 33 names its own, different behaviour. The first press of a
// fresh excursion (layoutCycleActive false) anchors preCollapseLayoutMode
// to the mode it is leaving and marks the excursion active; every
// subsequent press in the same excursion leaves that anchor untouched, so
// a run of presses that passes through an intermediate mode on its way to
// collapsed still remembers the mode the run started from, not the stop it
// passed through. Wrapping back to auto ends the excursion so the next
// press anchors fresh.
func (m Model) cycleLayoutMode() (tea.Model, tea.Cmd) {
	next := nextLayoutMode(m.layoutMode)
	if !m.layoutCycleActive {
		m.preCollapseLayoutMode = m.layoutMode
		m.layoutCycleActive = true
	}
	if next == LayoutAuto {
		m.layoutCycleActive = false
	}
	m.layoutMode = next
	return m, m.persistLayoutMode()
}

// restoreFromCollapsedStrip is the collapsed strip's own click path (SPEC
// §11.8 requirement 33: "click the collapsed strip -> restores the
// previous non-collapsed mode"), deliberately not cycleLayoutMode's `|`
// cycle: from collapsed, `|` always advances to auto, but the strip must
// return to whatever was pinned before the current `|` excursion began.
// No such mode was ever recorded (e.g. collapsed was reached by directly
// setting the pin rather than via `|`) falls back to auto, the same
// default every other unset pin uses. The restore itself ends the
// excursion, same as wrapping to auto would, so the next `|` press anchors
// fresh from the restored mode.
func (m Model) restoreFromCollapsedStrip() (tea.Model, tea.Cmd) {
	mode := m.preCollapseLayoutMode
	if mode == "" {
		mode = LayoutAuto
	}
	m.layoutMode = mode
	m.preCollapseLayoutMode = ""
	m.layoutCycleActive = false
	return m, m.persistLayoutMode()
}

// persistLayoutMode returns a command that writes the just-updated
// layoutMode pin to state.db's ui_state table (task 016, SPEC §11.2), never
// to config.toml. With no store attached (most unit tests) it is a no-op so
// callers can always chain it after mutating m.layoutMode.
func (m Model) persistLayoutMode() tea.Cmd {
	if m.store == nil {
		return nil
	}
	mode := m.layoutMode
	return func() tea.Msg {
		return uiStatePersisted{err: m.store.SetLayoutMode(context.Background(), mode)}
	}
}

// persistSidebarWidth is persistLayoutMode's sidebar_width counterpart.
func (m Model) persistSidebarWidth() tea.Cmd {
	if m.store == nil {
		return nil
	}
	width := m.sidebarWidth
	return func() tea.Msg {
		return uiStatePersisted{err: m.store.SetSidebarWidth(context.Background(), width)}
	}
}

func (m Model) View() string {
	if m.help {
		return m.helpView()
	}
	if m.creating {
		return m.createView()
	}
	if m.profileSwitching && len(m.sessions) > 0 {
		return m.profileSwitchView()
	}
	if m.pinning && len(m.sessions) > 0 {
		return m.pinView()
	}
	if m.envEditing && len(m.sessions) > 0 {
		return m.envView()
	}
	if m.restartChoosing && len(m.sessions) > 0 {
		return m.restartChoiceView()
	}
	if m.deleteConfirming && len(m.sessions) > 0 {
		return m.deleteConfirmView()
	}
	if m.archiveConfirming && len(m.sessions) > 0 {
		return m.archiveConfirmView()
	}
	if m.settingsOpen {
		return m.settingsView()
	}
	if m.renaming && len(m.sessions) > 0 {
		return m.renameView()
	}
	if m.detail && len(m.sessions) > 0 {
		return m.detailView()
	}
	if m.eventLogOpen {
		return m.eventLogView()
	}
	return m.mainView()
}

// frameSize is the terminal size mainView renders at. A zero width/height
// (every Model built without ever receiving a tea.WindowSizeMsg — which is
// most of this package's unit tests) defaults to deck's own 80×24 supported
// minimum rather than to a degenerate empty frame, so those tests keep
// exercising a real, legible layout.
func (m Model) frameSize() (width, height int) {
	width, height = m.width, m.height
	if width <= 0 {
		width = AutoSideBySideWidth
	}
	if height <= 0 {
		height = MinRows
	}
	return width, height
}

// startupBanner is the tmux-unavailable startup note (SPEC requirement: the
// install guidance stays reachable), rendered across the full terminal
// width above the panels rather than wrapped inside the sidebar's 33-column
// budget, which is too narrow to hold features/tmux_contract.feature's
// "tmux 3.1c is too old" on one line. It returns no lines at all once tmux
// is fine, so it costs nothing in the common case.
func (m Model) startupBanner(width int) []string {
	if m.startupNote == "" {
		return nil
	}
	var lines []string
	lines = append(lines, wrapText("tmux unavailable: "+m.startupNote, width)...)
	lines = append(lines, wrapText("Install tmux 3.2 or newer, then restart deck.", width)...)
	lines = append(lines, "")
	return lines
}

// themeBanner is requirement 28's fallback notice for the [ui] theme key:
// theme.Resolve (invoked by config.LoadFrom) computed ThemeReason once, at
// load time, whenever the configured name could not be resolved -- unknown
// name, or a user theme file that failed to parse -- and fell back to
// theme.Default(). The very first painted frame must say so; it must never
// silently render the default as though the requested theme had applied
// (this is the theme system's own version of a fabricated status). Since
// ThemeReason cannot change for the lifetime of one config load, this
// banner is simply always present once set -- there is no later frame at
// which showing it would stop being honest -- which trivially satisfies
// "on first paint" without needing a one-shot flag threaded through Update.
// It returns no lines at all when ThemeReason is "" (nothing was
// configured, or the configured name resolved cleanly), so it costs
// nothing in the common case, matching startupBanner's own convention.
func (m Model) themeBanner(width int) []string {
	if m.settings.ThemeReason == "" {
		return nil
	}
	var lines []string
	lines = append(lines, wrapText(m.settings.ThemeReason, width)...)
	lines = append(lines, "")
	return lines
}

// effectiveSortOrder resolves requirement R53's `[ui] sort_order` for THIS
// render: one of the four names sort_order.go declares (SortOrderAttention/
// Created/Activity/Name), or an empty string (meaning "nothing configured",
// which config.LoadFrom's own defaultFileConfig already fills in as
// "attention" for a real load -- this only shows up as "" for a
// config.Settings{} test builds directly, exactly the same zero-value
// convention m.settings.Mouse/GroupByWorkspace already rely on), both
// resolve to attention with no reason -- nothing was misconfigured. Any
// OTHER value -- reachable only when something bypasses config.toml's own
// parse-time enum rejection (internal/config/toml.go's KindEnum case),
// e.g. a Settings built directly with a typo -- resolves to attention WITH
// a stated reason, the same shape theme.Resolve's ThemeReason already has
// for an unknown [ui] theme. sortOrderBanner (below) is the one caller
// that reads the reason; sessionsLoaded (below) is the one caller that
// reads the resolved order.
func (m Model) effectiveSortOrder() (order string, reason string) {
	switch m.settings.SortOrder {
	case "", SortOrderAttention:
		return SortOrderAttention, ""
	case SortOrderCreated, SortOrderActivity, SortOrderName:
		return m.settings.SortOrder, ""
	default:
		return SortOrderAttention, fmt.Sprintf("Unknown [ui] sort_order %q -- showing attention order instead.", m.settings.SortOrder)
	}
}

// sortOrderBanner is requirement R53's fallback notice for an unknown/
// malformed [ui] sort_order, on themeBanner's own footing (see its doc
// comment just above): the first painted frame must say so rather than
// silently rendering attention order as though the configured value had
// applied. Returns no lines at all when effectiveSortOrder found nothing
// to explain, matching themeBanner/startupBanner's shared
// costs-nothing-in-the-common-case convention.
func (m Model) sortOrderBanner(width int) []string {
	_, reason := m.effectiveSortOrder()
	if reason == "" {
		return nil
	}
	var lines []string
	lines = append(lines, wrapText(reason, width)...)
	lines = append(lines, "")
	return lines
}

// attachErrorLines is requirement 37's wrapped attachError line set, shared
// between computeLayout's reservation and mainView's actual render so the
// two never disagree about how many rows the message costs. It returns no
// lines at all when there is no error, so it costs nothing in the common
// case — computeLayout must not unconditionally shrink the panels by this
// message's worst case when none is present.
func (m Model) attachErrorLines(width int) []string {
	if m.attachError == "" {
		return nil
	}
	return wrapText(m.attachError, width)
}

// resumeNoteLines is requirement 37's wrapped resumeNote line set, the
// resumeNote counterpart to attachErrorLines above.
func (m Model) resumeNoteLines(width int) []string {
	if m.resumeNote == "" {
		return nil
	}
	return wrapText(m.resumeNote, width)
}

// undoNoteLines is requirement 22's transient toast: visible for exactly
// DECK_UNDO_MS after a successful x, naming the killed session and stating
// that u undoes it, then gone once undoSessionID is cleared (by u itself or
// by the undoExpired tick). Wrapped/reserved the same way attachErrorLines
// and resumeNoteLines are, so it never pushes the frame off-screen. Task
// 112's batch-kill window is checked first: a marked-set x can never be
// active AT THE SAME TIME as a single-row undoSessionID (x clears
// m.marked and sets exactly one of the two tracker pairs, never both), but
// checking the batch one first keeps this in the same priority order the
// `u` key itself uses.
func (m Model) undoNoteLines(width int) []string {
	if len(m.batchUndoSessionIDs) > 0 {
		return wrapText(fmt.Sprintf("Killed %d sessions \u2014 press u to undo", len(m.batchUndoSessionIDs)), width)
	}
	if m.undoSessionID == "" {
		return nil
	}
	return wrapText(fmt.Sprintf("Killed %q \u2014 press u to undo", m.undoSessionName), width)
}

// deleteUndoNoteLines mirrors undoNoteLines's SHAPE (task 106, requirement
// 23): visible for DECK_DELETE_GRACE_MS after a successful dd delete,
// stating that u restores it, then gone once deleteUndoSessionID is
// cleared (by u itself or by the deleteGraceExpired tick, at which point
// the row is also reaped). Deliberately never names the deleted session
// the way undoNoteLines names a killed one: the confirm dialog's own
// submit scenario asserts the deleted session's name is gone from the
// WHOLE screen immediately (dd hides the row, unlike x's "stopped" row
// that stays visible), and a toast quoting that same name back would
// violate that the instant it renders. Task 112's batch-delete window
// (checked first, mirroring undoNoteLines above) never names a session
// either, for the same reason plus its own: it would have to name N.
func (m Model) deleteUndoNoteLines(width int) []string {
	if len(m.batchDeleteUndoSessionIDs) > 0 {
		return wrapText(fmt.Sprintf("Deleted %d sessions \u2014 press u to undo", len(m.batchDeleteUndoSessionIDs)), width)
	}
	if m.deleteUndoSessionID == "" {
		return nil
	}
	return wrapText("Deleted \u2014 press u to undo", width)
}

// archiveUndoNoteLines is R72's success toast (issue #10, SPEC.md:752 "On
// success a toast says what happened, with `u` to undo"): visible for
// DECK_UNDO_MS after a successful `A` submit, then gone once
// archiveUndoSessionID is cleared (by u itself or by the archiveUndoExpired
// tick). Wrapped and reserved exactly like undoNoteLines and
// deleteUndoNoteLines above, so it never pushes the frame off-screen.
//
// Two deliberate wording decisions:
//   - It never names the archived session, for deleteUndoNoteLines' own
//     reason: `A` hides the row from the default list, and
//     features/kill_delete_undo.feature's archive-submit scenario asserts the
//     name is gone from the WHOLE screen, which a toast quoting it back would
//     violate the instant it rendered.
//   - It states the kill when there was one (archiveUndoKilled, i.e. the row
//     was not already stopped), mirroring archiveConfirmBody's own two
//     wordings, and then says `u` unarchives rather than "undoes": the
//     reversal is UnarchiveSession alone, so the agent stays stopped and a
//     bare "press u to undo" would promise the live agent back.
func (m Model) archiveUndoNoteLines(width int) []string {
	if m.archiveUndoSessionID == "" {
		return nil
	}
	if m.archiveUndoKilled {
		return wrapText("Killed and archived \u2014 press u to unarchive (agent stays stopped)", width)
	}
	return wrapText("Archived \u2014 press u to unarchive", width)
}

// pendingDeleteLines is task 105's first-`d` visible indicator: gone the
// instant any key resolves it (the second `d`, opening the confirm dialog,
// or anything else, clearing it with no destructive action), so it is
// never shown at the same time as the confirm dialog itself (deleteConfirmView
// replaces the whole frame -- see View()) or the undo toast (a pending
// delete on a session that was just killed makes little sense, and neither
// key sets both flags together). Task 112: a non-empty mark set names the
// batch size instead of the selected row's own name -- the selected row is
// not necessarily even one of the marked sessions dd is about to act on.
func (m Model) pendingDeleteLines(width int) []string {
	if !m.pendingDelete || len(m.sessions) == 0 {
		return nil
	}
	if len(m.marked) > 0 {
		return wrapText(fmt.Sprintf("Delete %d marked sessions? press d again to confirm, any other key cancels", len(m.marked)), width)
	}
	return wrapText(fmt.Sprintf("Delete %q? press d again to confirm, any other key cancels", m.sessions[m.selected].Name), width)
}

// computeLayout is the one place mainView and the page-size math below call
// ComputeLayout, reserving exactly one row for the footer (SPEC §11.3: "the
// footer is one line, outside both panels") plus the startup banner's own
// rows, if any, plus attachError's and resumeNote's own wrapped row counts,
// if either is set (requirement 37), before handing the rest to the §11.2
// geometry function, which knows about none of them. Skipping any of these
// reservations would let that message's lines push the whole frame past the
// terminal's actual row count, which does not shrink the frame to fit — it
// scrolls, carrying the banner (and the sidebar's own top border) off the
// top of the visible screen (features/tmux_contract.feature's "old tmux is
// actionable" scenario would never see either again). attachError and
// resumeNote are mutually exclusive in practice (tui.go's Update clears one
// whenever it sets the other) but both are budgeted here regardless, so a
// future caller that sets both together still gets a frame that fits.
func (m Model) computeLayout() LayoutResult {
	width, height := m.frameSize()
	reserved := 1 + len(m.startupBanner(width)) + len(m.themeBanner(width)) + len(m.sortOrderBanner(width)) + len(m.themePickerLines(width)) + len(m.attachErrorLines(width)) + len(m.resumeNoteLines(width)) + len(m.undoNoteLines(width)) + len(m.deleteUndoNoteLines(width)) + len(m.archiveUndoNoteLines(width)) + len(m.pendingDeleteLines(width)) + len(m.filterStatusLine(width))
	result := ComputeLayout(width, height-reserved, m.layoutMode, m.sidebarWidth)
	// ComputeLayout's own BelowMinimum reads its rows argument as the full
	// terminal height (its doc comment says so, and its direct unit tests
	// call it that way), but the reserved rows above already subtract the
	// footer/banner before rows ever reaches it here, so an exact 80x24
	// terminal would otherwise report BelowMinimum=true (23 content rows
	// < MinRows) even though 24 is deck's own supported minimum, not below
	// it. Recompute the flag against the real, unreserved terminal size so
	// SPEC requirement 14's footer notice (footerLine) only ever fires for
	// a genuinely below-minimum terminal.
	result.BelowMinimum = width < AutoSideBySideWidth || height < MinRows
	return result
}

// adjustSidebarWidth is `<`/`>`'s one-column step (SPEC requirement 15),
// clamped through the same ClampSidebarWidth bound ComputeLayout itself
// uses so the persisted value and the rendered geometry never disagree.
// m.sidebarWidth's zero-means-default convention (§11.2) is resolved to
// its concrete default before stepping, so `<` from an unset width steps
// down from 35 rather than from 0.
func (m Model) adjustSidebarWidth(delta int) int {
	width, _ := m.frameSize()
	current := m.sidebarWidth
	if current <= 0 {
		current = store.DefaultSidebarWidth
	}
	return ClampSidebarWidth(width, current+delta)
}

// sidebarRowsPerPage is PgUp/PgDn's page size (SPEC requirement 19): the
// number of two-line session rows that actually fit in the sidebar's
// current content height, never fewer than one.
func (m Model) sidebarRowsPerPage() int {
	layout := m.computeLayout()
	rows := layout.Sidebar.Height - 2
	const linesPerRow = 2
	perPage := rows / linesPerRow
	if perPage < 1 {
		perPage = 1
	}
	return perPage
}

// mainView renders the §11.3 chrome: a sidebar and a preview sharing one
// seam in side-by-side/collapsed layouts, or two independently-bordered
// panels stacked vertically in the below-minimum stacked layout, followed by
// the one-line footer outside both panels.
func (m Model) mainView() string {
	width, _ := m.frameSize()
	layout := m.computeLayout()
	lines := m.startupBanner(width)
	lines = append(lines, m.themeBanner(width)...)
	lines = append(lines, m.sortOrderBanner(width)...)
	lines = append(lines, m.themePickerLines(width)...)
	if layout.Effective == LayoutStacked {
		lines = append(lines, m.renderStackedFrame(layout)...)
	} else {
		lines = append(lines, m.renderSideBySideFrame(layout)...)
	}
	lines = append(lines, m.attachErrorLines(width)...)
	lines = append(lines, m.resumeNoteLines(width)...)
	lines = append(lines, m.undoNoteLines(width)...)
	lines = append(lines, m.deleteUndoNoteLines(width)...)
	lines = append(lines, m.archiveUndoNoteLines(width)...)
	lines = append(lines, m.pendingDeleteLines(width)...)
	lines = append(lines, m.filterStatusLine(width)...)
	lines = append(lines, m.footerLine())
	return strings.Join(lines, "\n")
}

// footerLine is SPEC requirement 20's single footer line: the key legend,
// preceded by the selected row's status reason (task 012) when it has one.
// It never lists a key that is not bound. When the terminal is below
// deck's supported 80x24 minimum (SPEC requirement 14), the footer states
// that instead — renderStackedFrame no longer draws that notice above the
// panels, where it could scroll the sidebar's own top border off screen;
// the footer is the one line SPEC §11.3 guarantees stays on screen. At a
// width narrower than the notice's own 87 columns, truncateToWidth elides
// its tail rather than letting it wrap and push the frame's line count
// past the terminal's actual height (task 010) — a below-minimum terminal
// is, by definition, exactly the case where that budget is tight.
//
// Task 013: reason and legend SHARE the line, within the terminal's own
// width. §7's reasons are prose (`pane failed after the stale frame`) and
// the curated legend is long, so at deck's 80-column minimum the two
// together can exceed the width — and a footer that overflows is a footer
// that wraps into a second physical line, costing the frame a row and
// breaking §11.3's guarantee that this line stays on screen (bubbletea
// clips it instead, mid-glyph, which advertises half a key). So each half
// gets a share: whatever it needs when both fit, otherwise the reason is
// held to half the line and elided (its full text is in the `i` detail,
// where §11.3 puts a long one) and the legend takes the rest, dropping
// whole trailing entries. Neither half can squeeze the other out
// entirely, which is what "shares the line" has to mean to be worth
// anything.
func (m Model) footerLine() string {
	if m.computeLayout().BelowMinimum {
		width, _ := m.frameSize()
		return truncateToWidth(belowMinimumNotice, width)
	}
	if m.interactive {
		return m.interactiveFooterLine()
	}
	width, _ := m.frameSize()
	reason := m.selectedRowReason()
	if reason == "" {
		return m.footerLegendWithin(width)
	}
	const gap = "    "
	available := width - len(gap)
	if available <= 0 {
		return m.elideToWidth(reason, width)
	}
	reasonWidth, legendWidth := stringWidth(reason), m.footerLegendWidth()
	if reasonWidth+legendWidth <= available {
		return reason + gap + m.footerKeyLegend()
	}
	reasonBudget := available / 2
	if reasonWidth < reasonBudget {
		reasonBudget = reasonWidth
	}
	return m.elideToWidth(reason, reasonBudget) + gap + m.footerLegendWithin(available-reasonBudget)
}

// interactiveFooterLine is PRD Part II requirement 43's other half: list
// mode's own footer (footerKeyLegend, unchanged in shape) advertises `↵
// interactive`/`a attach`; interactive mode's footer is a DIFFERENT line
// entirely, since none of list mode's bindings are live here -- it states
// plainly that keystrokes are forwarded rather than interpreted, then
// names the one bound exit chord, never listing a key (like `a` or `Y`)
// that interactive mode does not itself bind.
func (m Model) interactiveFooterLine() string {
	forwardNote := m.colorToken(theme.Hint, "keystrokes forward to the live pane")
	sep := m.glyph(" · ", " - ")
	key := m.colorToken(theme.Key, m.glyph("Ctrl+Q", "Ctrl+Q"))
	hint := m.colorToken(theme.Hint, "leave interactive mode")
	return forwardNote + sep + key + " " + hint
}

// belowMinimumNotice is SPEC requirement 14's exact below-minimum copy,
// shared between the footer (footerLine) and the mouse hit-tester so the
// two never disagree about whether it is on screen.
const belowMinimumNotice = "Terminal is below deck's supported minimum of 80x24; showing stacked as far as it fits."

// footerKeyHint pairs one footer key legend entry's key glyph (in both its
// Unicode and DECK_ASCII forms, mirroring m.glyph's own two-string calls
// elsewhere) with the hint word after it. The very first entry (↑/↓) has
// no hint -- it names the up/down keys without a verb, exactly as the
// legend always has -- so hint is left "" there rather than inventing one.
type footerKeyHint struct {
	unicodeKey, asciiKey, hint string

	// eligible reports whether this entry belongs in the footer for the
	// current Model state (SPEC §11.3: "nor does it list a key that would
	// refuse the current selection"). nil means the entry carries no
	// per-row eligibility question at all -- it is a global command that
	// never acts on a row (`n`, `?`, `q`, `↑`/`↓`) -- and it is always
	// shown. A non-nil eligible is always one of the per-action predicates
	// the key handlers themselves call (task 012's
	// canAcknowledge/canKill/canResume/canRestart, plus task 013's
	// canReachPane/canShowDetail/canSwitchProfile/canPinResume), wired
	// through footerRowEligible so the footer can never drift from the key
	// handler's own verdict.
	eligible func(m Model) bool
}

// footerRowEligible answers whether a predicated footer entry belongs in
// the legend: false outright when the list is empty (task 105/§11.3: every
// per-row key vanishes when there is nothing for it to act on), otherwise
// the predicate evaluated over the marked set when batch is true and a
// mark is in force -- ANY marked row the action would actually touch is
// enough to keep the key, mirroring x's own batch handler, which silently
// skips the rows predicate rejects rather than refusing outright -- or
// over the selected row otherwise. Y/r/R never switch to the marked set
// here because their own key handlers (case "Y"/"r"/"R" above) don't
// either: a mark set changes nothing about what pressing those keys does.
func footerRowEligible(m Model, batch bool, predicate func(store.Session) bool) bool {
	if len(m.sessions) == 0 {
		return false
	}
	if batch && len(m.marked) > 0 {
		for _, s := range m.markedSessions() {
			if predicate(s) {
				return true
			}
		}
		return false
	}
	if m.selected < 0 || m.selected >= len(m.sessions) {
		return false
	}
	return predicate(m.sessions[m.selected])
}

// footerLegend is SPEC requirement 20's key legend, SPEC requirement 35's
// `key`/`hint` tokens applied structurally (task 021) rather than as one
// undifferentiated string: every entry's key glyph and hint word are
// tracked separately so footerKeyLegend can colour them apart, while the
// concatenated visible text (key legend and Unicode fallback) is byte-for-
// byte what it always was, and pending tests that grep for a plain
// substring like "up/down" or "Enter interactive" never see the joins move.
var footerLegend = []footerKeyHint{
	{"↑/↓", "up/down", "", nil},
	{"↵", "Enter", "interactive", func(m Model) bool { return footerRowEligible(m, false, canReachPane) }},
	{"a", "a", "attach", func(m Model) bool { return footerRowEligible(m, false, canReachPane) }},
	{"Y", "Y", "acknowledge", func(m Model) bool { return footerRowEligible(m, false, canAcknowledge) }},
	{"n", "n", "new", nil},
	{"x", "x", "kill", func(m Model) bool { return footerRowEligible(m, true, canKill) }},
	{"r", "r", "resume", func(m Model) bool { return footerRowEligible(m, false, canResume) }},
	// The footer's own hint word was originally "relaunch" rather than
	// "restart" purely so this entry's length kept the legend's 100-column
	// wrap boundary (a real 100-column PTY, features/pty_driver_test.go's
	// default) landing on a space that NormalizeFrame's trailing-space trim
	// removes. Task 061 (§11.9, PRD Part II) shifted every entry from here
	// on by inserting the new `a` entry above, moving that boundary; no
	// test pins the exact landing column (only the legend's leading
	// "up/down - Enter ..." text, which this insertion left alone), so the
	// wording is kept as "relaunch" for its own sake now, not for the tuning.
	{"R", "R", "relaunch", func(m Model) bool { return footerRowEligible(m, false, canRestart) }},
	// dd and the eligible one of A/U (task 014, SPEC §11.3's curated fixed
	// set) land here, between R and , -- exactly the order that paragraph
	// lists them in. dd shares x's batch treatment (the mark set, when
	// non-empty, acts on the whole batch for both), while A and U stay on
	// the selected row alone, like Y/r/R above -- their own key handlers
	// (case "A"/case "U") never consult m.marked either. P (profile) and p
	// (pin) are deliberately NOT here: SPEC §11.3 keeps them bound, in the
	// `?` overlay and in §11's keymap, but out of the footer -- one line is
	// a budget, and a rarely-pressed per-row action loses it to dd, the
	// A/U reversal and , (settings has no other visible entry point).
	{"dd", "dd", "delete", func(m Model) bool { return footerRowEligible(m, true, canDelete) }},
	{"A", "A", "archive", func(m Model) bool { return footerRowEligible(m, false, footerArchiveEligible) }},
	{"U", "U", "unarchive", func(m Model) bool { return footerRowEligible(m, false, canUnarchive) }},
	{",", ",", "settings", nil},
	{"i", "i", "detail", func(m Model) bool { return footerRowEligible(m, false, canShowDetail) }},
	{"?", "?", "help", nil},
	{"q", "q", "quit", nil},
}

// footerKeyLegend renders footerLegend into the footer's key legend line,
// each entry's key glyph in the `key` token and its hint word in the
// `hint` token (SPEC requirement 35), joined by the same " · "/" - "
// separator the legend has always used -- never coloured itself, so it
// reads as neutral punctuation between differently-coloured runs rather
// than borrowing either token. Task 013: an entry whose eligible predicate
// (footerRowEligible over one of task 012's own canX functions) rejects
// the current selection -- or the marked set, for the one action (x) that
// switches to it -- is skipped entirely, never rendered as a disabled or
// greyed key; SPEC §11.3 is that the footer either lists a key or it
// doesn't, with nothing in between.
func (m Model) footerKeyLegend() string {
	segments, _ := m.footerLegendSegments()
	return strings.Join(segments, m.glyph(" · ", " - "))
}

// footerLegendSegments renders the eligible entries of footerLegend, one
// styled segment each, paired with the visible width of that segment's
// plain text. Splitting the render from the join is what lets the
// width-aware footer (footerLine) drop whole entries without ever
// measuring an SGR escape as if it occupied cells, and without cutting a
// key glyph or its hint word in half -- a footer showing `Y ackno` claims
// a key that does not exist, which SPEC §11.3 rates worse than no footer.
func (m Model) footerLegendSegments() (segments []string, widths []int) {
	segments = make([]string, 0, len(footerLegend))
	widths = make([]int, 0, len(footerLegend))
	for _, e := range footerLegend {
		if e.eligible != nil && !e.eligible(m) {
			continue
		}
		key := m.glyph(e.unicodeKey, e.asciiKey)
		seg := m.colorToken(theme.Key, key)
		plain := key
		if e.hint != "" {
			seg += " " + m.colorToken(theme.Hint, e.hint)
			plain += " " + e.hint
		}
		segments = append(segments, seg)
		widths = append(widths, stringWidth(plain))
	}
	return segments, widths
}

// footerLegendWidth is the visible width footerKeyLegend needs to render
// in full for the current state.
func (m Model) footerLegendWidth() int {
	_, widths := m.footerLegendSegments()
	sep := stringWidth(m.glyph(" · ", " - "))
	total := 0
	for i, w := range widths {
		if i > 0 {
			total += sep
		}
		total += w
	}
	return total
}

// footerLegendWithin renders the legend into at most budget cells. When
// the eligible keys fit, this is exactly footerKeyLegend. When they do
// not, whole trailing entries are dropped -- never a partial one -- and
// the elision is marked with `…` (`...` under DECK_ASCII) in the `hint`
// token, so the line stays the one line SPEC §11.3 guarantees stays on
// screen and the user can see that the legend is not the whole story (the
// `?` overlay and §11's keymap remain the complete list). Only a budget
// too small for even the first entry plus that marker falls back to a
// plain clip, which is a terminal narrower than any deck supports.
func (m Model) footerLegendWithin(budget int) string {
	segments, widths := m.footerLegendSegments()
	sep := m.glyph(" · ", " - ")
	sepWidth := stringWidth(sep)
	if m.footerLegendWidth() <= budget {
		return strings.Join(segments, sep)
	}
	marker := m.colorToken(theme.Hint, m.glyph("…", "..."))
	markerWidth := stringWidth(marker)
	kept, width := 0, 0
	for i, w := range widths {
		next := width + w
		if i > 0 {
			next += sepWidth
		}
		if next+sepWidth+markerWidth > budget {
			break
		}
		width, kept = next, i+1
	}
	if kept == 0 {
		return truncateToWidth(strings.Join(segments, sep), budget)
	}
	return strings.Join(segments[:kept], sep) + sep + marker
}

// elideToWidth clips s to budget cells, marking the clip with `…`
// (`...` under DECK_ASCII) whenever anything was actually dropped, so an
// elided status reason never reads as the whole reason -- the full text is
// in the `i` detail dialog, which is where SPEC §11.3 puts a long one.
func (m Model) elideToWidth(s string, budget int) string {
	if stringWidth(s) <= budget {
		return s
	}
	marker := m.glyph("…", "...")
	if budget <= stringWidth(marker) {
		return truncateToWidth(s, budget)
	}
	return truncateToWidth(s, budget-stringWidth(marker)) + marker
}

// renderSideBySideFrame draws two panels sharing one seam (SPEC requirement
// 18): the sidebar draws its top/left/bottom borders only, and the
// preview's own left border is the seam, so there is exactly one vertical
// bar between them, never "││". This same shape also draws the collapsed
// strip (a narrower "sidebar" panel beside a wider preview); task 015 fills
// in the strip's own content.
func (m Model) renderSideBySideFrame(layout LayoutResult) []string {
	sw, pw := layout.Sidebar.Width, layout.Preview.Width
	height := layout.Sidebar.Height
	contentRows := height - 2
	if contentRows < 0 {
		contentRows = 0
	}
	collapsed := layout.Effective == LayoutCollapsed
	sidebarTop := m.sidebarTopLine(sw, m.sidebarTitleText())
	var sidebar []string
	var sidebarBg []theme.Token
	if collapsed {
		// The 3-column strip has no room for the "deck — sessions" title
		// and draws its own attention-count content instead of session
		// rows (task 015, SPEC requirement 15).
		sidebarTop = m.sidebarTopLine(sw, "")
		sidebar = fitLines(m.collapsedStripLines(), contentRows)
	} else {
		// sidebarVisibleEntries applies the wheel-scroll offset (task 028)
		// through the identical entries hitTest resolves clicks against.
		visible := m.sidebarVisibleEntries(max(sw-2, 0), contentRows)
		sidebar = make([]string, len(visible))
		sidebarBg = make([]theme.Token, len(visible))
		for i, e := range visible {
			sidebar[i] = e.text
			sidebarBg[i] = e.bg
		}
	}
	preview := m.previewBodyLines(max(pw-4, 0), contentRows)
	lines := make([]string, 0, height)
	lines = append(lines, sidebarTop+m.previewTopLine(pw, m.previewTitle(), true))
	for i := 0; i < contentRows; i++ {
		var sidebarLine string
		if collapsed {
			sidebarLine = m.collapsedStripContentLine(sw, sidebar[i])
		} else {
			sidebarLine = m.sidebarContentLine(sw, sidebar[i], sidebarBg[i])
		}
		lines = append(lines, sidebarLine+m.previewContentLine(pw, preview[i]))
	}
	lines = append(lines, m.sidebarBottomLine(sw)+m.previewBottomLine(pw, true))
	return lines
}

// collapsedStripLines is the 3-column collapsed strip's own content (SPEC
// requirement 15): the `»` glyph, then the attention count's digits each
// on their own line so a multi-digit count still reads inside the strip's
// single content column. attentionCount goes through the shared
// NeedsAttention answer (internal/tui/attention.go, task 025), so this
// count always agrees with the sort and `space`.
func (m Model) collapsedStripLines() []string {
	lines := []string{m.glyph("»", ">")}
	for _, r := range strconv.Itoa(m.attentionCount()) {
		lines = append(lines, string(r))
	}
	return lines
}

// attentionCount counts sessions that need attention (SPEC requirements 31,
// 32), via the single shared NeedsAttention answer also used by the
// attention sort and `space` (Model.nextAttentionSelection) — the three can
// never disagree about what counts.
func (m Model) attentionCount() int {
	n := 0
	for _, s := range m.sessions {
		if NeedsAttention(s) {
			n++
		}
	}
	return n
}

// nextAttentionSelection is `space`'s own step (SPEC requirements 31, 32):
// the index of the next *visible* session needing attention, searching
// forward from just after from's VISUAL position and wrapping around the
// whole painted list back through from itself, so a lone session needing
// attention (including the one already selected) is still found rather
// than treated as "nothing needs attention". Wrapping is done over
// visualOrder (painted order), not m.sessions index order, for the same
// reason ↑/↓ and PgUp/PgDn do (002-steering.md): otherwise `space` could
// jump backwards or skip a visible row whenever a workspace's sessions
// are non-adjacent in m.sessions. ok is false only when no visible
// session needs attention at all, in which case the caller must leave
// selection (and everything else) untouched — this function never
// mutates m.sessions or any session's status, only answers where to move.
func (m Model) nextAttentionSelection(from int) (int, bool) {
	order := m.visualOrder()
	n := len(order)
	if n == 0 {
		return from, false
	}
	pos := -1
	for i, idx := range order {
		if idx == from {
			pos = i
			break
		}
	}
	for step := 1; step <= n; step++ {
		i := (pos + step) % n
		idx := order[i]
		if m.isSessionVisible(idx) && NeedsAttention(m.sessions[idx]) {
			return idx, true
		}
	}
	return from, false
}

// renderStackedFrame draws the below-80-column fallback (SPEC §11.2): the
// list and the preview stack vertically with no seam between them, so each
// keeps all four of its own borders.
func (m Model) renderStackedFrame(layout LayoutResult) []string {
	lw, lh := layout.Sidebar.Width, layout.Sidebar.Height
	pw, ph := layout.Preview.Width, layout.Preview.Height
	var lines []string
	if lh >= 2 {
		listRows := lh - 2
		visible := m.sidebarVisibleEntries(max(lw-4, 0), listRows)
		body := make([]string, len(visible))
		bgs := make([]theme.Token, len(visible))
		for i, e := range visible {
			body[i] = e.text
			bgs[i] = e.bg
		}
		sidebarFocused := !m.previewFocused()
		lines = append(lines, m.fullBoxTop(lw, m.sidebarTitleText(), sidebarFocused))
		for i := 0; i < listRows; i++ {
			lines = append(lines, m.fullBoxContentLine(lw, body[i], sidebarFocused, bgs[i]))
		}
		lines = append(lines, m.fullBoxBottom(lw, sidebarFocused))
	}
	if ph >= 2 {
		previewRows := ph - 2
		body := m.previewBodyLines(max(pw-4, 0), previewRows)
		previewFocused := m.previewFocused()
		lines = append(lines, m.fullBoxTop(pw, m.previewTitle(), previewFocused))
		for i := 0; i < previewRows; i++ {
			lines = append(lines, m.fullBoxContentLine(pw, body[i], previewFocused, ""))
		}
		lines = append(lines, m.fullBoxBottom(pw, previewFocused))
	}
	return lines
}

// sidebarTitleText is the plain (uncoloured) form of the sidebar's title,
// used by the stacked layout's fullBoxTop, which has no special-cased
// "deck" colour run the way sidebarTitleLine does.
func (m Model) sidebarTitleText() string {
	return "deck" + m.glyph(" — ", " - ") + "sessions"
}

// sidebarBodyLines is the sidebar's content, before it is fit to the
// panel's actual content height: an optional socket line, the empty state,
// or every session's two-line row (SPEC §11.3: "the empty state and Press n
// copy now live inside the sidebar"). The tmux-unavailable startup note is
// not part of this body — see mainView's full-width banner.
func (m Model) sidebarBodyLines(contentWidth int) []string {
	entries := m.sidebarEntries(contentWidth)
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.text
	}
	return lines
}

// sidebarLineKind distinguishes what a sidebarEntry line actually is, so
// hit-testing (SPEC §11.8, task 028) can resolve a clicked line back to a
// group header or a session row through the exact same content this
// function feeds the renderer -- "exactly one geometry implementation", per
// the task's own success criterion.
type sidebarLineKind int

const (
	sidebarLineOther sidebarLineKind = iota
	sidebarLineHeader
	sidebarLineRow
)

// sidebarEntry is one rendered line of the sidebar body, tagged with enough
// to resolve a click: sidebarLineHeader carries Workspace, sidebarLineRow
// carries SessionIndex (an index into m.sessions), and sidebarLineOther
// (the socket line, the empty state) carries neither.
type sidebarEntry struct {
	text         string
	kind         sidebarLineKind
	workspace    string
	sessionIndex int
	// bg (task 321/R58b) is the selection/selection_idle/surface-stripe
	// background token a sidebarLineRow entry wants painted across the
	// full panel width by sidebarContentLine; "" (the zero value) for
	// every non-row entry (headers, the socket line, the empty-state
	// message), which never carry a background.
	bg theme.Token
}

// sidebarEntries is sidebarBodyLines' one real implementation: every line
// the sidebar can render, each tagged with what it is. sidebarBodyLines
// (the renderer's own text-only view) and hitTest (task 028's click
// resolution) both build on this rather than keeping two descriptions of
// the same content in sync by hand.
func (m Model) sidebarEntries(contentWidth int) []sidebarEntry {
	var entries []sidebarEntry
	if m.settings.Socket != "" {
		for _, line := range wrapText(fmt.Sprintf("socket: %s", m.settings.Socket), contentWidth) {
			entries = append(entries, sidebarEntry{text: line})
		}
	}
	if len(m.sessions) == 0 {
		msg := "No sessions yet. Press n to create a session."
		if m.filterQuery != "" {
			// Task 123/I-10: a filter query in force with zero matches is a
			// different state from a genuinely empty store -- there is
			// nothing to create here, and "press n" would be misleading
			// copy while a query is narrowing an otherwise non-empty list.
			msg = fmt.Sprintf("No sessions match %q.", m.filterQuery)
		}
		for _, line := range wrapText(msg, contentWidth) {
			entries = append(entries, sidebarEntry{text: line})
		}
		return entries
	}
	if !m.groupingEnabled() {
		// Requirement 35: grouping off renders one flat list, in
		// visualOrder() (identity order when ungrouped), with ZERO header
		// rows -- not a header with a blank/omitted label, no header
		// sidebarEntry at all, so a scan for sidebarLineHeader entries
		// finds none. Collapse state cannot apply (there is no group to
		// collapse), so nothing here ever calls
		// isGroupCollapsed/setGroupCollapsed.
		//
		// Task 084's stripe phase is this loop's own position (range's
		// second value), NOT the session index idx -- the two only diverge
		// once grouping is on, but keeping the same "running position over
		// rendered session rows" idea in both branches means a group
		// toggle never has to reconcile two different phase sources.
		for pos, idx := range m.visualOrder() {
			lines, bg := m.sidebarRowLines(idx, m.sessions[idx], pos%2 == 1)
			for _, line := range lines {
				entries = append(entries, sidebarEntry{text: line, kind: sidebarLineRow, sessionIndex: idx, bg: bg})
			}
		}
		return entries
	}
	// Task 084: sessionPos counts only rendered SESSION rows, continuing
	// across group boundaries -- a workspace header never advances or
	// resets it, so the stripe phase a session gets does not depend on
	// which group happens to precede it, and headers themselves stay on
	// theme.Background (sidebarRowLines is never called for a header).
	sessionPos := 0
	for _, group := range m.groupSessions() {
		entries = append(entries, sidebarEntry{text: m.groupHeaderText(group), kind: sidebarLineHeader, workspace: group.Workspace})
		if m.isGroupCollapsed(group.Workspace) {
			continue
		}
		for _, is := range group.Sessions {
			lines, bg := m.sidebarRowLines(is.Index, is.Session, sessionPos%2 == 1)
			for _, line := range lines {
				entries = append(entries, sidebarEntry{text: line, kind: sidebarLineRow, sessionIndex: is.Index, bg: bg})
			}
			sessionPos++
		}
	}
	return entries
}

// resortSessionsLive re-sorts the exact session set already held in
// m.baseSessions/m.sessions for the just-changed [ui] sort_order
// (requirement R53/task 306's settingsApplyLiveFields consumer) -- unlike
// sessionsLoaded's own resort above, which reacts to a fresh ListSessions
// result, this reorders the SAME sessions already on screen, so previous
// and incoming are deliberately the same slice for every
// sortSessionsByOrder/sortSessionsByAttentionStable call below (a
// same-set re-sort, not a reconcile). The selected SESSION is preserved by
// id -- never by index, since that is exactly what a re-sort can change --
// and scrollSessionIntoView keeps its row inside the sidebar's visible
// window, mirroring requirement 52's own one-shot new-session path.
func (m *Model) resortSessionsLive() {
	var selectedID string
	if m.selected >= 0 && m.selected < len(m.sessions) {
		selectedID = m.sessions[m.selected].ID
	}
	order, _ := m.effectiveSortOrder()
	attentionOrder := sortSessionsByAttentionStable(m.baseSessions, m.baseSessions)
	switch {
	case order == SortOrderAttention:
		m.baseSessions = attentionOrder
	case m.groupingEnabled():
		m.baseSessions = reorderPreservingGrouping(attentionOrder, sortSessionsByOrder(m.baseSessions, m.baseSessions, order))
	default:
		m.baseSessions = sortSessionsByOrder(m.baseSessions, m.baseSessions, order)
	}
	m.sessions = m.filteredSessions()
	if idx := indexOfSessionID(m.sessions, selectedID); idx >= 0 {
		m.selected = idx
		m.scrollSessionIntoView(idx)
	} else if m.selected >= len(m.sessions) {
		m.selected = max(0, len(m.sessions)-1)
	}
}

// scrollSessionIntoView adjusts m.sidebarScroll (SPEC requirement 52) so
// that the session at m.sessions[sessionIndex]'s row is fully within the
// sidebar's current content window, moving the offset the minimum amount
// necessary -- up if the row starts above the window, down if any of its
// lines fall below it -- and leaving it untouched if the row is already
// fully visible. Ordinary selection moves (↑/↓, PgUp/PgDn) never auto-
// scroll today; this one-shot path exists so landing on a freshly created
// session is visible without an extra keystroke, without changing that
// general behaviour. A sessionIndex not currently present as a row entry
// (e.g. its group is collapsed) leaves the scroll offset untouched.
func (m *Model) scrollSessionIntoView(sessionIndex int) {
	layout := m.computeLayout()
	contentWidth := max(layout.Sidebar.Width-2, 0)
	contentHeight := layout.Sidebar.Height - 2
	if contentHeight <= 0 {
		return
	}
	entries := m.sidebarEntries(contentWidth)
	start, end := -1, -1
	for i, e := range entries {
		if e.kind == sidebarLineRow && e.sessionIndex == sessionIndex {
			if start == -1 {
				start = i
			}
			end = i
		}
	}
	if start == -1 {
		return
	}
	offset := m.sidebarScroll
	if start < offset {
		offset = start
	}
	if end >= offset+contentHeight {
		offset = end - contentHeight + 1
	}
	m.sidebarScroll = clampSidebarScroll(offset, len(entries), contentHeight)
}

// clampSidebarScroll bounds a raw scroll offset into [0, max(0,
// total-contentHeight)] (SPEC §11.8's wheel scroll, task 028): never
// negative, and never past the point where the last line would leave the
// panel's bottom empty while there is still content to show.
func clampSidebarScroll(raw, total, contentHeight int) int {
	maxScroll := total - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if raw < 0 {
		return 0
	}
	if raw > maxScroll {
		return maxScroll
	}
	return raw
}

// sidebarVisibleEntries returns exactly contentHeight sidebarEntry values
// currently shown in the sidebar's content area, applying the wheel-scroll
// offset (task 028). Both renderSideBySideFrame/renderStackedFrame's actual
// drawing and hitTest's click resolution call this same function, so a
// scrolled frame and a click against it can never disagree about which
// entry is on which line.
func (m Model) sidebarVisibleEntries(contentWidth, contentHeight int) []sidebarEntry {
	entries := m.sidebarEntries(contentWidth)
	offset := clampSidebarScroll(m.sidebarScroll, len(entries), contentHeight)
	var visible []sidebarEntry
	if offset < len(entries) {
		visible = entries[offset:]
	}
	if len(visible) > contentHeight {
		visible = visible[:contentHeight]
	}
	out := make([]sidebarEntry, contentHeight)
	copy(out, visible)
	return out
}

// sidebarRowLines is one session's two-line row: glyph/marker, name, its
// unseen glyph and status/quality badges on the first line (SPEC §11.3,
// task 012 — no reason text), and its permission-profile badge plus
// creation time on the second. It intentionally omits the row's cwd and
// agent kind that the pre-chrome list used to print: at the sidebar's
// default 35-column width (33 content columns) there is no room for both a
// name and a full path on one line, and a wrapped second cwd line would
// halve how many sessions fit on screen. Both remain one keystroke away in
// the `i` detail dialog; a fuller compact row (matching SPEC §11's
// illustrated `● api-refactor claude live waiting 2m`) is Phase 2b-1's
// attention-sort/grouping work (tasks 023/024), which already has to touch
// this row's shape for the status glyph and workspace headers.
//
// The unseen glyph and quality badge are only added to the first line's
// badge run when each actually has something to say, rather than reserving
// a fixed-width placeholder column for each, and the profile badge moves to
// the second line entirely: features/status_probe.feature and
// features/concurrency.feature both require a session's real name plus its
// quality word ("live"/"sampled") or status word on the very same rendered
// line (clientRowContainsWithinReconcile, features/status_probe_test.go),
// and a realistic name (10-22 columns seen across features/*.feature) plus
// a profile badge plus a quality badge plus a status word does not fit
// inside 33 columns; nothing tested requires the profile badge on that same
// line, so it is the one dropped rather than risk ellipsis-truncating a
// word an assertion depends on.
func (m Model) sidebarRowLines(index int, session store.Session, stripe bool) ([]string, theme.Token) {
	marker := "  "
	selected := index == m.selected
	if selected {
		marker = "> "
	}
	// Task 084 (steer 006 item 2): the alternating background stripe uses
	// theme.Surface for every row in this session's block (both lines
	// share the one phase the caller computed). Task 321 (R58b) moved the
	// actual painting of this background out of this function entirely --
	// it used to be opened here per line via settingsRenderRow's own bg/
	// idleBg and closed the moment that line's text ended, which is
	// exactly why the highlight used to stop short of the panel's full
	// width. sidebarRowBackground below now just answers WHICH token (if
	// any) this row wants; sidebarContentLine is the one that opens it,
	// spanning the leading pad column, the text, ITS pad-fill, and the
	// trailing pad column, and closes it once at the very end.
	bg := m.sidebarRowBackground(selected, stripe)
	// SPEC requirement 35's `dimmed` covers a starting row (task 021): it
	// carries no signal yet beyond its own liveness, so everything but the
	// status word itself -- coloured in its own starting token below, which
	// must keep reading as "starting" rather than fade to grey -- is dimmed
	// so the eye is not drawn to it the way a row with real news is.
	nameTok := theme.Title
	if session.Status == "starting" {
		nameTok = theme.Dimmed
	}
	// Every run of text on this row is composed as a settingsRowSegment
	// (the same generic label/value/selection-background helper task 019
	// built for the settings takeover, reused here rather than duplicated)
	// so a selected row's `selection` BACKGROUND (SPEC requirement 42) can
	// be opened once and stay live under every foreground-coloured segment
	// -- composing self-resetting m.colorToken calls into one string here,
	// the way this row used to, would have the FIRST inner segment's own
	// trailing \x1b[0m clear that background again immediately (the
	// gotcha theme_color.go's foregroundSGR/backgroundSGR doc already
	// warns about).
	segs := []settingsRowSegment{{Text: marker + session.Name + " ", Tok: nameTok}}
	var parts []settingsRowSegment
	if !session.Acknowledged && (session.Status == "waiting" || session.Status == "error") {
		unseen := m.glyph("●", "!")
		tok := theme.Text
		if t, ok := statusToken(session.Status); ok {
			tok = t
		}
		parts = append(parts, settingsRowSegment{Text: unseen, Tok: tok})
	}
	if quality := statusSourceQuality(session.StatusSource); quality != "" {
		parts = append(parts, settingsRowSegment{Text: quality, Tok: theme.Dimmed})
	}
	statusTok := theme.Text
	if t, ok := statusToken(session.Status); ok {
		statusTok = t
	}
	parts = append(parts, settingsRowSegment{Text: session.Status, Tok: statusTok})
	// SPEC requirement 27: archived_at is a FLAG, never a status -- the
	// ▣ glyph is rendered from that flag directly, never from any
	// notion of session.Status == "archived" (there is no such status;
	// the six SPEC-enumerated ones are untouched by this task, see
	// attention.go). An archived row keeps whatever status word it
	// already had above, so this is an ADDITIONAL badge, not a
	// replacement.
	if session.ArchivedAt != 0 {
		parts = append(parts, settingsRowSegment{Text: m.glyph("\u25a3", "[archived]"), Tok: theme.Archived})
	}
	// Task 112: the mark badge is driven from m.marked by session id
	// alone -- never a visual index -- so it stays attached to the right
	// row through a re-sort or re-group exactly like every other lookup
	// keyed off m.marked (markedSessions()).
	if m.marked[session.ID] {
		parts = append(parts, settingsRowSegment{Text: m.glyph("\u2713 marked", "[marked]"), Tok: theme.Badge})
	}
	for i, p := range parts {
		if i > 0 {
			segs = append(segs, settingsRowSegment{Text: " ", Tok: theme.Text})
		}
		segs = append(segs, p)
	}
	line1 := m.settingsRenderRowOpen(segs)

	// Both the default (steer 006) and the starting-row override (SPEC
	// requirement 35 / task 021) resolve to theme.Dimmed now, so there is
	// nothing left for a starting row to override on line 2 -- unlike
	// nameTok above, where Title vs. Dimmed still differ.
	line2Tok := theme.Dimmed
	line2Segs := []settingsRowSegment{{Text: "  ", Tok: theme.Text}}
	if text, tok, ok := m.profileBadgeSegment(session); ok {
		line2Segs = append(line2Segs, settingsRowSegment{Text: text, Tok: tok}, settingsRowSegment{Text: " ", Tok: theme.Text})
	}
	// SPEC §6.1/§6.3, task 021: env_dirty means a session-env edit has been
	// persisted and mirrored into tmux's own environment table for future
	// panes, but has NOT yet reached the pane's already-running process --
	// only an explicit restart (task 022's `R`) applies it and clears the
	// badge. Shown next to the profile badge (no room on line1, same
	// reasoning sidebarRowLines' own doc already states for that badge).
	if session.EnvDirty {
		line2Segs = append(line2Segs, settingsRowSegment{Text: m.glyph("env\u21bb", "env*"), Tok: theme.BadgeWarn}, settingsRowSegment{Text: " ", Tok: theme.Text})
	}
	line2Segs = append(line2Segs, settingsRowSegment{Text: "created " + m.relativeTime(session.CreatedAt), Tok: line2Tok})
	line2 := m.settingsRenderRowOpen(line2Segs)
	return []string{line1, line2}, bg
}

// sidebarRowBackground (task 321/R58b) answers which background token, if
// any, a session row's two lines should be highlighted with: the focus-
// aware selection/selection_idle token (SPEC requirement 42) when this row
// is selected -- that always wins -- otherwise the alternating surface
// stripe (task 084/steer 006 item 2) when this row's block is the odd
// phase, otherwise none at all (theme.Token("")). sidebarContentLine is the
// only caller that actually paints it.
func (m Model) sidebarRowBackground(selected, stripe bool) theme.Token {
	if selected {
		return m.sidebarSelectionToken()
	}
	if stripe {
		return theme.Surface
	}
	return theme.Token("")
}

// previewTitle is the preview panel's border title. Outside interactive
// mode it intentionally never embeds the selected session's name: every
// helper across this package and features/ that locates "a session's row"
// does so by finding the first screen line containing that name, and the
// preview's top border shares a screen line with the sidebar's own top
// border (row 0) -- embedding the name there would make it the *first*
// match for the selected session, ahead of its real row, and silently
// break every such lookup.
//
// While m.interactive is true this changes (SPEC requirement 44/task 063):
// the name IS embedded, naming which session's window now owns the
// preview panel, because focus has moved off the sidebar and NO_COLOR
// drops deck to monochrome -- a colour-only focus cue (border_focus vs.
// border) would pass a golden-frame diff whether or not focus actually
// moved, so the label must be legible as plain screen text too. The
// row-lookup collision above is accepted deliberately, scoped to this one
// mode: outside it (the only time list-mode row lookups/clicks run) the
// name is still never in the title, so nothing that locates a row by name
// in list mode is affected.
//
// PRD Part II requirement 46 also lands here: interactive mode fits the
// window to the panel's own content box (enterInteractive), so
// contentWidth/Height and the real pane size are always equal -- stating
// that in cropPreviewBottomLeft's own "WxH of realWxrealH" form would
// degenerate to the misleading "45x22 of 45x22" (a crop statement about a
// pane that was never cropped). The title states it instead, in a form
// that cannot be mistaken for a crop: "WxH fitted", no "of", no second
// pair of dimensions, since fitted the two are the same number by
// construction. This is a border-title addition, not a body line, so it
// never steals a row from the live grid the way a crop's geometry line
// steals one from previewBodyLines' content budget.
func (m Model) previewTitle() string {
	if m.interactive {
		name := ""
		if m.selected >= 0 && m.selected < len(m.sessions) {
			name = m.sessions[m.selected].Name + " "
		}
		width, height := m.previewContentSize()
		geom := fmt.Sprintf("%dx%d fitted", width, height)
		return " " + name + "interactive " + geom + " " + m.glyph("—", "-") + " Ctrl+Q to leave "
	}
	return ""
}

// previewBodyLines is the preview panel's content: the real cropped pane
// capture (task 018, SPEC requirement 23) when the selected session has a
// live capture on file, otherwise a placeholder naming exactly which
// no-live-pane state applies (task 020, SPEC requirement 26) -- an `error`
// row's stored crash tail, or a one-line placeholder for `stopped`,
// `archived` and a `starting` row with no pane yet. Stale bytes are never
// presented as live: the placeholder branch is only reached when the most
// recent capture for this exact session id found no live pane (or none has
// been attempted yet). It always returns exactly contentHeight lines, each
// exactly contentWidth runes, so callers no longer need their own fitLines
// pass for the preview panel.
func (m Model) previewBodyLines(contentWidth, contentHeight int) []string {
	if m.interactive && m.interactiveGrid != nil {
		return m.interactiveBodyLines(contentWidth, contentHeight)
	}
	if len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return fitLines(wrapText("Select or create a session to preview it here.", contentWidth), contentHeight)
	}
	session := m.sessions[m.selected]
	if m.previewLive && m.previewSessionID == session.ID {
		return m.cropPreviewBottomLeft(m.previewBytes, contentWidth, contentHeight, m.previewPaneWidth, m.previewPaneHeight)
	}
	return fitLines(m.previewPlaceholderLines(session, contentWidth, contentHeight), contentHeight)
}

// previewPlaceholderLines names, rather than papers over, why the preview
// has nothing live to show for the selected session (task 020, SPEC
// requirement 26). `error` gets the durable crash tail headed by copy
// stating it is the last output before the exit and is not live; the other
// three named states get a single line naming the state and nothing else
// that could be mistaken for live pane content. Any other status reaching
// here (e.g. a row not yet captured on the very first tick after
// selection) keeps the original CWD-plus-notice copy.
func (m Model) previewPlaceholderLines(session store.Session, contentWidth, contentHeight int) []string {
	switch session.Status {
	case "error":
		return m.crashTailPreviewLines(session.CrashTail, contentWidth, contentHeight)
	case "stopped":
		return wrapText("Session is stopped. No live preview to show.", contentWidth)
	case "archived":
		return wrapText("Session is archived. No live preview to show.", contentWidth)
	case "starting":
		return wrapText("Session is starting; no pane yet.", contentWidth)
	default:
		var lines []string
		lines = append(lines, wrapText(session.CWD, contentWidth)...)
		lines = append(lines, "")
		lines = append(lines, wrapText("No live preview captured for this row yet.", contentWidth)...)
		return lines
	}
}

// crashTailPreviewLines renders an `error` row's durable §7 crash tail into
// the preview panel (task 020, SPEC requirement 26), headed by copy that
// states plainly it is the last output before the process exited and is
// not live -- never letting a stale capture be mistaken for a live pane.
// The tail is anchored bottom, mirroring requirement 23's own bottom-left
// crop anchoring: when the stored tail has more lines than the panel has
// room for, the newest (last) lines win.
func (m Model) crashTailPreviewLines(tail string, contentWidth, contentHeight int) []string {
	header := wrapText(m.glyph("Last output before exit \u2014 not live:", "Last output before exit - not live:"), contentWidth)
	if tail == "" {
		return append(header, wrapText("No crash output was captured.", contentWidth)...)
	}
	var body []string
	for _, raw := range strings.Split(strings.Trim(tail, "\n"), "\n") {
		body = append(body, wrapText(raw, contentWidth)...)
	}
	budget := contentHeight - len(header)
	if budget < 0 {
		budget = 0
	}
	if len(body) > budget {
		body = body[len(body)-budget:]
	}
	return append(header, body...)
}

// selectedRowReason returns the reason text belonging to whichever row is
// currently selected (SPEC §11.3, task 012): the row itself only ever shows
// the bare status word now, so the one piece of "why" a user is actually
// looking at gets a stable home on the footer's left, clearly separated from
// the key legend, rather than jittering every row's width as sessions move
// between statuses. It returns "" when the selected session has no reason to
// show (e.g. running, or a starting shell whose only signal is its own
// liveness).
//
// The two derived reasons come first, because they say more than the
// stored one does: a stopped row's `resumable` names the way out, and an
// unsignalled agent's `awaiting signal` explains a status that otherwise
// looks stuck. Otherwise the row's own stored StatusReason is the reason
// (task 013) — that is where §7's prose verdicts live, including the long
// ones like `pane failed after the stale frame`, and the footer was always
// meant to be where the selected row's "why" is legible without opening
// the `i` detail.
func (m Model) selectedRowReason() string {
	if len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return ""
	}
	session := m.sessions[m.selected]
	switch {
	case session.Status == "stopped":
		return session.Status + m.glyph(" · resumable", " - resumable")
	case session.Status == "starting" && session.Agent != "shell":
		return "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
	case session.StatusReason != "":
		return session.Status + m.glyph(" · ", " - ") + session.StatusReason
	default:
		return ""
	}
}

// profileBadge renders the bracketed permission-profile badge shown next to
// an agent row in the session list. Shell sessions have no notion of a
// permission profile at all (SPEC §5/§8): agentCapabilities reports
// them as not applicable, so they render no badge rather than a meaningless
// cosmetic "safe".
func (m Model) profileBadgeSegment(session store.Session) (text string, tok theme.Token, ok bool) {
	if _, applicable := m.agentCapabilities(session.Agent); !applicable {
		return "", "", false
	}
	// SPEC's builtin themes comment badge_warn as "non-safe permission
	// profiles, yolo" (task 021): `safe` is merely informational (`badge`),
	// every other profile -- plan, edits, yolo -- is the thing worth a
	// warning colour.
	tok = theme.Badge
	if session.PermissionProfile != "safe" {
		tok = theme.BadgeWarn
	}
	return "[" + session.PermissionProfile + "]", tok, true
}

// profileBadge self-colours profileBadgeSegment's text for a caller that
// just wants a ready-to-print badge; sidebarRowLines instead composes the
// segment directly so a selected row's `selection` background stays live
// underneath it (see settingsRenderRow's own doc on why a self-resetting
// colorToken call must never be nested inside one).
func (m Model) profileBadge(session store.Session) string {
	text, tok, ok := m.profileBadgeSegment(session)
	if !ok {
		return ""
	}
	return m.colorToken(tok, text)
}

// statusSourceQuality describes only the quality of an agent verdict. Hook
// events are live and pane probes are sampled; user and tmux transitions make
// no claim about what an agent is doing and therefore receive no badge.
func statusSourceQuality(source string) string {
	switch source {
	case "hook":
		return "live"
	case "probe":
		return "sampled"
	default:
		return ""
	}
}

// updateProfileSwitch handles keys while the `P` permission-profile switch
// dialog is open (task 020). It only ever cycles a locally-held candidate
// value and, on confirmation, persists it through m.profileSwitch; it never
// issues any argv to the selected session's pane, live or otherwise.
func (m Model) updateProfileSwitch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	options := m.createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	cycle := func(delta int) {
		m.profileSwitchValue = cycleOption(options, m.profileSwitchValue, delta)
	}
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: cycle},
		Cancel: func() {
			m.profileSwitching = false
			m.profileSwitchNote = ""
		},
		Submit: func() tea.Cmd {
			if m.profileSwitch == nil {
				m.profileSwitchNote = "changing the permission profile is unavailable"
				return nil
			}
			sessionID, profile := session.ID, m.profileSwitchValue
			return func() tea.Msg {
				updated, err := m.profileSwitch(context.Background(), sessionID, profile)
				return profileSwitched{session: updated, err: err}
			}
		},
	}); handled {
		return m, cmd
	}
	return m, nil
}

// profileSwitchView renders the `P` permission-profile switch dialog. It
// states plainly that the change only applies on the session's next
// launch/restart; it never claims the live pane's mode changed (SPEC §5).
func (m Model) profileSwitchView() string {
	session := m.sessions[m.selected]
	options := m.createProfileOptionsFor(session.Agent, m.settings.AllowYolo)
	var b strings.Builder
	fmt.Fprintf(&b, "Change permission profile for %s\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Current:   ", session.PermissionProfile))
	fmt.Fprintf(&b, "%s\n", m.detailField("New:       ", fmt.Sprintf("%s (left/right cycles: %s)", m.profileSwitchValue, strings.Join(options, ", "))))
	b.WriteString("\nThis applies on the session's next launch/restart; it does not change a\nrunning pane's mode.\n")
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.profileSwitchNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.profileSwitchNote)
	}
	return m.framedDialog(b.String())
}

// resumeModeOptions lists the SPEC §8/§9.3 resume_state choices the `p`
// dialog cycles through: auto (resume the session's own last-known
// conversation), pinned (always resume the session's own current
// conversation id, sticky across a deck restart), and fresh-once (start a
// brand-new conversation exactly once, then revert to auto).
var resumeModeOptions = []string{"auto", "pinned", "fresh-once"}

// updatePinDialog handles keys while the `p` pin/start-fresh dialog is open
// (task 021). It only ever cycles a locally-held candidate resume_state
// and, on confirmation, persists it through m.resumeMode; it never issues
// any argv to the selected session's pane, live or otherwise, and takes
// effect only on the session's next resume.
func (m Model) updatePinDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: func(delta int) {
			m.pinValue = cycleOption(resumeModeOptions, m.pinValue, delta)
		}},
		Cancel: func() {
			m.pinning = false
			m.pinNote = ""
		},
		Submit: func() tea.Cmd {
			if m.resumeMode == nil {
				m.pinNote = "changing the resume mode is unavailable"
				return nil
			}
			sessionID, mode := session.ID, m.pinValue
			return func() tea.Msg {
				updated, err := m.resumeMode(context.Background(), sessionID, mode)
				return resumeModeChanged{session: updated, err: err}
			}
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// pinView renders the `p` pin/start-fresh dialog. "pinned" always resumes
// the session's own current conversation id, sticky across a deck restart;
// "fresh-once" starts a brand-new conversation exactly once and then
// reverts to auto; neither ever touches a live pane.
func (m Model) pinView() string {
	session := m.sessions[m.selected]
	state := session.ResumeState
	if state == "" {
		state = "auto"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Change resume mode for %s\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Current:   ", state))
	fmt.Fprintf(&b, "%s\n", m.detailField("New:       ", fmt.Sprintf("%s (left/right cycles: %s)", m.pinValue, strings.Join(resumeModeOptions, ", "))))
	b.WriteString("\npinned always resumes this session's own current conversation id, sticky\nacross a deck restart. fresh-once starts a brand-new conversation exactly\nonce, then reverts to auto. Neither changes a running pane.\n")
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.pinNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.pinNote)
	}
	return m.framedDialog(b.String())
}

// restartChoiceOptions lists task 023's `R` choice for a shell session:
// restart (Restart's existing kill-and-relaunch-with-resume-argv, the
// pre-task-023 behaviour, kept as the default candidate so a bare R+Enter
// still restarts exactly as before) or inject (export the keys changed
// since env_dirty was last cleared directly into the live, already-
// running shell, never killing or relaunching its pane).
var restartChoiceOptions = []string{"restart", "inject"}

// updateRestartChoice handles keys while the `R` restart/inject-instead
// choice (task 023, shell sessions only) is open. It only ever cycles a
// locally-held candidate and, on confirmation, dispatches to whichever of
// m.restart/m.inject the candidate names; it never issues anything to the
// selected session's pane itself -- that happens inside the dispatched
// service call, exactly as a direct (non-shell) `R` already does.
func (m Model) updateRestartChoice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	session := m.sessions[m.selected]
	cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: func(delta int) {
			m.restartChoiceValue = cycleOption(restartChoiceOptions, m.restartChoiceValue, delta)
		}},
		Cancel: func() {
			m.restartChoosing = false
			m.restartChoiceNote = ""
		},
		Submit: func() tea.Cmd {
			sessionID := session.ID
			if m.restartChoiceValue == "inject" {
				if m.inject == nil {
					m.restartChoiceNote = "injecting the environment is unavailable"
					return nil
				}
				return func() tea.Msg {
					updated, keys, err := m.inject(context.Background(), sessionID)
					return envInjected{session: updated, keys: keys, err: err}
				}
			}
			if m.restart == nil {
				m.restartChoiceNote = "restarting is unavailable"
				return nil
			}
			return func() tea.Msg {
				restarted, outcome, err := m.restart(context.Background(), sessionID)
				return sessionRestarted{session: restarted, outcome: outcome, err: err}
			}
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// restartChoiceView renders task 023's `R` restart/inject-instead choice
// for a shell session. It states plainly what each option does and does
// NOT do, so the choice is never mistaken for identical outcomes: restart
// kills and relaunches the pane (losing the shell's own running state);
// inject exports the pending change into the SAME already-running shell
// process without ever killing it.
func (m Model) restartChoiceView() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "Restart or inject for %s\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Choice:     ", fmt.Sprintf("%s (left/right cycles: %s)", m.restartChoiceValue, strings.Join(restartChoiceOptions, ", "))))
	b.WriteString("\nrestart kills this session's pane and relaunches it with the resume argv\n(same conversation id), losing whatever state the running shell had.\n")
	b.WriteString("inject exports the pending environment change into the SAME live shell\nprocess via export -- the pane is never killed or relaunched.\n")
	b.WriteString("\nLeft/Right cycles · Enter confirms · Esc cancels\n")
	if m.restartChoiceNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.restartChoiceNote)
	}
	return m.framedDialog(b.String())
}

// deletePurgeOptions lists task 110's non-default "purge conversation"
// choice inside the dd confirm dialog: "keep" (the default candidate,
// so a bare dd+Enter still deletes exactly as task 105 always has) or
// "purge" (additionally remove the agent's own declared transcript file,
// SPEC.md:684-691 -- never implicit, never offered anywhere else).
var deletePurgeOptions = []string{"keep", "purge"}

// updateDeleteConfirm handles keys while task 105's second-`d` confirm
// dialog is open. It has no navigable fields (nothing to cycle, nothing
// to tab between) -- only Esc (cancel, tombstones nothing) and Enter
// (submit: kill the live pane if any, then tombstone the row) -- so it
// defers to the shared §11.4 contract exactly like detailView/helpView's
// esc-only case does, just with a Submit as well. Task 112: a non-empty
// mark set at the moment this dialog opened routes to
// updateBulkDeleteConfirm instead -- the two never run at the same time
// (m.marked and the single-session deletePurge* trio are set by mutually
// exclusive branches of the second-`d` intercept above).
func (m Model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.marked) > 0 {
		return m.updateBulkDeleteConfirm(msg)
	}
	session := m.sessions[m.selected]
	cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{Cycle: func(delta int) {
			m.deletePurgeValue = cycleOption(deletePurgeOptions, m.deletePurgeValue, delta)
		}},
		Cancel: func() {
			m.deleteConfirming = false
			m.deleteNote = ""
			m.deletePurgeValue = ""
			m.deletePurgePath = ""
			m.deletePurgeOK = false
		},
		Submit: func() tea.Cmd {
			if m.deleteSvc == nil {
				m.deleteNote = "deleting is unavailable"
				return nil
			}
			purgePath := ""
			if m.deletePurgeValue == "purge" && m.deletePurgeOK {
				purgePath = m.deletePurgePath
			}
			purgeSvc := m.purgeSvc
			return func() tea.Msg {
				err := m.deleteSvc(context.Background(), session)
				var purgeErr error
				if err == nil && purgePath != "" {
					if purgeSvc == nil {
						purgeErr = errors.New("purging the transcript is unavailable")
					} else {
						purgeErr = purgeSvc(context.Background(), purgePath)
					}
				}
				return sessionDeleted{session: session, err: err, purgeErr: purgeErr}
			}
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// updateBulkDeleteConfirm is task 112's marked-set second-`d` confirm: no
// Fields.Cycle at all (purge is not offered for a bulk delete -- it
// resolves one session's one declared transcript path at a time, task
// 109/110 -- so left/right/space are simply not contract keys here, which
// applyDialogContract already handles via a nil Cycle). Submit clears the
// mark set THE MOMENT it fires ("the marks clear on the action"), before
// the async delete loop even runs, using the session list resolved right
// now rather than re-reading m.marked after it is gone.
func (m Model) updateBulkDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sessions := m.markedSessions()
	cmd, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() {
			m.deleteConfirming = false
			m.deleteNote = ""
			m.marked = nil
		},
		Submit: func() tea.Cmd {
			if m.deleteSvc == nil {
				m.deleteNote = "deleting is unavailable"
				return nil
			}
			deleteSvc := m.deleteSvc
			m.marked = nil
			return func() tea.Msg {
				result := sessionsBulkDeleted{}
				for _, s := range sessions {
					result.sessions = append(result.sessions, s)
					result.errs = append(result.errs, deleteSvc(context.Background(), s))
				}
				return result
			}
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// updateArchiveConfirm handles keys while R72's `A` confirm dialog is open
// (issue #10, SPEC.md:752). It is deliberately deleteConfirming's shape with
// one field fewer: no navigable fields at all (archiving offers no purge
// choice and no options -- SPEC.md:751 keeps purge to the delete confirm
// alone), so only Esc (cancel, writes nothing) and Enter (submit: kill the
// live pane if any, then set archived_at) are contract keys, exactly as
// updateBulkDeleteConfirm's own nil Cycle already handles. Every other key
// -- including a second `A`, `x`, `dd` or `r` -- is swallowed here rather
// than reaching the bare-letter keymap, which is what "the dialog suppresses
// the keymap" means in this codebase and why nothing surprising happens
// inside it.
func (m Model) updateArchiveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.sessions) == 0 {
		m.archiveConfirming = false
		m.archiveNote = ""
		return m, nil
	}
	session := m.sessions[m.selected]
	cmd, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() {
			m.archiveConfirming = false
			m.archiveNote = ""
		},
		Submit: func() tea.Cmd {
			if m.archiveSvc == nil {
				m.archiveNote = "archiving is unavailable"
				return nil
			}
			archiveSvc := m.archiveSvc
			return func() tea.Msg {
				return sessionArchived{session: session, err: archiveSvc(context.Background(), session)}
			}
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// archiveConfirmView renders R72's `A` confirm (issue #10). Per SPEC.md:752
// and §11.4's pre-existing dialog rule at :1248 ("the confirmation names the
// target and what will survive it") it names the session, says what survives
// -- the whole record, reachable again through `/` and reversible with `U` --
// and, when the row is not stopped, states in as many words that confirming
// kills the live agent first. Nothing is written until Enter.
func (m Model) archiveConfirmView() string {
	return m.framedDialog(m.archiveConfirmBody())
}

// archiveConfirmBody builds archiveConfirmView's text before framedDialog's
// box-width padTrunc touches it, split out for the same reason
// deleteConfirmBody is: a test can assert the exact wording (above all the
// live-agent sentence, which is the whole point of this dialog) without a
// terminal-rendering concern in between. The live-agent sentence is keyed on
// the row's own Status, never on a cached flag: `A` on a stopped row writes
// only archived_at, so promising a kill there would be a lie in the other
// direction.
func (m Model) archiveConfirmBody() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "Archive %s\n\n", session.Name)
	if session.Status != "stopped" {
		fmt.Fprintf(&b, "This session is %s, not stopped: confirming kills the live agent\nfirst and archives it in the same action.\n\n", session.Status)
	}
	b.WriteString("Archiving keeps the record and hides the row from the default list.\nIt survives, untouched:\n")
	fmt.Fprintf(&b, "%s\n", m.detailField("Conversation:       ", session.ConversationID))
	fmt.Fprintf(&b, "%s\n", m.detailField("Working directory:  ", session.CWD))
	b.WriteString("\nThe / filter is where an archived row is found again, and U there\nunarchives it. Nothing is written until you confirm.\n")
	b.WriteString("\nEnter archives · Esc cancels\n")
	if m.archiveNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.archiveNote)
	}
	return b.String()
}

// deleteConfirmView renders task 105's second-`d` confirm dialog. It states
// plainly what dd does NOT destroy -- the conversation id and the working
// directory both survive -- and, since task 110, offers "purge" as an
// explicit, non-default choice that names the exact absolute path it will
// delete before doing it (SPEC.md:684-691, requirement 26): purge is
// offered here and nowhere else. An adapter with no declared transcript
// (or one that could not locate it for this session right now) is stated
// plainly to decline, and submitting with purge chosen in that state
// deletes nothing beyond the tombstone itself. Task 009: when the target row
// is archived (ArchivedAt != 0), the body says so in as many words alongside
// §11.4's existing target/survives text -- reaching it through / (the only
// route to an archived row, task 008) must not read like an ordinary
// live-row delete.
func (m Model) deleteConfirmView() string {
	return m.framedDialog(m.deleteConfirmBody())
}

// deleteConfirmBody builds deleteConfirmView's text before framedDialog's
// box-width padTrunc gets anywhere near it (§11.4's dialog box is clamped
// to at most 80 columns, exactly like every other dialog field with a long
// value, e.g. Working directory above). Task 110's own "displays the exact
// absolute path" is about what this function puts in the string -- the
// literal, untruncated m.deletePurgePath, never an inferred or abbreviated
// one -- not about the box renderer's separate, pre-existing display
// truncation, which applies uniformly to every field in every dialog here
// and is exactly why this is its own function: a test can assert against
// this body directly, the same way it would grep the source, without a
// terminal-rendering concern in between. Task 112: a non-empty mark set
// renders the batch body instead (no purge choice -- see
// updateBulkDeleteConfirm's own doc for why).
func (m Model) deleteConfirmBody() string {
	if len(m.marked) > 0 {
		return m.bulkDeleteConfirmBody()
	}
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "Delete %s\n\n", session.Name)
	if session.ArchivedAt != 0 {
		b.WriteString("This session is archived: the target is the archived record itself,\nnot a live one.\n\n")
	}
	b.WriteString("This kills the live pane (if any) and removes the session from the\nlist. It survives, untouched:\n")
	fmt.Fprintf(&b, "%s\n", m.detailField("Conversation:       ", session.ConversationID))
	fmt.Fprintf(&b, "%s\n", m.detailField("Working directory:  ", session.CWD))
	fmt.Fprintf(&b, "\n%s\n", m.detailField("Purge:      ", fmt.Sprintf("%s (left/right cycles: %s)", m.deletePurgeValue, strings.Join(deletePurgeOptions, ", "))))
	switch {
	case m.deletePurgeValue == "purge" && m.deletePurgeOK:
		fmt.Fprintf(&b, "%s\n", m.detailField("Will delete: ", m.deletePurgePath))
	case m.deletePurgeValue == "purge":
		b.WriteString("No transcript could be located for this agent; purge deletes nothing.\n")
	}
	b.WriteString("\nEnter deletes · Esc cancels\n")
	if m.deleteNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.deleteNote)
	}
	return b.String()
}

// bulkDeleteConfirmBody is deleteConfirmBody's task 112 counterpart for a
// non-empty mark set: it names the batch size and every marked session's
// own name rather than one session's conversation id/working directory (a
// batch of N each has its own), and states plainly that purge is not
// offered here.
func (m Model) bulkDeleteConfirmBody() string {
	sessions := m.markedSessions()
	var b strings.Builder
	fmt.Fprintf(&b, "Delete %d marked sessions\n\n", len(sessions))
	b.WriteString("This kills each live pane (if any) and removes each session from the\nlist. Every marked session's own conversation and working directory\nsurvive, untouched. Purge is not offered for a bulk delete.\n\n")
	for _, s := range sessions {
		fmt.Fprintf(&b, "  %s\n", s.Name)
	}
	b.WriteString("\nEnter deletes all · Esc cancels\n")
	if m.deleteNote != "" {
		fmt.Fprintf(&b, "\n%s\n", m.deleteNote)
	}
	return b.String()
}

// detailBody builds the selected session's full detail text (detailView's
// own content, before framedDialogScrollable's box/scroll wrapping),
// including an explicit degradation sentence when the adapter could not
// honour the originally requested permission profile (SPEC §5: "say so in
// the row detail rather than silently lying"). Split out from detailView
// (task 078) so updateDetailView's PgUp/PgDn handling can measure the
// same content dialogMaxScroll would, without re-deriving it.
func (m Model) detailBody() string {
	session := m.sessions[m.selected]
	var b strings.Builder
	fmt.Fprintf(&b, "%s detail\n\n", session.Name)
	fmt.Fprintf(&b, "%s\n", m.detailField("Agent:              ", session.Agent))
	fmt.Fprintf(&b, "%s\n", m.detailField("Working directory:  ", session.CWD))
	if session.CapturedPathAdvisory() {
		fmt.Fprintf(&b, "%s\n", m.detailField("Captured PATH:      ", "advisory only (login_shell overrides PATH; SPEC \u00a76.3)"))
	}
	status := session.Status
	if status == "stopped" {
		status += m.glyph(" · resumable", " - resumable")
	}
	if status == "starting" && session.Agent != "shell" {
		status = "starting" + m.glyph(" · awaiting signal", " - awaiting signal")
	}
	fmt.Fprintf(&b, "%s\n", m.detailField("Status:             ", status))
	if session.StatusReason != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Status reason:      ", session.StatusReason))
	}
	source := session.StatusSource
	if source == "" {
		source = "unknown"
	}
	if quality := statusSourceQuality(session.StatusSource); quality != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict source:     ", fmt.Sprintf("%s (%s)", source, quality)))
	} else {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict source:     ", source))
	}
	if session.StatusAt > 0 {
		fmt.Fprintf(&b, "%s\n", m.detailField("Verdict age:        ", m.relativeAge(session.StatusAt)))
	}
	// A total probe miss (no probeRule matched the sampled pane at all) never
	// changes Status/StatusSource/StatusAt (SPEC §7 is untouched), so it is
	// only surfaced here, distinct from "never sampled": a miss strictly newer
	// than the row's current verdict is the freshest evidence deck has, and is
	// superseded (stops rendering) the instant any later verdict lands.
	if session.LastProbeAt > session.StatusAt {
		fmt.Fprintf(&b, "%s\n", m.detailField("Probe:              ", fmt.Sprintf("sampled, no rule matched (%s)", m.relativeAge(session.LastProbeAt))))
	}
	if _, applicable := m.agentCapabilities(session.Agent); applicable {
		fmt.Fprintf(&b, "%s\n", m.detailField("Permission profile: ", session.PermissionProfile))
		if session.PermissionProfileReason != "" {
			fmt.Fprintf(&b, "%s\n", m.detailField("  degraded: ", session.PermissionProfileReason))
		}
	} else {
		fmt.Fprintf(&b, "%s\n", m.detailField("Permission profile: ", "n/a (shell has no permission profile)"))
	}
	if session.ConversationID != "" {
		fmt.Fprintf(&b, "%s\n", m.detailField("Conversation id:    ", session.ConversationID))
	}
	if session.LastMessage != "" {
		fmt.Fprintf(&b, "\nLast message:\n%s\n", session.LastMessage)
	}
	if session.CrashTail != "" {
		crashTail := m.renderCrashTail(session.CrashTail)
		if session.PaneExitStatus != nil {
			fmt.Fprintf(&b, "\nCrash tail (exit status %d):\n%s\n", *session.PaneExitStatus, crashTail)
		} else {
			fmt.Fprintf(&b, "\nCrash tail:\n%s\n", crashTail)
		}
	}
	b.WriteString("\n" + m.glyph("r renames · i or Esc closes detail", "r renames - i or Esc closes detail") + "\n")
	return b.String()
}

// detailView renders detailBody inside framedDialogScrollable (task 078,
// requirement 39 residual): the WHOLE dialog scrolls uniformly via
// m.detailScroll (PgUp/PgDn, updateDetailView) once a session's fields --
// most of all a long Last message -- push it past the frame budget; no
// field (Last message included) gets its own truncation, line-count
// indicator, or other special-cased bound.
func (m Model) detailView() string {
	return m.framedDialogScrollable(m.detailBody(), m.detailScroll)
}

// renderCrashTail keeps a single crash-tail field readable even when the
// durable 200-line capture contains a full terminal of blank or noisy
// output -- distinct from task 078's whole-dialog PgUp/PgDn scrolling,
// which bounds the OVERALL detail view, not any one field. The stored
// artifact is unchanged; detail shows both ends and says exactly how much
// was omitted.
func (m Model) renderCrashTail(tail string) string {
	lines := strings.Split(strings.Trim(tail, "\n"), "\n")
	const maxLines = 8
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	omitted := len(lines) - maxLines
	visible := append([]string(nil), lines[:maxLines/2]...)
	visible = append(visible, m.colorToken(theme.Dimmed, fmt.Sprintf(m.glyph("… %d lines omitted …", "... %d lines omitted ..."), omitted)))
	visible = append(visible, lines[len(lines)-maxLines/2:]...)
	return strings.Join(visible, "\n")
}

// detailField renders one `label: value` line shared by detailView,
// pinView and profileSwitchView (task 022, SPEC requirement 34): the label
// (including its trailing alignment spaces, exactly as it always rendered)
// in the `hint` token, the value in `text` — mirroring settings.go's own
// settingsRowSegment label/value split rather than inventing a second
// convention. Each half self-resets via colorToken, so plain concatenation
// is safe here: neither dialog composes these lines under a shared
// selection background the way the sidebar/settings rows do.
func (m Model) detailField(label, value string) string {
	return m.colorToken(theme.Hint, label) + m.colorToken(theme.Text, value)
}

// glyph selects the documented ASCII fallback for terminals where optional
// Unicode symbols are unwanted or unsuitable.
func (m Model) glyph(unicode, ascii string) string {
	if m.settings.ASCII {
		return ascii
	}
	return unicode
}

// relativeTime is intentionally based on the configured wall clock. A frozen
// DECK_CLOCK therefore keeps this rendered value stable while Clock.Elapsed
// remains monotonic for measurements and audit durations.
func (m Model) relativeTime(createdAt int64) string {
	age := m.relativeAge(createdAt)
	if age == "just now" {
		return age
	}
	return age + " ago"
}

// relativeAge is wall-clock-derived so verdict freshness remains honest and
// deterministic under DECK_CLOCK. A timestamp in the future is treated as new
// rather than rendering a misleading negative age.
func (m Model) relativeAge(at int64) string {
	now := time.Now()
	if m.settings.Clock != nil {
		now = m.settings.Clock.Now()
	}
	age := now.Sub(time.UnixMilli(at))
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm", int(age/time.Minute))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh", int(age/time.Hour))
	}
	return fmt.Sprintf("%dd", int(age/(24*time.Hour)))
}

// createProfileOptions is the values the create modal's Permission profile
// field cycles through. The Agent field instead cycles m.registry().Kinds(),
// so adding an adapter to the registry never requires an internal/tui edit
// (PRD requirement 1). createProfileOptions is the master ordering used only
// as defaultCreateProfile's own fallback ("safe") below; the actual offered
// set while the modal is open is narrowed per selected agent and per
// allow_yolo by createProfileOptionsFor (SPEC §5, task 017).
var createProfileOptions = []string{"safe", "plan", "edits", "yolo"}

// defaultCreateProfile is the Permission profile field's value when the
// create modal opens fresh (steer 017 item 2 / SPEC §5's yolo_default): it
// is "yolo" when config.toml's yolo_default is true AND yolo is actually
// offered for kind (i.e. allow_yolo is true and the adapter declares yolo
// support) -- yolo_default is inert otherwise, per the config key's own
// documented "inert unless allow_yolo" contract, so a stale yolo_default=
// true left over from a disabled deployment never surprises the operator
// with a modal that opens on an unavailable profile. Every other case falls
// back to createProfileOptionsFor's own narrowed list, options[0] ("safe"
// for every adapter today, since every Caps.Profiles declaration starts
// with "safe" -- internal/agent/claude.go, pi.go), exactly as the modal
// opened before this task existed.
func (m Model) defaultCreateProfile(kind string) string {
	options := m.createProfileOptionsFor(kind, m.settings.AllowYolo)
	if m.settings.YoloDefault && m.settings.AllowYolo && contains(options, "yolo") {
		return "yolo"
	}
	if len(options) == 0 {
		return createProfileOptions[0]
	}
	return options[0]
}

// createProfileOptionsFor returns exactly the permission profiles the
// selected adapter declares (SPEC §5), narrowed further to exclude "yolo"
// when allowYolo is false so the config gate is honoured before yolo is
// even offered. shell has no notion of permission profiles at all
// (agentCapabilities reports !applicable): its field stays a single
// cosmetic "safe" value, per the existing shell-is-inert-to-profiles rule.
func (m Model) createProfileOptionsFor(kind string, allowYolo bool) []string {
	caps, applicable := m.agentCapabilities(kind)
	if !applicable {
		return []string{"safe"}
	}
	options := make([]string, 0, len(caps.Profiles))
	for _, profile := range caps.Profiles {
		if profile == "yolo" && !allowYolo {
			continue
		}
		options = append(options, profile)
	}
	if len(options) == 0 {
		options = []string{"safe"}
	}
	return options
}

const createFieldCount = 8

// createFieldIsText reports whether field accepts free-typed runes, as
// opposed to being a cycled selection (agent, permission profile, login
// shell) that only left/right/space change.
func createFieldIsText(field int) bool {
	switch field {
	case 0, 1, 4, 5, 6:
		return true
	default:
		return false
	}
}

func cycleOption(options []string, current string, delta int) string {
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	index = (index + delta + len(options)) % len(options)
	return options[index]
}

// agentCapabilities returns the declared capabilities for kind, looked up
// in the model's agent registry, and whether a permission profile is even
// applicable to it. shell has no notion of permission profiles at all
// (SPEC §5/§8): its create field is present for consistency but never
// validated, so a shell session is never rejected for an "unsupported"
// profile that simply does not apply to it. An adapter that declares no
// profiles at all (shell) is treated as not applicable, exactly as the old
// hardcoded switch did.
func (m Model) agentCapabilities(kind string) (agent.Caps, bool) {
	adapter, ok := m.registry().Lookup(kind)
	if !ok {
		return agent.Caps{}, false
	}
	caps := adapter.Capabilities()
	return caps, len(caps.Profiles) > 0
}

// transcriptPathFor resolves the declared transcript path (task 109's
// agent.Adapter.TranscriptPaths) for session against the current
// process's own $HOME, exactly as expandCreateCWD/expandCWDDirForScan
// already resolve `~` elsewhere in this package -- never an inferred or
// guessed directory. ok is false whenever the adapter has no transcript
// convention at all (Capabilities().HasTranscript false, e.g. shell) or
// it does but none could be located for this session right now (missing
// HOME, missing project directory, no matching file) -- task 110's
// purge choice declines in every one of those cases rather than deleting
// anything.
func (m Model) transcriptPathFor(session store.Session) (string, bool) {
	adapter, ok := m.registry().Lookup(session.Agent)
	if !ok {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return adapter.TranscriptPaths(agent.TranscriptInput{
		Home:           home,
		CWD:            session.CWD,
		ConversationID: session.ConversationID,
	})
}

// markedSessions resolves task 112's `m` mark set into the actual
// store.Session values, walking m.visualOrder() -- the sidebar's own
// painted order -- rather than m.sessions index order or any range over
// the m.marked map itself (map iteration order is unspecified in Go,
// which would make a "batch" action's own effective order nondeterministic
// run to run). This is the ONE place the mark set is turned into a session
// list; x and dd both call this rather than re-walking m.sessions or
// m.marked themselves.
func (m Model) markedSessions() []store.Session {
	if len(m.marked) == 0 {
		return nil
	}
	var out []store.Session
	for _, idx := range m.visualOrder() {
		session := m.sessions[idx]
		if m.marked[session.ID] {
			out = append(out, session)
		}
	}
	return out
}

// validateCreateFields checks the create modal's free-form fields (cwd,
// launch_args JSON, env pairs, and the selected permission profile) before
// any create call is attempted, returning a specific message for the first
// problem found and "" when the fields are acceptable. Name uniqueness and
// slug collisions are deliberately NOT checked here: those can only be
// known by the store at create time, and their specific messages already
// surface through the shellCreated error path (see createView).
//
// Retaining whatever the user typed is automatic here: this function never
// mutates m, so a non-empty result leaves every field exactly as typed.
func (m Model) validateCreateFields() string {
	if strings.TrimSpace(m.createCWD) == "" {
		return "working directory is required"
	}
	resolvedCWD, err := expandCreateCWD(m.createCWD)
	if err != nil {
		return err.Error()
	}
	info, err := os.Stat(resolvedCWD)
	if err != nil {
		return fmt.Sprintf("working directory %q does not exist", resolvedCWD)
	}
	if !info.IsDir() {
		return fmt.Sprintf("working directory %q is not a directory", resolvedCWD)
	}
	if strings.TrimSpace(m.createLaunchArgs) != "" {
		var args []string
		if err := json.Unmarshal([]byte(m.createLaunchArgs), &args); err != nil {
			return "launch_args must be a JSON array of strings: " + err.Error()
		}
	}
	if strings.TrimSpace(m.createEnv) != "" {
		for _, entry := range strings.Split(m.createEnv, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			key, _, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(key) == "" {
				return fmt.Sprintf("env entry %q must be key=value", entry)
			}
		}
	}
	if caps, applicable := m.agentCapabilities(m.createAgent); applicable {
		if !caps.SupportsProfile(m.createProfile) {
			_, _, reason := caps.ResolveProfile(m.createAgent, m.createProfile)
			return "unsupported permission profile: " + reason
		}
	}
	if m.createProfile == "yolo" {
		if !m.settings.AllowYolo {
			return "yolo permission profile is not available: enable allow_yolo in config.toml"
		}
	}
	return ""
}

// cycleCreateCWDRecent implements the cwd field's second declared §11.7
// per-field key set (task 009): up/down cycle recents shell-history style.
// delta is +1 for "up" (older) and -1 for "down" (newer, eventually exiting
// the cycle back to whatever the field held before it started).
//
// The first "up" snapshots both the recent_cwds list itself (so it cannot
// change under a live cycle) and the field's pre-cycle state (value plus
// its prefilled/last-used flags) so "down" can restore it exactly once the
// cycle runs back past the most recent entry -- an untouched §11.7 prefill,
// or whatever the user had already typed, comes back exactly as it was
// rather than as a blank field or a stale recents[0]. A "down" with no
// cycle in progress, or a store with no history at all, is a no-op: there
// is nothing to cycle to.
func (m *Model) cycleCreateCWDRecent(delta int) {
	if m.createCWDRecentIndex < 0 {
		if delta <= 0 || m.store == nil {
			return
		}
		recents, err := m.store.RecentCwds(context.Background())
		if err != nil || len(recents) == 0 {
			return
		}
		m.createCWDRecents = recents
		m.createCWDPreCycleValue = m.createCWD
		m.createCWDPreCyclePrefilled = m.createCWDPrefilled
		m.createCWDPreCycleLastUsed = m.createCWDLastUsed
		m.createCWDRecentIndex = 0
		m.createCWD = recents[0].Path
		m.createCWDPrefilled = false
		m.createCWDLastUsed = false
		return
	}
	next := m.createCWDRecentIndex + delta
	if next < 0 {
		// Ran "down" back past the most recent entry: exit the cycle and
		// restore exactly what was there before the first "up".
		m.createCWD = m.createCWDPreCycleValue
		m.createCWDPrefilled = m.createCWDPreCyclePrefilled
		m.createCWDLastUsed = m.createCWDPreCycleLastUsed
		m.createCWDRecentIndex = -1
		return
	}
	if next >= len(m.createCWDRecents) {
		// "up" at the oldest entry stays there rather than wrapping --
		// shell history has an end, and wrapping back to recents[0] would
		// be indistinguishable on screen from having never moved.
		next = len(m.createCWDRecents) - 1
	}
	m.createCWDRecentIndex = next
	m.createCWD = m.createCWDRecents[next].Path
}

// prefillCreateCWD returns the value and "is an untouched prefill" flag the
// create modal's cwd field opens with (SPEC §11.7): the most recent
// recent_cwds entry (rendered with the "(last used)" label in
// createFieldRows) when the store has any history, otherwise the directory
// deck itself was started in. A store error or no history at all falls back
// to startCWD rather than leaving the field blank, matching every other
// best-effort recent_cwds read in this package (promoteRecentCwd's own
// swallowed error: §11.7 history is never load-bearing).
func (m Model) prefillCreateCWD() (value string, isRecent bool) {
	if m.store != nil {
		if recents, err := m.store.RecentCwds(context.Background()); err == nil && len(recents) > 0 {
			return recents[0].Path, true
		}
	}
	return m.startCWD, false
}

// expandCreateCWD expands a leading `~` or `~/...` in raw to the current
// user's home directory and returns the resolved absolute path, per SPEC
// §11.7's tilde-expansion rule (the minimum slice of it Phase 2b-2 owns:
// no recent_cwds, prefill, history cycling, ghost completion or tab —
// those stay in Phase 3). A bare `~otheruser` form is rejected with a
// stated reason rather than half-expanded, since resolving another user's
// home directory is out of scope. Inputs with no leading `~` are returned
// unchanged, preserving every existing relative/absolute-path behaviour.
func expandCreateCWD(raw string) (string, error) {
	if !strings.HasPrefix(raw, "~") {
		return raw, nil
	}
	if raw != "~" && !strings.HasPrefix(raw, "~/") {
		return "", fmt.Errorf("cannot expand %q: only your own home directory (~ or ~/...) is supported, not another user's", raw)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %w", raw, err)
	}
	if raw == "~" {
		return filepath.Abs(home)
	}
	return filepath.Abs(filepath.Join(home, strings.TrimPrefix(raw, "~/")))
}

// parseCreateLaunchArgs parses the create modal's launch_args field into the
// JSON array of strings service.AgentCreateInput expects. An empty field is
// simply no extra args, matching validateCreateFields' own "" is fine rule.
func parseCreateLaunchArgs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("launch_args must be a JSON array of strings: %w", err)
	}
	return args, nil
}

// parseCreateEnv parses the create modal's comma-separated key=value env
// field into the map service.AgentCreateInput expects, mirroring the same
// splitting rule validateCreateFields already checked.
func parseCreateEnv(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	env := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("env entry %q must be key=value", entry)
		}
		env[strings.TrimSpace(key)] = value
	}
	return env, nil
}

func (m Model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The candidate list (task 012) only ever exists while field 1 (cwd)
	// is focused; leaving it any other way -- shift+tab away, cycling the
	// Agent/profile fields, anything applyDialogContract's own field
	// navigation does -- must not leave it open and stale for the next
	// time the user tabs back onto this field.
	if m.createField != 1 {
		m.closeCreateCWDCandidates()
	}
	if m.createField == 1 {
		switch msg.String() {
		case "tab":
			// §11.7's bash-completion-contract key (task 012): "tab completes
			// to the longest common prefix when that advances the text, and
			// otherwise lists the candidates for selection". This replaces
			// the cwd field's normal "tab moves to the next field" ONLY
			// while there is something to complete or list; whenever there
			// is nothing to do at all -- no directory matches the segment,
			// or the segment already names the one candidate there is in
			// full -- tabCompleteCreateCWD reports false and tab falls
			// through to applyDialogContract below exactly as it always
			// has, so existing callers that tab straight through an
			// untouched, already-real cwd prefill onto the next field stay
			// unaffected.
			if m.tabCompleteCreateCWD() {
				return m, nil
			}
		case "esc":
			// esc closes an open candidate list without changing the field
			// or the value, one step short of applyDialogContract's own esc
			// (cancel the whole modal) -- consumed here first so a user
			// backing out of the list is not also thrown out of the create
			// modal in the same keystroke.
			if len(m.createCWDCandidates) > 0 {
				m.closeCreateCWDCandidates()
				return m, nil
			}
		case "enter":
			// enter selects the highlighted candidate into the field rather
			// than submitting the whole modal, one step short of
			// applyDialogContract's own enter -- exactly like esc above.
			if len(m.createCWDCandidates) > 0 {
				m.acceptCWDCandidate(m.createCWDCandidates[m.createCWDCandidateIndex])
				return m, nil
			}
		case "up":
			// While the list is open, up/down move the highlighted entry
			// rather than cycling recent_cwds (the existing "up"/"down" case
			// below): the two per-field key sets are mutually exclusive,
			// same as the ghost/ambiguous-count/recent labels already are.
			if len(m.createCWDCandidates) > 0 {
				m.createCWDCandidateIndex = (m.createCWDCandidateIndex - 1 + len(m.createCWDCandidates)) % len(m.createCWDCandidates)
				return m, nil
			}
		case "down":
			if len(m.createCWDCandidates) > 0 {
				m.createCWDCandidateIndex = (m.createCWDCandidateIndex + 1) % len(m.createCWDCandidates)
				return m, nil
			}
		}
	}
	if cmd, handled := applyDialogContract(msg, dialogContract{
		Fields: dialogFields{
			Count:          createFieldCount,
			Index:          &m.createField,
			Cycle:          m.cycleCreateField,
			SpaceTypesText: func() bool { return createFieldIsText(m.createField) },
		},
		Cancel: func() { m.creating, m.createError = false, "" },
		Submit: m.submitCreate,
	}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "up":
		// §11.7's second declared per-field key set on the cwd field (task
		// 009): shell-history-style cycling through recent_cwds, older with
		// each press. Only field 1 (cwd) binds it -- every other field's
		// up/down stays a no-op here, matching "up"/"down" doing nothing at
		// all in this dialog before this task.
		if m.createField == 1 {
			m.cycleCreateCWDRecent(1)
			return m, nil
		}
	case "down":
		if m.createField == 1 {
			m.cycleCreateCWDRecent(-1)
			return m, nil
		}
	case "end":
		// The ghost completion's other declared acceptance key (task 010),
		// alongside right -- see cycleCreateField's case 1. Only field 1
		// (cwd) ever has a ghost to accept; every other field leaves "end"
		// unhandled here, falling through with no effect, exactly as it did
		// before this task.
		if m.createField == 1 {
			m.acceptCWDGhost()
			return m, nil
		}
	case "backspace", "ctrl+h":
		m.backspaceCreateField()
		return m, nil
	case "pgup":
		// Task 016: the create modal moved onto framedDialogScrollable
		// (its field set + candidate list + footer/error lines can wrap
		// well past the frame budget), so PgUp/PgDn now scroll it exactly
		// like the three no-fields overlays already do -- measured off
		// m.createBody(), the plain body, never the coloured one, so a
		// theme change can never move where a page boundary falls.
		m.createScroll = m.dialogScrollByPage(m.createScroll, m.createBody(), -1)
		return m, nil
	case "pgdown":
		m.createScroll = m.dialogScrollByPage(m.createScroll, m.createBody(), 1)
		return m, nil
	}
	if runes := msg.Runes; len(runes) > 0 && createFieldIsText(m.createField) {
		switch m.createField {
		case 0:
			m.createName += string(runes)
		case 1:
			// The prefilled recent/startup cwd is replaced wholesale by the
			// first keystroke rather than appended to (SPEC §11.7): once
			// the user has typed anything, the field holds only what they
			// typed and no longer carries the "last used" label.
			if m.createCWDPrefilled {
				m.createCWD, m.createCWDPrefilled, m.createCWDLastUsed = "", false, false
			}
			// Typing ends any up/down cycle in progress (task 009): the
			// field now holds what the user typed, not a recent_cwds
			// snapshot, so "recent N/M" must stop being shown and a later
			// "down" must not resurrect the pre-cycle value out from under
			// what was just typed.
			m.createCWDRecentIndex = -1
			// Typing also ends an open tab-completion candidate list (task
			// 012): the segment it was built for no longer exists once the
			// user keeps typing, so the stale list must not linger.
			m.closeCreateCWDCandidates()
			m.createCWD += string(runes)
		case 4:
			m.createLaunchArgs += string(runes)
		case 5:
			m.createEnv += string(runes)
		case 6:
			m.createPreLaunch += string(runes)
		}
	}
	return m, nil
}

// submitCreate implements the create modal's enter (SPEC §11.4 submit): it
// validates, resolves the cwd, dispatches the right create call for the
// chosen agent, and reports every rejection in-dialog via m.createError,
// exactly as the dialog's own hand-written "enter" case used to before the
// shared §11.4 contract (dialogContract.Submit) took over dispatching the
// key itself.
func (m *Model) submitCreate() tea.Cmd {
	if msg := m.validateCreateFields(); msg != "" {
		m.createError = msg
		return nil
	}
	// validateCreateFields already proved this succeeds; the resolved,
	// absolute path (never the typed tilde) is what gets stored.
	resolvedCWD, err := expandCreateCWD(m.createCWD)
	if err != nil {
		m.createError = err.Error()
		return nil
	}
	name := m.resolveCreateName(resolvedCWD)
	if m.createAgent != "shell" {
		if m.createAgentSession == nil {
			m.createError = "creating " + m.createAgent + " sessions is not available yet"
			return nil
		}
		launchArgs, err := parseCreateLaunchArgs(m.createLaunchArgs)
		if err != nil {
			m.createError = err.Error()
			return nil
		}
		env, err := parseCreateEnv(m.createEnv)
		if err != nil {
			m.createError = err.Error()
			return nil
		}
		input := service.AgentCreateInput{
			Name: name, CWD: resolvedCWD, Agent: m.createAgent,
			PermissionProfile: m.createProfile, LaunchArgs: launchArgs, Env: env,
			PreLaunch: m.createPreLaunch, LoginShell: m.createLoginShell,
		}
		createAgentSession := m.createAgentSession
		return func() tea.Msg {
			session, err := createAgentSession(context.Background(), input)
			return shellCreated{session: session, err: err}
		}
	}
	if m.create == nil {
		m.createError = "shell creation is unavailable"
		return nil
	}
	cwd, create := resolvedCWD, m.create
	return func() tea.Msg {
		session, err := create(context.Background(), service.ShellCreateInput{Name: name, CWD: cwd})
		return shellCreated{session: session, err: err}
	}
}

// resolveCreateName implements SPEC.md:174-176's blank-name default (PRD
// requirement 6): a name the user actually typed (after trimming) is
// always used verbatim, exactly as before this task -- the default is only
// *derived*, never special, so it goes through the identical uniqueness
// path as any other name (the store's own UNIQUE constraint surfaces a
// collision on an explicitly typed name as "already exists", per
// validateCreateFields' doc comment). Only a blank field synthesises
// `<workspace>-<MMDD-HHMM>` from m.settings.Clock (so a frozen DECK_CLOCK
// makes it deterministic, per SPEC.md:176) and DefaultWorkspace's
// basename-of-cwd rule (store.DefaultWorkspace, matching the sidebar's own
// grouping key), then appends the smallest free `-2`, `-3`, ... suffix by
// checking the store's current names -- never failing the create outright
// on a collision, per SPEC.md:175 ("collisions append a suffix rather than
// failing the create").
func (m *Model) resolveCreateName(resolvedCWD string) string {
	if trimmed := strings.TrimSpace(m.createName); trimmed != "" {
		return trimmed
	}
	now := time.Now()
	if m.settings.Clock != nil {
		now = m.settings.Clock.Now()
	}
	base := store.DefaultWorkspace(resolvedCWD) + "-" + now.Format("0102-1504")
	taken := make(map[string]bool)
	if m.store != nil {
		if sessions, err := m.store.ListSessions(context.Background()); err == nil {
			for _, session := range sessions {
				taken[session.Name] = true
			}
		}
	}
	if !taken[base] {
		return base
	}
	for suffix := 2; suffix < 100000; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !taken[candidate] {
			return candidate
		}
	}
	// Unreachable outside a pathological test setup deliberately creating
	// 100000 collisions; fall back to the bare base rather than panicking,
	// leaving the store's own UNIQUE constraint to report the failure.
	return base
}

// cycleCreateField advances a selection-type field's value by delta; it is a
// no-op on the free-text fields.
func (m *Model) cycleCreateField(delta int) {
	switch m.createField {
	case 2:
		m.createAgent = cycleOption(m.registry().Kinds(), m.createAgent, delta)
		// createProfileTouched (steer 017 item 2 / yolo_default): as long as
		// the user has not yet cycled the Permission profile field itself
		// (case 3 below), the value showing there is still "whatever the
		// default would pick for the currently selected agent", not a
		// deliberate choice -- so changing agent here must re-run
		// defaultCreateProfile for the NEW agent (SPEC §5/§6.5: yolo_default
		// should be visible once an agent that offers yolo is selected, not
		// only when it happens to be the very first agent defaultCreateAgent
		// picks, which is always "shell" and never offers yolo at all). Once
		// touched, the user's own explicit selection is never overridden by
		// an agent change; it only falls back to options[0] if the chosen
		// value becomes unavailable for the new agent, exactly as before.
		if !m.createProfileTouched {
			m.createProfile = m.defaultCreateProfile(m.createAgent)
		} else if options := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo); !contains(options, m.createProfile) {
			m.createProfile = options[0]
		}
	case 3:
		options := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
		m.createProfile = cycleOption(options, m.createProfile, delta)
		m.createProfileTouched = true
	case 1:
		// Right (never left/space -- see updateCreate's SpaceTypesText gate
		// and cycleCreateField's own delta<=0 no-op) accepts the ghost
		// completion shown inline, if any (task 010).
		if delta > 0 {
			m.acceptCWDGhost()
		}
	case 7:
		m.createLoginShell = !m.createLoginShell
	}
}

// contains reports whether options includes value.
func contains(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func (m *Model) backspaceCreateField() {
	switch m.createField {
	case 0:
		if len(m.createName) > 0 {
			m.createName = m.createName[:len(m.createName)-1]
		}
	case 1:
		if m.createCWDPrefilled {
			// Backspace is also an edit: clear the untouched prefill
			// wholesale rather than trimming one rune off the end of it
			// (SPEC §11.7's "typing replaces it wholesale" applies to any
			// edit, not only appended runes).
			m.createCWD, m.createCWDPrefilled, m.createCWDLastUsed = "", false, false
			m.createCWDRecentIndex = -1
			m.closeCreateCWDCandidates()
			return
		}
		// Backspace also ends an up/down cycle in progress (task 009), same
		// reasoning as the rune-append path above: the field is being edited
		// now, so it must stop being a live view of createCWDRecents.
		m.createCWDRecentIndex = -1
		// Backspace also ends an open tab-completion candidate list (task
		// 012), same reasoning: the segment it was built for is changing.
		m.closeCreateCWDCandidates()
		if len(m.createCWD) > 0 {
			m.createCWD = m.createCWD[:len(m.createCWD)-1]
		}
	case 4:
		if len(m.createLaunchArgs) > 0 {
			m.createLaunchArgs = m.createLaunchArgs[:len(m.createLaunchArgs)-1]
		}
	case 5:
		if len(m.createEnv) > 0 {
			m.createEnv = m.createEnv[:len(m.createEnv)-1]
		}
	case 6:
		if len(m.createPreLaunch) > 0 {
			m.createPreLaunch = m.createPreLaunch[:len(m.createPreLaunch)-1]
		}
	}
}

// createCWDHelp prefixes the cwd field's usual one-line explanation with
// one of: "N matches — tab to list" (task 011, requirement 15) while raw's
// segment has several directory candidates and no ghost is shown at all
// (checked first, since it only ever applies while actively typing, which
// already means the field holds neither a prefill nor an in-progress
// recent_cwds cycle); "(last used)" (task 008) while the field still holds
// its untouched most-recent recent_cwds prefill; or "recent N/M" (task 009,
// SPEC §11.7's declared up/down per-field key set) while an up/down cycle
// through recent_cwds is in progress -- never more than one of the three
// at once, since starting a cycle or typing a fresh segment clears the
// others' flags. Each label is prefixed onto the field's short,
// constant-length help text rather than appended to the value itself: the
// value line's own width varies with the resolved path length, and
// framedDialog's word-wrap (task 030) can split a value-suffixed label
// across two rendered lines at an arbitrary point depending on that
// length, whereas a label prefixed to the help text never does. The plain
// directory-deck-started-in fallback (no history, not cycling, not
// ambiguous) carries no label at all.
// createCWDGhostSuffix is the cwd field's ghost completion (task 010) as
// the PLAIN trailing suffix of createCWDDisplayValue's value: non-empty
// only when a unique directory match exists AND the cwd field is actually
// focused -- "cursor at end of field" only means something while this
// field is the one being edited, so a ghost is never shown, and right/end
// never has anything to accept, on any OTHER field's rendering of this
// same row.
//
// It exists so the ghost's §11.6 `dimmed` colouring can be applied where
// the frame is drawn (styledCreateBody, which asks this for the cwd row
// and colours exactly those trailing bytes) instead of inside the value
// itself: colouring it here baked SGR bytes into the string
// createFieldRows returns, which createBody then interpolated into the
// very body wrapDialogLines/dialogMaxScroll measure -- so an escape
// sequence counted as display width and moved where a page boundary fell,
// exactly what SPEC.md:1355 forbids.
func (m Model) createCWDGhostSuffix() string {
	if m.createField != 1 {
		return ""
	}
	ghost, ok := createCWDGhostCompletion(m.createCWD)
	if !ok {
		return ""
	}
	return ghost
}

// createCWDDisplayValue is the cwd field's rendered value: m.createCWD
// plus its ghost completion, both plain -- see createCWDGhostSuffix for
// where the ghost's dimmed token is applied instead.
func (m Model) createCWDDisplayValue() string {
	return m.createCWD + m.createCWDGhostSuffix()
}

func (m Model) createCWDHelp() string {
	help := "the session's cwd; must exist and be a directory; \u2191/\u2193 cycles recent history; right/end completes a shown directory match"
	// Ambiguous-match counting (task 011, requirement 15) only applies
	// while this field is actually focused and being typed into, exactly
	// like createCWDDisplayValue's ghost: "the cursor is at end of field"
	// only means something during an edit of THIS field, so another
	// field's rendering of this same row never shows a stale count left
	// over from before the user tabbed away.
	if m.createField == 1 {
		if count, ok := createCWDAmbiguousMatchCount(m.createCWD); ok {
			return fmt.Sprintf("%d matches \u2014 tab to list ", count) + help
		}
	}
	if m.createCWDRecentIndex >= 0 && len(m.createCWDRecents) > 0 {
		return fmt.Sprintf("recent %d/%d ", m.createCWDRecentIndex+1, len(m.createCWDRecents)) + help
	}
	if m.createCWDLastUsed {
		return "(last used) " + help
	}
	return help
}

// createNameReuseWarning reports the specific, in-dialog reason to show on
// the Name field before submit (PRD R77 / SPEC §11.4's "the dialog says so
// before it happens"): when the typed name (or its §3.2 slug) is currently
// held ONLY by a tombstoned row, submitting reuses it, and CreateSession's
// own tx-scoped reap (task 003, reapTombstonedHolderTx) silently removes
// that row -- events, files and all -- forfeiting its 60s undo as a side
// effect the user would otherwise only discover after the fact. "" means
// nothing to warn about: a blank field (resolveCreateName's synthesized
// default was never typed by the user, so it is not checked here), a name
// that is free, or one held by a LIVE or ARCHIVED row (those refuse the
// create outright instead -- store's own UNIQUE constraint / R78 -- so
// there is no undo at stake to warn about). A nil store (e.g. this Model
// used in a unit test with no backing store) is treated the same as
// "nothing to warn about" rather than a panic.
func (m Model) createNameReuseWarning() string {
	name := strings.TrimSpace(m.createName)
	if name == "" || m.store == nil {
		return ""
	}
	holders, err := m.store.TombstonedNameHolders(context.Background(), name)
	if err != nil || len(holders) == 0 {
		return ""
	}
	return "this name belongs to a deleted session; reuse discards its undo"
}

// createFieldRows describes the create modal's field set: label, current
// value renderer and a one-line explanation of what the field does. Keeping
// this as one table (rather than scattered Fprintf calls) is what lets a
// keyboard-only PTY test assert every label and its explanation are
// rendered together (task 015).
func (m Model) createFieldRows() []struct{ label, value, help string } {
	loginShell := "off"
	if m.createLoginShell {
		loginShell = "on"
	}
	profileOptions := m.createProfileOptionsFor(m.createAgent, m.settings.AllowYolo)
	profileValue := m.createProfile + " (left/right cycles: " + strings.Join(profileOptions, ", ") + ")"
	profileHelp := "how much the agent may do without asking; an unsupported profile degrades to safe"
	if !m.settings.AllowYolo {
		profileHelp += "; yolo is not offered because allow_yolo is not enabled in config.toml"
	}
	return []struct{ label, value, help string }{
		{"Name", m.createName, "the display name; also the source of the session's tmux slug"},
		{"Working directory", m.createCWDDisplayValue(), m.createCWDHelp()},
		{"Agent", m.createAgent + " (left/right cycles: " + strings.Join(m.registry().Kinds(), ", ") + ")", "which coding agent adapter launches this session"},
		{"Permission profile", profileValue, profileHelp},
		{"Launch args (JSON array)", m.createLaunchArgs, "extra arguments appended verbatim after the adapter's own argv"},
		{"Env (key=value, comma-separated)", m.createEnv, "session-level environment variables, highest priority in PATH resolution"},
		{"Pre-launch command", m.createPreLaunch, "a command run in the pane before the agent starts, e.g. to load secrets"},
		{"Login shell", loginShell + " (space toggles)", "makes captured_path advisory only (not applied): runs via $SHELL -lc instead of the agent argv, so the login shell sets PATH"},
	}
}

// createFieldMarker is the create modal's own ">"/"  " focus marker
// (task 016 split this out of createBody so styledCreateBody can build
// the identical plain label line it colours, without duplicating the
// marker literal in two places).
func (m Model) createFieldMarker(field int) string {
	if m.createField == field {
		return "> "
	}
	return "  "
}

// createBody builds the create modal's PLAIN, unstyled content -- byte
// for byte what createView rendered directly before task 016 added a
// themed rendering pass. It stays the one text scroll math measures
// (dialogScrollByPage in updateCreate's new pgup/pgdown cases, mirroring
// helpText/eventLogBody's own plain body passed to their PgUp/PgDn
// handlers) so a colour token this task adds can never move where a page
// boundary falls. styledCreateBody (below) is createView's only other
// caller, and never re-derives this structure independently -- both walk
// createFieldRows/createNameReuseWarning/createCWDCandidates in the same
// order, so the two can never drift into a different physical line count.
func (m Model) createBody() string {
	var b strings.Builder
	title := "Create session"
	if m.createAgent == "shell" {
		title = "Create shell session"
	}
	b.WriteString(title + "\n")
	for field, row := range m.createFieldRows() {
		fmt.Fprintf(&b, "%s%s: %s\n    %s\n", m.createFieldMarker(field), row.label, row.value, row.help)
		if field == 0 {
			// The reuse warning (PRD R77 / SPEC §11.4) gets its own
			// dedicated line rather than being folded into the Name
			// row's own one-line explanation above: that explanation is
			// already long enough on its own that appending this to it
			// risks framedDialog's word-wrap splitting the warning's own
			// sentence across two physical lines at a point that varies
			// with dialogWidth, whereas a short, dedicated line stays
			// intact.
			if warning := m.createNameReuseWarning(); warning != "" {
				fmt.Fprintf(&b, "    %s\n", warning)
			}
		}
	}
	if len(m.createCWDCandidates) > 0 {
		// task 012's tab-completion listing branch: rendered directly under
		// the field set (not buried in the one-line help text, unlike
		// "N matches" -- an actually navigable list needs its own lines) so
		// a scenario can assert both the candidate set and which one is
		// highlighted.
		b.WriteString("  candidates (up/down selects, enter or tab accepts, esc closes):\n")
		for i, name := range m.createCWDCandidates {
			marker := "    "
			if i == m.createCWDCandidateIndex {
				marker = "  > "
			}
			fmt.Fprintf(&b, "%s%s/\n", marker, name)
		}
	}
	b.WriteString("Tab/Shift+Tab field · Left/Right/Space cycles · Enter submits · Esc cancels\n")
	if m.createError != "" {
		if strings.Contains(m.createError, "collides with existing slug") {
			b.WriteString("\nCannot create session: name collides with existing slug.\n")
		} else {
			fmt.Fprintf(&b, "\nCannot create session: %s\n", m.createError)
		}
	}
	return b.String()
}

// createFooterKeyTokens is the create modal's footer legend vocabulary
// (task 016), mirroring help_style.go's helpKeycapTokens one section
// down: the leading token of each " · "-separated entry in createBody's
// own footer line ("Tab/Shift+Tab field · Left/Right/Space cycles ·
// Enter submits · Esc cancels"), used only to decide which already-
// wrapped word gets theme.Key instead of theme.Hint -- see
// styledCreateBody's colorFooterLine for why this runs word-by-word on
// the PLAIN, already-wrapped line rather than colouring before wrapping.
var createFooterKeyTokens = map[string]bool{
	"Tab/Shift+Tab":    true,
	"Left/Right/Space": true,
	"Enter":            true,
	"Esc":              true,
}

// renderCreateRowSegments composes segs onto one already-wrapped physical
// line (task 016): a plain colorToken call per segment for an unfocused
// row (each self-resetting, exactly like every other themed line
// styledCreateBody builds), or -- for the one row m.createField currently
// names -- SPEC.md:1355's "focused field carrying the same selection
// treatment a selected list row does": composed through
// settingsRenderRowOpen (settings.go), which opens each segment's own
// foreground colour but never closes it, wrapped in exactly one
// bgColorToken(theme.Selection, ...) reset at the very end. A per-segment
// colorToken reset would double as clearing that outer background the
// instant the first segment's own text ended (foregroundSGR's own doc
// comment on theme_color.go), which is why a focused row cannot reuse the
// unfocused branch's plain per-segment colorToken calls.
func (m Model) renderCreateRowSegments(focused bool, segs []settingsRowSegment) string {
	if focused {
		return m.bgColorToken(theme.Selection, m.settingsRenderRowOpen(segs))
	}
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(m.colorToken(s.Tok, s.Text))
	}
	return b.String()
}

// styledCreateBody re-derives createBody's exact structure -- same field
// loop, same reuse-warning/candidate/footer/error branches, in the same
// order -- but colours each finished PHYSICAL line rather than the
// logical one: every call below wraps a plain (uncoloured) string via
// wrap (m.wrapDialogLines) FIRST, and only ever colours the strings that
// call already returned. A colour token this task adds can therefore
// never straddle a word-wrap boundary wrapDialogLines hasn't drawn yet --
// the failure mode SPEC.md:1355's "colour is applied where the frame is
// drawn and never baked into the strings the model holds" warns against,
// and the reason createBody above stays the one plain body every scroll
// measurement uses. Token mapping is SPEC.md:1355 verbatim: the title in
// `title`, a field's label in `hint` and its value in `text`, its help in
// `dimmed`, the footer legend's keys in `key`, a validation message in
// `error`, and the focused field in `selection` (renderCreateRowSegments).
// The one-line reuse warning (R77) is not itself a validation message --
// it renders before any submit is attempted -- so it takes `dimmed`,
// matching every other inline explanatory caveat in this dialog.
func (m Model) styledCreateBody() string {
	wrap := m.wrapDialogLines
	var out []string

	colorWhole := func(tok theme.Token, line string) {
		for _, l := range wrap(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	// colorLabelValue colours one field row's already-wrapped label/value
	// line. ghost is the plain trailing suffix of plainLine that is a ghost
	// completion rather than typed text (createCWDGhostSuffix; "" for every
	// row but the focused cwd one): those bytes take `dimmed` while the
	// typed part keeps `text`, which is how the ghost stays visibly
	// provisional now that the suffix itself reaches here uncoloured.
	colorLabelValue := func(labelPrefix, plainLine, ghost string, focused bool) {
		lines := wrap(plainLine)
		// ghostSpan[i] is how many TRAILING bytes of lines[i] belong to the
		// ghost. Walking the physical lines backwards, consuming the ghost
		// from its own end, attributes it correctly even when wrapping
		// splits it across two lines. A line whose tail does not match the
		// ghost's remaining tail byte for byte (wrapping dropped a space at
		// the break) stops the walk instead of guessing: the affected line
		// then renders wholly as `text`, never as a mis-aligned colour span.
		ghostSpan := make([]int, len(lines))
		for i, rem := len(lines)-1, ghost; i >= 0 && rem != ""; i-- {
			n := len(rem)
			if n > len(lines[i]) {
				n = len(lines[i])
			}
			if !strings.HasSuffix(lines[i], rem[len(rem)-n:]) {
				break
			}
			ghostSpan[i] = n
			rem = rem[:len(rem)-n]
		}
		for i, l := range lines {
			var segs []settingsRowSegment
			value := l
			if strings.HasPrefix(l, labelPrefix) {
				segs = append(segs, settingsRowSegment{Text: labelPrefix, Tok: theme.Hint})
				value = strings.TrimPrefix(l, labelPrefix)
			}
			// Otherwise this is a physical continuation line (the value
			// overflowed onto its own line): no label prefix left to split
			// out, so the whole line is the value's own overflow.
			n := ghostSpan[i]
			if n > len(value) {
				n = len(value)
			}
			if typed := value[:len(value)-n]; typed != "" {
				segs = append(segs, settingsRowSegment{Text: typed, Tok: theme.Text})
			}
			if n > 0 {
				segs = append(segs, settingsRowSegment{Text: value[len(value)-n:], Tok: theme.Dimmed})
			}
			if len(segs) == 0 {
				segs = []settingsRowSegment{{Text: l, Tok: theme.Text}}
			}
			out = append(out, m.renderCreateRowSegments(focused, segs))
		}
	}
	// colorFooterLine colours createBody's already-wrapped footer legend
	// line word by word (never before wrap: see this function's own doc
	// comment) -- safe because every createFooterKeyTokens entry and its
	// one-word meaning ("Tab/Shift+Tab field", "Enter submits", ...) is
	// exactly two whitespace-delimited words, so no colour span this adds
	// ever covers more than one word, and rejoining strings.Fields' output
	// with single spaces reproduces createBody's own single-space-and-
	// " · "-separated layout exactly.
	colorFooterLine := func(line string) {
		for _, l := range wrap(line) {
			fields := strings.Fields(l)
			for i, f := range fields {
				if createFooterKeyTokens[f] {
					fields[i] = m.colorToken(theme.Key, f)
				} else {
					fields[i] = m.colorToken(theme.Hint, f)
				}
			}
			out = append(out, strings.Join(fields, " "))
		}
	}

	title := "Create session"
	if m.createAgent == "shell" {
		title = "Create shell session"
	}
	colorWhole(theme.Title, title)

	for field, row := range m.createFieldRows() {
		marker := m.createFieldMarker(field)
		labelPrefix := fmt.Sprintf("%s%s: ", marker, row.label)
		// Field 1 is the cwd row (createFieldRows' own order): the one row
		// whose value can end in a ghost completion rather than typed text.
		ghost := ""
		if field == 1 {
			ghost = m.createCWDGhostSuffix()
		}
		colorLabelValue(labelPrefix, labelPrefix+row.value, ghost, field == m.createField)
		colorWhole(theme.Dimmed, "    "+row.help)
		if field == 0 {
			if warning := m.createNameReuseWarning(); warning != "" {
				colorWhole(theme.Dimmed, "    "+warning)
			}
		}
	}
	if len(m.createCWDCandidates) > 0 {
		colorWhole(theme.Hint, "  candidates (up/down selects, enter or tab accepts, esc closes):")
		for i, name := range m.createCWDCandidates {
			marker := "    "
			if i == m.createCWDCandidateIndex {
				marker = "  > "
			}
			colorWhole(theme.Text, marker+name+"/")
		}
	}
	colorFooterLine("Tab/Shift+Tab field · Left/Right/Space cycles · Enter submits · Esc cancels")
	if m.createError != "" {
		out = append(out, "")
		if strings.Contains(m.createError, "collides with existing slug") {
			colorWhole(theme.Error, "Cannot create session: name collides with existing slug.")
		} else {
			colorWhole(theme.Error, "Cannot create session: "+m.createError)
		}
	}
	return strings.Join(out, "\n")
}

// createView renders the create modal through framedDialogScrollable
// (task 016) rather than the unbounded framedDialog: its own field set
// alone already reaches 29 lines untouched (framedDialogScrollable's doc
// comment), well past an 80x24 frame's budget, and this task's theming
// pass adds no new content but must not regress that -- PgUp/PgDn
// (updateCreate) scroll m.createScroll so the submit line stays reachable
// instead of ever silently truncated off the bottom.
//
// A rejection is the one case that must not wait for a manual PgDn: every
// existing validation/collision test in this package (predating this
// task, e.g. create_validation_test.go) submits, then reads createView's
// OWN string for the rejection reason with no scroll keystroke in
// between -- exactly the SPEC.md requirement-15 contract ("names the
// specific problem in-dialog"). So whenever an error is set and the user
// has not already scrolled away from the top (m.createScroll == 0, its
// value the instant `n`/submit opened or last redrew this dialog), the
// view scrolls itself to the bottom -- where every error/collision line
// this file ever appends always lands, since createBody appends them
// last -- rather than requiring a page-down to see why the submit
// failed. A user who has already scrolled elsewhere keeps their own
// position; this only ever overrides the untouched default.
func (m Model) createView() string {
	scroll := m.createScroll
	if m.createError != "" && scroll == 0 {
		scroll = m.dialogMaxScroll(m.createBody())
	}
	return m.framedDialogScrollable(m.styledCreateBody(), scroll)
}

// helpView renders the `?` help overlay (SPEC requirement 16: bordered like
// every other panel/dialog/overlay). It is a Model method rather than a
// free function so it can reuse framedDialog's box-drawing without
// duplicating boxGlyphs/ASCII-fallback logic here. Task 078 (requirement
// 39 residual): the overlay is height-bounded via framedDialogScrollable
// rather than framedDialog -- helpText alone wraps to 379 lines at 80x24
// (task 014's re-measurement after R73's added lines), far
// past the frame budget -- with m.helpScroll (PgUp/PgDn a page,
// up/down/j/k and the wheel a line; updateHelpView, R73)
// selecting the visible window instead of ever truncating content away.
// Task 082 (steer 005 item 2): the rendered body is styledHelpText, not
// the bare helpText -- see help_style.go's own doc comment for why the
// colouring lives here and never touches helpText itself.
func (m Model) helpView() string {
	return m.framedDialogScrollable(m.styledHelpText(), m.helpScroll)
}

// updateHelpView handles every key while the `?` help overlay is open
// (task 078). It is dispatched ahead of the list-mode switch exactly like
// updateDetailView/updateEventLog, which is why those two, and every
// `!m.help` guard still scattered through that switch, now only ever see
// m.help == false: PgUp/PgDn scroll the overlay's own window by a page
// and up/down (with their j/k sidebar aliases) by a single line (R73,
// issue #7); q/Ctrl+C
// still quit (matching the help text's own unconditional "q or Ctrl+C
// quit deck", not "while help is closed"); esc and a second ? both close
// it, esc also clearing the mark set exactly like the top-level esc case
// this replaces for m.help already did. Every other key is a no-op, same
// as the `!m.help` guards already made it.
func (m Model) updateHelpView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, handled := applyDialogContract(msg, dialogContract{Cancel: func() {
		m.help = false
		m.detail = false
		m.marked = nil
	}}); handled {
		return m, cmd
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.help = false
	case "pgup":
		m.helpScroll = m.dialogScrollByPage(m.helpScroll, helpText(m.settings.ASCII), -1)
	case "pgdown":
		m.helpScroll = m.dialogScrollByPage(m.helpScroll, helpText(m.settings.ASCII), 1)
	case "up", "k":
		// R73 (issue #7): the arrows and their sidebar aliases scroll by
		// ONE line, never a second PgUp/PgDn.
		m.helpScroll = m.dialogScrollByLines(m.helpScroll, helpText(m.settings.ASCII), -1)
	case "down", "j":
		m.helpScroll = m.dialogScrollByLines(m.helpScroll, helpText(m.settings.ASCII), 1)
	}
	return m, nil
}

func helpText(ascii bool) string {
	text := `deck help

Keys
  ↑/↓ or j/k select a session; as the selection settles (coalesced
    against DECK_PREVIEW_MS, not once per row while a key is held),
    deck also fits the newly selected session's window to the preview
    panel -- [ui] preview_fit, on by default -- the same cost ↵ below
    pays at entry: a SIGWINCH, and scrollback consumed faster while the
    window is narrower than usual. Skipped below the interactive floor
    (never resizes into a box too small to be worth it, and never
    enters interactive mode as a side effect); best-effort, so a session
    an attached client is also watching simply keeps that client's own
    size the next time it redraws. preview_fit = false turns this off
    and leaves a session cropped bottom-left instead. While ? help, E the
    event log or i the detail view covers the list, these same keys
    instead scroll that overlay's own content by exactly one line per
    press, leaving the session selection where it was
  PgUp/PgDn page up/down through the list, one page at a time; while ?
    help, E the event log or i the detail view covers the list, the
    same keys instead page that overlay's own content once it grows
    taller than the frame -- a whole page per press, so consecutive
    pages share no line; ↑/↓ or j/k there move one line at a time, and
    with mouse reporting on a wheel notch over the overlay does the same
  ↵ enter interactive mode on the selected running session: keystrokes
    forward to its live pane exactly as a real attached client's would,
    until Ctrl+Q leaves and returns the terminal to this list; entering
    resizes the agent's window to fit the preview panel, and output the
    agent produces while that window is narrower than its usual size
    consumes its own scrollback faster than the same output would at
    full width; while interactive, a wheel notch or Shift+PgUp/PgDn
    scrolls this bounded, deck-owned scrollback of the fitted view (not
    the pane's own tmux scrollback) rather than forwarding to the pane;
    typing snaps the view back to the live bottom
  a attach the selected running session (full-screen, like Ctrl+Q never
    happened -- ↵ enters interactive mode instead)
  Y acknowledge the selected waiting/error session, clear its unseen marker
  n create a session (shell, or an agent: claude or pi)
  x kill the selected running session; a toast naming undo stays visible
    for DECK_UNDO_MS afterward
  u undo the most recent x within its DECK_UNDO_MS window: resumes that
    session with its own resume argv, exactly like r; once the window has
    expired, or nothing has been killed since the last undo, it does nothing
  dd delete the selected session: the first d shows a pending indicator and
    changes nothing; Esc or any other key cancels it; the second d opens a
    confirm dialog naming what survives -- the conversation and its working
    directory are never touched; the dialog also offers a non-default
    purge choice naming the exact transcript path it will delete, or
    declining when the agent has none to locate; Enter kills the live pane
    (if any) and tombstones the row, which disappears from the list
    immediately; a toast naming undo stays visible for DECK_DELETE_GRACE_MS
    afterward -- u within that window restores the row (deleted_at
    cleared, back in the list); once the window expires the row is reaped
    and u does nothing
  A archive the selected session, hidden from the default list from then
    on: a confirm dialog opens first, naming the session -- and, on a row
    that is not stopped, saying that confirming does "kill and archive" as
    a single action rather than refusing the keypress the way x refuses an
    already-stopped row; nothing is written until you confirm;
    archived_at is a flag, never a status, so the row keeps whatever status
    it had; U below is the way back
  U unarchive the selected archived row: clears archived_at so the row
    returns to the default list, keeping whatever status it had (a session
    killed on its way into the archive comes back stopped -- r resumes it
    as a separate step). Reachable from inside the / filter's results,
    which is where an archived row is found in the first place: type enough
    of its name to surface it, Enter to keep the filter applied, then U
  m toggle a mark on the selected session (kept by session id, so it
    survives a re-sort or re-group); with the mark set non-empty, x and dd
    act on the whole batch instead of just the selected row, and ONE u
    restores the entire batch in one action; the marks clear on that
    action and on Esc
  r resume the selected stopped session with its own agent argv (never
    --continue or "most recent"); resumed agents read "starting · awaiting
    signal" until a hook or sampled probe reports ready, while live shells
    become "running" on reconciliation; a client that loses the launch-lease
    race sees "starting elsewhere" instead of an error
  R restart the selected non-stopped session: kills its live pane if one
    exists and relaunches it with the same resume argv and conversation id
    (never a fresh conversation); this is the only action that applies a
    pending env↻ edit to the new pane and clears env↻ once it is up
  P switch the permission profile of the selected session; only takes
    effect on the next launch or resume ("restart to apply"), never the
    live pane
  p pin the selected session's conversation id so future resumes always
    reuse it, or launch a one-shot fresh conversation (reverts to normal
    auto-resume afterward, it does not stay pinned or cleared)
  i toggle detail view for the selected session; r inside it renames the
    session's display name only -- the tmux session keeps its own name
    (deck_<slug>), never renamed, so a rename can never move or disturb a
    live pane's identity
  e open the env editor for the selected session: every key deck resolved a
    layer for, its effective value, and which layer won -- server env,
    captured_path, config [env] or session env (SPEC §6.1/§6.3); j/k select
    a key, Enter edits its value, typing then Enter saves it into the
    session's own env and marks it env-dirty until a later restart picks
    it up; the save mirrors into tmux's own environment for future panes,
    never into whatever the pane's already-running process started with;
    secret-shaped keys mask by default (§6.4); r toggles reveal, resets on
    reopen; Esc cancels an edit in progress, or closes the dialog otherwise
  E open/close the event log: every recorded event across every session --
    kind, reason, a bounded payload -- newest first; a payload's
    secret-shaped values mask the same way the env editor's do; Esc closes
  / filter the list by name, workspace or cwd, incrementally as you type;
    Enter keeps the filter applied and returns the keymap to the (now
    narrowed) list, Esc clears it back to the full list; this is also the
    only route to an archived session (A), which is hidden from the
    default list entirely -- type enough of its name, workspace or cwd to
    match it and it appears like any other row, and U there unarchives it
  space move to the next session needing attention (waiting or error),
    wrapping around; does nothing when nothing needs attention and never
    changes any session's status
  c toggle the selected row's workspace group collapsed/expanded
  g / G jump to the first / last visible row
  , open/close settings (edit config.toml's keys); Esc closes, prompting to
    discard if there are unsaved changes
  t open/close the theme picker; previews live on the real session list,
    Enter selects, Esc reverts byte-for-byte
  | cycle the layout mode: auto → side-by-side → stacked → collapsed →
    auto; a chosen mode is pinned regardless of terminal width until you
    cycle again or the terminal cannot hold its floors, when deck falls
    back to auto without forgetting the pin
  < / > shrink/grow the sidebar by one column
  ? open/close help; Esc closes help
  q or Ctrl+C quit deck

Create dialog fields
  Name                the display name; also the source of the tmux slug
  Working directory    the session's cwd; must exist and be a directory
  Agent               shell, claude, or pi; which adapter launches it
  Permission profile  how much the agent may do without asking (safe, plan,
                      edits, yolo); an unsupported profile for the chosen
                      agent degrades visibly instead of silently
  Launch args         extra argv appended after the adapter's own argv
  Env                 session-level environment variables (highest priority
                      in PATH resolution)
  Pre-launch command  runs in the pane before the agent starts; the SPEC's
                      intended use is loading secrets into the pane's
                      environment without deck ever storing or logging them
  Login shell         run the pane via $SHELL -lc instead of execing the
                      agent argv directly
  Tab or ↑/↓ changes field; ↵ advances or submits; Esc cancels

Yolo is gated by allow_yolo: it must be enabled in config.toml, or yolo is
not offered at all (the UI states why). Once allow_yolo is enabled, choosing
yolo -- at create time or when switching profile with P -- takes effect
immediately, with no separate confirm keystroke. yolo_default (also
config.toml, default false) opens the create modal already on yolo once
allow_yolo is also enabled; with allow_yolo disabled it is inert, and
settings says so on its own row rather than silently ignoring it. In yolo,
permission prompts never fire, so the waiting column goes quiet for that
session -- attention then comes only from questions/needs-input
notifications, not from waiting; this is expected, not deck failing to
notice, and is worth remembering now that yolo_default can put a brand new
session there without the user ever having chosen yolo by hand.

Settings takeover (opened with ,)
  Tab or Left/Right      switch focus between the category list and the
                         field list
  Up/Down or j/k         move within the focused list
  Enter or Space         toggle/cycle the focused field's value
  + / -                  adjust a bounded-integer field
  /                      fuzzy-search every field by label or description
  Ctrl+S                 save staged edits to config.toml
  Esc                    close; with unsaved changes, prompts to discard
                         (y/Enter discards, any other key keeps editing)
  [env] entries editor: - removes an entry, r reveals/masks secret-shaped
                         values (SPEC §6.4, resets to masked on reopen)

Theme picker (opened with t)
  Left/Right or Up/Down or Space   change the previewed theme
  Enter                  select the previewed theme and save it
  Esc                    revert to the theme active before the picker opened

Runtime controls
  DECK_HOME             isolated data/config/state root
  DECK_TMUX_SOCKET      private tmux socket (default: deck)
  DECK_CLOCK            freeze wall clock (RFC3339); clock.now overrides it
  DECK_CLOCK_STEP       exact amount advanced by each on-demand trigger
  Trigger             kill -USR1 <deck-client-pid>; each invocation advances
                        the shared clock by exactly DECK_CLOCK_STEP
  clock.now             shared RFC3339 state under the resolved data root;
                        the trigger updates it and every process reads it
  DECK_ID_SEED          deterministic generated UUIDs
  DECK_RECONCILE_MS     list/reconciliation interval in milliseconds
  DECK_PREVIEW_MS       pane-preview interval in milliseconds
  DECK_UNDO_MS          undo-toast window after x, in milliseconds
  DECK_DELETE_GRACE_MS  undo/reap window after dd, in milliseconds
  DECK_ASCII=1          use ASCII instead of optional glyphs
  DECK_ANIM=0           disable animation
  DECK_COLOR            explicitly enable or disable colour
  DECK_COLOR_DEPTH      force truecolor or 16-colour (quantised) rendering
                         instead of terminal auto-detection
  NO_COLOR              disable colour

Sessions use a private tmux server. Inspect it with:
  tmux -L deck ls
(or swap deck with DECK_TMUX_SOCKET). Plain tmux attach does not find deck
sessions. Attach clears TMUX because nested tmux is unsupported. Attached
clients share one pane geometry; the latest active client controls it.
With mouse reporting on, a wheel notch in an attached pane scrolls its own
scrollback via tmux's copy-mode instead of typing into the shell; the cost
is that a drag no longer makes the terminal's own text selection there
either -- hold your terminal's override modifier (usually Shift) to select
and copy pane text, or use tmux's own copy-mode.

Mouse (every binding duplicates a key above; nothing here is mouse-only)
  click a sidebar row       select it (like ↑/↓); the preview follows on
                            its next tick
  double-click a row        enter interactive mode (like ↵)
  click a group header      toggle that group's collapse (like g)
  wheel over the sidebar    scroll the list without changing selection
                            (like ↑/↓/PgUp/PgDn)
  wheel over an overlay     scroll ? help, E the event log or i the
                            detail view by one line (like ↑/↓ or j/k);
                            no other overlay scrolls, and a click or a
                            drag over any overlay still does nothing
  drag the seam             adjust sidebar_width live (like </>)
  click the collapsed strip restore the previous layout mode (like |)
  click over the preview     does nothing; a click outside a dialog does
                            nothing (Esc cancels)
  wheel over the preview     while interactive, scrolls the grid's own
                            bounded scrollback (like Shift+PgUp/PgDn);
                            otherwise does nothing
  drag over the preview      while interactive, selects text; releasing
                            copies it into deck's own tmux buffer and
                            (best-effort) the system clipboard via OSC 52
                            (like a, then tmux's own copy-mode); otherwise
                            does nothing -- hold your terminal's override
                            modifier (usually shift) for its own selection
  DECK_MOUSE=0 (or [ui] mouse = false) disables all mouse reporting and
  every mouse binding above; every keyboard path keeps working, only the
  shortcuts are lost
  Mouse reporting takes over the terminal's own click-drag text selection;
  hold your terminal's override modifier (usually shift) to select and
  copy text while deck is running

? closes help; Esc closes help; q quits deck.
`
	if ascii {
		return strings.NewReplacer("↵", "Enter", "·", "-", "—", "-").Replace(text)
	}
	return text
}

// TmuxHealth verifies the runtime dependency without creating a tmux server.
func TmuxHealth(settings config.Settings) string {
	client := tmux.Client{Socket: settings.Socket}
	if _, err := client.Discover(context.Background()); err != nil {
		return err.Error()
	}
	return ""
}
