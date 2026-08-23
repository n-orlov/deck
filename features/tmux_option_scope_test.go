package features

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// tmuxOptionScope names which of tmux's two per-object option tables
// `show-options` reads from for a single option name: the type-appropriate
// *global* table (server-wide for a server option, session-wide for a
// session option, window-wide for a window option -- tmux picks the right
// one for `-g` itself based on the option's own declared type) versus the
// *window-local* table, populated only by an explicit `-w` set targeted at
// that one window. PRD phase3b requirement 3 (II-3): a geometry assertion
// that can only see a merged/effective value, not which of these two tables
// it came from, cannot prove requirements 8 and 10, which are about scope
// itself (resize-window's window-local `window-size manual` shadowing a
// server-global `window-size latest`).
type tmuxOptionScope string

const (
	// tmuxOptionScopeGlobal is `show-options -gv`.
	tmuxOptionScopeGlobal tmuxOptionScope = "-gv"
	// tmuxOptionScopeWindow is `show-options -wv`.
	tmuxOptionScopeWindow tmuxOptionScope = "-wv"
)

// parseTmuxOptionScope maps the Gherkin word to the tmux flag pair. Any word
// other than "global" is treated as "window" -- the step's own regex already
// only accepts the two words, so this never has a third case to reject.
func parseTmuxOptionScope(word string) tmuxOptionScope {
	if word == "global" {
		return tmuxOptionScopeGlobal
	}
	return tmuxOptionScopeWindow
}

// tmuxOptionState is what one scope read actually saw. Two entirely
// different tmux exit shapes both collapse to Set == false here, matching
// the PRD's own "treat both as none" instruction: a builtin (non-"@")
// option that is unset in the queried scope prints an empty line and exits
// 0, while an unset user ("@"-prefixed) option makes tmux exit non-zero
// with "invalid option: <name>". A caller that trusted only the exit code,
// or only output emptiness, would get one of the two wrong.
type tmuxOptionState struct {
	Value string
	Set   bool
}

// readTmuxOptionInScope runs `tmux -L socket show-options <scope> -t target
// name` and normalizes both of tmux's "unset" shapes into
// tmuxOptionState{Set: false}. It returns a genuine error only when tmux
// fails for a reason other than "not present in this scope" (e.g. no such
// target). tmux gives the identical "invalid option: <name>" message for a
// user option that is merely unset here and for a name it has never heard
// of at all; distinguishing those two is out of scope for a scope assertion
// and left to whatever step chose the option name in the first place.
func readTmuxOptionInScope(ctx context.Context, socket, target string, scope tmuxOptionScope, name string) (tmuxOptionState, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := []string{"-L", socket, "show-options", string(scope), "-t", target, name}
	output, err := exec.CommandContext(commandCtx, "tmux", args...).CombinedOutput()
	trimmed := strings.TrimRight(string(output), "\n")
	if err != nil {
		if strings.Contains(trimmed, "invalid option") {
			return tmuxOptionState{}, nil
		}
		return tmuxOptionState{}, fmt.Errorf("tmux -L %s show-options %s -t %s %s: %w: %s", socket, scope, target, name, err, trimmed)
	}
	if trimmed == "" {
		return tmuxOptionState{}, nil
	}
	return tmuxOptionState{Value: trimmed, Set: true}, nil
}

// assertTmuxOptionInScope asserts that name reads as want in exactly the
// named scope of target -- set in the *other* scope, or set to a different
// value in this one, or genuinely unset, are all failures, never silently
// satisfied by a merged/effective value from elsewhere.
func assertTmuxOptionInScope(ctx context.Context, socket, target string, scope tmuxOptionScope, name, want string) error {
	got, err := readTmuxOptionInScope(ctx, socket, target, scope, name)
	if err != nil {
		return err
	}
	if !got.Set {
		return fmt.Errorf("tmux option %q in %s scope of %q is unset, want %q", name, scopeWord(scope), target, want)
	}
	if got.Value != want {
		return fmt.Errorf("tmux option %q in %s scope of %q = %q, want %q", name, scopeWord(scope), target, got.Value, want)
	}
	return nil
}

// assertTmuxOptionUnsetInScope asserts name is unset in exactly the named
// scope of target, treating tmux's two different "unset" exit shapes
// (see tmuxOptionState) identically, per the PRD.
func assertTmuxOptionUnsetInScope(ctx context.Context, socket, target string, scope tmuxOptionScope, name string) error {
	got, err := readTmuxOptionInScope(ctx, socket, target, scope, name)
	if err != nil {
		return err
	}
	if got.Set {
		return fmt.Errorf("tmux option %q in %s scope of %q = %q, want unset", name, scopeWord(scope), target, got.Value)
	}
	return nil
}

func scopeWord(scope tmuxOptionScope) string {
	if scope == tmuxOptionScopeGlobal {
		return "global"
	}
	return "window"
}

// registerTmuxOptionScopeSteps wires the harness prerequisite requirement 3
// (II-3) asks for before any geometry-owning requirement (7-13) can be
// proven: a step that can see which of tmux's two option tables a value
// came from, not merely its merged/effective value.
func registerTmuxOptionScopeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^tmux window "([^"]+)" option "([^"]+)" is "([^"]+)" in the (global|window) scope$`, tmuxWindowOptionIsInScope)
	sc.Step(`^tmux window "([^"]+)" option "([^"]+)" is unset in the (global|window) scope$`, tmuxWindowOptionIsUnsetInScope)
}

func tmuxWindowOptionIsInScope(ctx context.Context, target, name, want, scopeWord string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	return assertTmuxOptionInScope(ctx, h.Socket, target, parseTmuxOptionScope(scopeWord), name, want)
}

func tmuxWindowOptionIsUnsetInScope(ctx context.Context, target, name, scopeWord string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	return assertTmuxOptionUnsetInScope(ctx, h.Socket, target, parseTmuxOptionScope(scopeWord), name)
}
