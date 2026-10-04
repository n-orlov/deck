package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/n-orlov/deck/internal/store"
)

// envKeyPattern is the same shape env.go's editor already only ever offers
// (a plain [env] table key or a config-declared one) -- guarding it again
// here means InjectEnv never hands tmux send-keys anything but a value
// that already looks like a shell identifier before it becomes part of a
// literal `export ...` command line.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// InjectEnv implements task 023's "inject instead" alternative to `R` for
// shell sessions (SPEC §6.2/§6.3): rather than killing and relaunching the
// pane (Restart), it exports every env key changed since the session's
// env_dirty flag was last cleared directly into the LIVE shell process,
// by typing `export KEY=value` into its already-running pane via
// tmux send-keys -- the pane's pid never changes, since nothing is
// killed or relaunched. It then clears env_dirty exactly as Restart does,
// since those keys are now genuinely applied to the running process, not
// merely mirrored into tmux's own environment table for a future pane.
//
// It only ever applies to a shell session: an agent's own REPL has no
// reason to treat a typed `export` line as anything but ordinary chat
// input, so there is no safe "inject instead" for it -- SPEC §6.2/§6.3's
// pending-edit apply for a coding agent session is Restart alone.
func (s Service) InjectEnv(ctx context.Context, sessionID string) (store.Session, []string, error) {
	session, err := s.loadInjectableSession(ctx, sessionID)
	if err != nil {
		return session, nil, err
	}
	keys, err := s.Store.DirtyEnvKeys(ctx, sessionID)
	if err != nil {
		return session, nil, fmt.Errorf("list changed environment keys for session %q: %w", session.Name, err)
	}
	if len(keys) == 0 {
		return session, nil, nil
	}
	if err := s.sendEnvExports(ctx, session, keys); err != nil {
		return session, nil, err
	}
	if err := s.Store.MarkEnvInjected(ctx, sessionID, s.Clock.Now().UnixMilli()); err != nil {
		return session, nil, fmt.Errorf("clear env_dirty after injecting into session %q: %w", session.Name, err)
	}
	updated, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return session, keys, fmt.Errorf("get session %q after inject: %w", sessionID, err)
	}
	return updated, keys, nil
}

// loadInjectableSession is InjectEnv's up-front half: the dependency and id
// guards, the row lookup, and the shell-only / slug / live-pane refusals.
func (s Service) loadInjectableSession(ctx context.Context, sessionID string) (store.Session, error) {
	if s.Store == nil || s.Clock == nil {
		return store.Session{}, errors.New("injecting environment requires a store and clock")
	}
	if sessionID == "" {
		return store.Session{}, errors.New("session id is required")
	}
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return store.Session{}, fmt.Errorf("get session %q: %w", sessionID, err)
	}
	if session.Agent != "shell" {
		return session, fmt.Errorf("cannot inject environment into session %q: inject-instead only applies to shell sessions (use R to restart %s)", session.Name, session.Agent)
	}
	if session.Slug == "" {
		return session, errors.New("inject requires a durable session slug")
	}
	// R88: this must be R69's "has a live pane" notion (HasLivePane), not
	// mere has-session existence (Exists). A retained dead pane (deck's
	// server runs remain-on-exit failed, so a pane that exits non-zero is
	// kept together with its session -- issue #6's trap) makes Exists
	// report true forever, which would let this fall through into the
	// SendKeys loop over a corpse; HasLivePane correctly reports false for
	// it, so the refusal fires here, before any tmux command is attempted
	// against the dead pane at all.
	live, err := s.TMux.HasLivePane(ctx, session.Slug)
	if err != nil {
		return session, fmt.Errorf("check live pane for session %q: %w", session.Name, err)
	}
	if !live {
		return session, fmt.Errorf("cannot inject environment into session %q: a stopped or error row has no live pane, so it cannot take an injection (restart it instead -- SPEC \u00a76.4's restart-to-apply route)", session.Name)
	}
	return session, nil
}

// sendEnvExports types `export KEY=value` into the live pane for each key,
// validating every key as a shell identifier immediately before its own send.
func (s Service) sendEnvExports(ctx context.Context, session store.Session, keys []string) error {
	for _, key := range keys {
		if !envKeyPattern.MatchString(key) {
			return fmt.Errorf("cannot inject environment key %q into session %q: not a valid shell identifier", key, session.Name)
		}
		value := session.Env[key]
		if err := s.TMux.SendKeys(ctx, session.Slug, "export "+key+"="+shellSingleQuote(value)); err != nil {
			return fmt.Errorf("inject environment key %q into session %q: %w", key, session.Name, err)
		}
	}
	return nil
}

// shellSingleQuote wraps value in POSIX single quotes, escaping any
// embedded single quote as '\” -- the standard shell-safe quoting so an
// injected value (which may contain spaces, `$`, backticks or anything
// else) reaches the shell's export as one literal argument, never
// interpreted.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
