package features

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Socket aliasing (task cure-01-01, R158).
//
// Three scenarios exist to prove SPEC §3.4's own socket derivation end to
// end through the released binary: a plain default install uses socket
// "deck" (profile_default_unchanged.feature), and a named profile uses
// "deck-<profile>" (profile_creation.feature, profile_side_by_side.feature).
// Those names ARE the operator's live namespace, and ci/tmux-guard.sh
// refuses every "-L deck"/"-L deck-*" outright, with no exemption. So
// instead of letting deck reach those servers, these scenarios put a
// fixture `tmux` first on the client's PATH that:
//
//   - records the socket name deck itself derived and asked for (so the
//     scenario can still assert it was exactly "deck" / "deck-<profile>"),
//   - rewrites that one "-L <name>" argument to socketAlias(<name>), a
//     private per-scenario name outside the deck/deck-* namespace, and
//   - execs whatever `tmux` the test process itself resolves (under
//     ci/run.sh that is the guard, so the guard still sees every call).
//
// deck's production naming is untouched (the header still reads
// "socket: deck-<profile>"); only the server the fixture tmux actually
// talks to is private, and it is killed and probed at teardown like
// h.Socket.

// socketAliasPrefix is what the fixture tmux prepends to a requested
// deck/deck-* name. h.Socket ("deck_test_<pid>_<seq>") is itself outside
// the forbidden namespace, and so is anything that starts with it.
func (h *ScenarioHarness) socketAliasPrefix() string { return h.Socket + "-alias-" }

// socketAlias is the private tmux socket a scenario's fixture tmux really
// uses when deck asks for requested (e.g. "deck" or "deck-a").
func (h *ScenarioHarness) socketAlias(requested string) string {
	return h.socketAliasPrefix() + requested
}

func (h *ScenarioHarness) socketAliasDir() string { return filepath.Join(h.Home, "socket-alias-bin") }

func (h *ScenarioHarness) socketAliasLog() string {
	return filepath.Join(h.socketAliasDir(), "requested-sockets.log")
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// ensureSocketAliasTMux writes the fixture tmux (once per scenario) and
// returns its directory.
func (h *ScenarioHarness) ensureSocketAliasTMux() (string, error) {
	dir := h.socketAliasDir()
	shim := filepath.Join(dir, "tmux")
	if _, err := os.Stat(shim); err == nil {
		return dir, nil
	}
	next, err := exec.LookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("resolve the tmux the socket-alias fixture delegates to: %w", err)
	}
	if next, err = filepath.Abs(next); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create socket-alias fixture dir: %w", err)
	}
	// deck's tmux client always passes "-L <socket>" as its first two
	// arguments (internal/tmux Client.command). Only a leading -L whose
	// value is deck/deck-* is rewritten; anything else (including an
	// attached "-Ldeck" form) is passed on untouched, where the guard
	// refuses it.
	// Plain /bin/sh rather than bash: deck polls tmux every few tens of
	// milliseconds in these scenarios, so the shim's own start-up cost is
	// kept as small as possible.
	script := `#!/bin/sh
next=` + shellQuote(next) + `
if [ "$#" -ge 2 ] && [ "$1" = "-L" ]; then
	case "$2" in
	deck | deck-*)
		printf '%s\n' "$2" >>` + shellQuote(h.socketAliasLog()) + `
		socket=` + shellQuote(h.socketAliasPrefix()) + `"$2"
		shift 2
		exec "$next" -L "$socket" "$@"
		;;
	esac
fi
exec "$next" "$@"
`
	if err := os.WriteFile(shim, []byte(script), 0o700); err != nil {
		return "", fmt.Errorf("write socket-alias fixture tmux: %w", err)
	}
	return dir, nil
}

// socketAliasPATHEnv is the PATH= entry a client proving deck's own socket
// derivation must be started with: the fixture tmux first, then the
// scenario's agent fixture dir if it has one (a PATH= entry in extraEnv
// suppresses StartClientWithSize's own agentPATHDir prepend), then the
// test process's own PATH.
func (h *ScenarioHarness) socketAliasPATHEnv() (string, error) {
	dir, err := h.ensureSocketAliasTMux()
	if err != nil {
		return "", err
	}
	parts := []string{dir}
	if h.agentPATHDir != "" {
		parts = append(parts, h.agentPATHDir)
	}
	parts = append(parts, os.Getenv("PATH"))
	return "PATH=" + strings.Join(parts, string(os.PathListSeparator)), nil
}

// registerSocketAlias makes Close kill and probe the private server behind
// requested, once.
func (h *ScenarioHarness) registerSocketAlias(requested string) {
	alias := h.socketAlias(requested)
	for _, socket := range h.extraSockets {
		if socket == alias {
			return
		}
	}
	h.extraSockets = append(h.extraSockets, alias)
}

// requireDeckRequestedSocket is the derivation half of a "live on socket
// <name>" step: deck itself must have asked the fixture tmux for exactly
// requested at least once in this scenario.
func (h *ScenarioHarness) requireDeckRequestedSocket(requested string) error {
	file, err := os.Open(h.socketAliasLog())
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deck never asked tmux for any deck/deck-* socket, want %q", requested)
	}
	if err != nil {
		return fmt.Errorf("read socket-alias request log: %w", err)
	}
	defer file.Close()
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		seen[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read socket-alias request log: %w", err)
	}
	if !seen[requested] {
		names := make([]string, 0, len(seen))
		for name := range seen {
			names = append(names, name)
		}
		return fmt.Errorf("deck never asked tmux for socket %q (it asked for %q)", requested, names)
	}
	return nil
}
