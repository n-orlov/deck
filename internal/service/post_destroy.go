package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// postDestroyTimeout is SPEC \u00a79.2's bounded teardown timeout: a code
// value applied independently to each of the (up to two) hooks a single
// Archive/Delete runs, deliberately never a config key (unlike pre_launch,
// a teardown hook cannot be allowed to hang the action that already
// happened, so there is nothing for an operator to usefully tune here). It
// is an immutable constant -- every caller, test or production, sees the
// same 30s value; a test that needs a shorter bound to prove the
// timeout-kill path calls runOneTeardownHook directly with an explicit
// timeout argument instead of mutating this value.
const postDestroyTimeout = 30 * time.Second

// TeardownKindArchive and TeardownKindDelete are the two DECK_TEARDOWN_KIND
// values SPEC \u00a79.2 defines: which of the two actions that run post_destroy
// (`A` or `dd`) fired it, so one hook line can serve both.
const (
	TeardownKindArchive = "archive"
	TeardownKindDelete  = "delete"
)

// runPostDestroy is SPEC \u00a79.2's teardown-hook composition, called only
// after the caller's own archive/delete write has durably committed: the
// session's own PostDestroy runs first, then the service's own
// GlobalPostDestroy -- the reverse of \u00a76.5's launch order, releasing the
// specific before the general. Each hook is its own subprocess of deck,
// never a pane (the pane is already dead), under its own postDestroyTimeout,
// so neither hook ever observes the other's environment or exit status.
// Both are fail-open: a non-zero exit or a timeout never resurrects the row
// or reverses the action it followed. Each failure is recorded durably as a
// session-scoped "note" event (store.RecordSessionNote); the returned
// string is empty when every configured hook (there may be zero, one or
// two) ran cleanly, and otherwise names every hook that failed, for the
// caller to hand to a toast.
func (s Service) runPostDestroy(ctx context.Context, session store.Session, teardownKind string) string {
	type namedHook struct {
		label string
		line  string
	}
	var hooks []namedHook
	if session.PostDestroy != "" {
		hooks = append(hooks, namedHook{label: "session", line: session.PostDestroy})
	}
	if s.GlobalPostDestroy != "" {
		hooks = append(hooks, namedHook{label: "global", line: s.GlobalPostDestroy})
	}
	if len(hooks) == 0 {
		return ""
	}
	env := s.teardownEnv(session, teardownKind)
	var failures []string
	for _, hook := range hooks {
		if failure := s.runOneTeardownHook(ctx, session, hook.label, hook.line, env, postDestroyTimeout); failure != "" {
			failures = append(failures, failure)
		}
	}
	if len(failures) == 0 {
		return ""
	}
	message := failures[0]
	for _, f := range failures[1:] {
		message += "; " + f
	}
	return message
}

// teardownEnv is SPEC \u00a79.2's teardown environment: the same \u00a76.1 session
// context every launch builds (sessionContextEnv), minus
// DECK_SESSION_LAUNCH_KIND (which has no meaning once the pane is gone) and
// plus DECK_TEARDOWN_KIND (archive or delete), layered over the deck
// process's own inherited environment exactly the way a launched pane's
// environment layers over the tmux server's -- so a hook still sees a
// normal PATH etc. without deck having to reconstruct one.
func (s Service) teardownEnv(session store.Session, teardownKind string) []string {
	ctxEnv := s.sessionContextEnv(session, "")
	delete(ctxEnv, "DECK_SESSION_LAUNCH_KIND")
	ctxEnv["DECK_TEARDOWN_KIND"] = teardownKind
	env := os.Environ()
	for key, value := range ctxEnv {
		env = append(env, key+"="+value)
	}
	return env
}

// runOneTeardownHook runs a single teardown hook line as its own deck
// subprocess under the given timeout, records a durable "note" event on a
// non-zero exit or a timeout, and returns a non-empty failure description
// in that case (empty on success). ctx is the caller's own context (never
// the hook's timeout-bound child context, which may already have expired
// by the time the note is written). Every production caller passes the
// package constant postDestroyTimeout; timeout is an explicit parameter
// (rather than the helper reading the constant itself) solely so a test
// proving the timeout-kill path can call this same helper directly with a
// short bound, without mutating any production value or adding a second,
// test-only knob or branch anywhere in this file.
func (s Service) runOneTeardownHook(ctx context.Context, session store.Session, label, line string, env []string, timeout time.Duration) string {
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hookCtx, "/bin/sh", "-c", line)
	cmd.Env = env
	err := cmd.Run()
	if err == nil {
		return ""
	}
	var message string
	if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
		message = fmt.Sprintf("%s post_destroy timed out after %s", label, timeout)
	} else {
		message = fmt.Sprintf("%s post_destroy failed: %v", label, err)
	}
	if s.Store != nil && s.Clock != nil {
		_ = s.Store.RecordSessionNote(ctx, session.ID, message, s.Clock.Now().UnixMilli())
	}
	return message
}
