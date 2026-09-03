package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// Delete is task 105's `dd` submit: it kills the live pane if one exists
// (a stopped session is left as-is, exactly like a plain Kill would refuse
// to double-kill) and then tombstones the row via store.SoftDeleteSession,
// so it disappears from the default ListSessions view immediately while
// remaining restorable within the grace window task 106 implements. It
// never touches the session's cwd or its conversation id/transcript —
// those are exactly what the confirm dialog states survives.
//
// Once the tombstone itself has durably committed, Delete runs SPEC §9.2's
// teardown hooks (runPostDestroy, task 013), exactly as Archive does. The
// returned string is the hook failure message for a caller's toast --
// empty when nothing was configured or every configured hook ran cleanly
// -- and is never an error: a teardown hook can never undo a delete that
// has already happened.
func (s Service) Delete(ctx context.Context, session store.Session) (string, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return "", errors.New("session delete requires store, audit logger, and clock")
	}
	if session.ID == "" {
		return "", errors.New("session delete requires a durable session id")
	}
	if session.Status != "stopped" {
		if session.Slug == "" {
			return "", errors.New("session delete requires a durable slug to kill a live pane")
		}
		if err := s.TMux.Kill(ctx, session.Slug); err != nil {
			return "", fmt.Errorf("kill tmux session %q: %w", session.Name, err)
		}
		if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
			SessionID:    session.ID,
			Status:       "stopped",
			Reason:       "killed by user",
			Source:       "user",
			At:           s.Clock.Now().UnixMilli(),
			EventKind:    "killed",
			KilledByUser: true,
		}); err != nil {
			return "", fmt.Errorf("record killed session %q: %w", session.Name, err)
		}
		if err := s.Audit.Transition(session.ID, "killed"); err != nil {
			return "", fmt.Errorf("audit killed session %q: %w", session.Name, err)
		}
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.SoftDeleteSession(ctx, session.ID, at); err != nil {
		return "", fmt.Errorf("tombstone session %q: %w", session.Name, err)
	}
	if err := s.Audit.Transition(session.ID, "deleted"); err != nil {
		return "", fmt.Errorf("audit deleted session %q: %w", session.Name, err)
	}
	return s.runPostDestroy(ctx, session, TeardownKindDelete), nil
}

// Restore is task 106's `u` undo for a completed dd: within
// DECK_DELETE_GRACE_MS it clears the tombstone (store.RestoreSession),
// returning the row to the default ListSessions view. It never resumes or
// relaunches a live pane -- Delete's own kill step is not undone by this;
// restoring only ever un-hides the row, exactly as the confirm dialog's
// own "kills the live pane (if any)" wording implies that kill is final.
func (s Service) Restore(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil {
		return store.Session{}, errors.New("session restore requires store, audit logger, and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session restore requires a durable session id")
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.RestoreSession(ctx, sessionID, at); err != nil {
		return store.Session{}, fmt.Errorf("restore session %q: %w", sessionID, err)
	}
	if err := s.Audit.Transition(sessionID, "restored"); err != nil {
		return store.Session{}, fmt.Errorf("audit restored session %q: %w", sessionID, err)
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("read restored session %q: %w", sessionID, err)
	}
	return session, nil
}

// Reap is task 106's DECK_DELETE_GRACE_MS expiry: once the grace window
// has elapsed since Delete tombstoned a row and it was never restored,
// the row is permanently removed (store.ReapSession, which cascades the
// row's own events -- and, with them, every deck-owned row hanging off
// the session: there is no separate outbox or waiting/notify_epoch table
// in this schema, so ON DELETE CASCADE already leaves nothing of those
// behind). Requirement 24's deeper cascade -- deck's own per-session
// *files* -- is task 107's addition here: the captures directory and the
// §9.4 history file (config.CapturesDir/config.HistoryFile, the single
// place those paths are defined) are removed if present, and their
// absence -- the common case today, since nothing in this tree writes
// either one yet -- is never an error. The JSONL audit log is deliberately
// never touched: it keeps this session's earlier history past the reap,
// per SPEC §9.2's "a log that rewrites itself ... is not a log".
func (s Service) Reap(ctx context.Context, sessionID string) error {
	if s.Store == nil || s.Clock == nil {
		return errors.New("session reap requires store and clock")
	}
	if sessionID == "" {
		return errors.New("session reap requires a durable session id")
	}
	at := s.Clock.Now().UnixMilli()
	if err := s.Store.ReapSession(ctx, sessionID, at); err != nil {
		return fmt.Errorf("reap session %q: %w", sessionID, err)
	}
	return s.removeReapedSessionFiles(sessionID)
}

// removeReapedSessionFiles is the filesystem half of a reap, shared by the
// grace-window Reap above and by the create path's name-reuse reap
// (reapedHolderFiles below): the captures directory and the §9.4 history
// file, the two paths config.CapturesDir/config.HistoryFile define, are
// removed if present and their absence is never an error. It is always
// called AFTER the SQL removal has committed -- files are the one part of
// a reap that cannot be rolled back, so nothing may delete them while the
// row is still restorable.
func (s Service) removeReapedSessionFiles(sessionID string) error {
	if s.DeckHome == "" {
		return nil
	}
	if err := os.RemoveAll(config.CapturesDir(s.DeckHome, sessionID)); err != nil {
		return fmt.Errorf("remove captures for reaped session %q: %w", sessionID, err)
	}
	if err := os.Remove(config.HistoryFile(s.DeckHome, sessionID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove history file for reaped session %q: %w", sessionID, err)
	}
	return nil
}

// tombstonedNameHolders reads, BEFORE a create runs, which tombstoned rows
// currently hold the name that create is about to take -- i.e. exactly the
// rows store.CreateSession will reap inside its own transaction (R77,
// SPEC.md §9.2, task 003). The ids are only remembered here; nothing is
// deleted until reapedHolderFiles runs after the commit.
func (s Service) tombstonedNameHolders(ctx context.Context, name string) ([]string, error) {
	if s.DeckHome == "" {
		return nil, nil
	}
	ids, err := s.Store.TombstonedNameHolders(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("check deleted holders of session name %q: %w", name, err)
	}
	return ids, nil
}

// reapedHolderFiles completes a name-reuse reap on the filesystem, and is
// called only ever AFTER store.CreateSession's transaction has committed:
// SPEC §9.2 says taking a tombstoned session's name reaps that session,
// and a reap removes deck's own per-session files as well as its row. Each
// candidate is re-checked against the store first, so a holder that is no
// longer gone -- restored, or never reaped because the create took a
// different name in the end -- keeps its scrollback.
func (s Service) reapedHolderFiles(ctx context.Context, candidateIDs []string) error {
	for _, id := range candidateIDs {
		exists, err := s.Store.SessionRowExists(ctx, id)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := s.removeReapedSessionFiles(id); err != nil {
			return err
		}
	}
	return nil
}

// Purge is task 110's non-default "purge conversation" choice inside the
// dd confirm dialog (SPEC.md:684-691, requirement 26): it deletes exactly
// the path the caller already resolved via the session's own agent
// adapter's declared TranscriptPaths (task 109, internal/agent) -- it
// never resolves, infers or globs a path itself. An empty path is a
// no-op, mirroring TranscriptPaths' own "cannot locate" contract: the
// caller never calls Purge at all unless that lookup already succeeded,
// but treating "" as a no-op here rather than a panic keeps the two
// contracts consistent instead of trusting the caller never to slip.
func (s Service) Purge(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("purge transcript %q: %w", path, err)
	}
	return nil
}
