package features

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerProfileDefaultUnchangedSteps wires SPEC §3.4's R156 default-install
// scenario: unlike every other scenario in this package, which points every
// client at DECK_HOME=h.Home (a shortcut real installations never use),
// this one clears DECK_HOME and instead sets HOME (plus every XDG_* root
// cleared alongside it) to a scenario-owned temp directory, so
// internal/config's own $HOME/.config, $HOME/.local/share, $HOME/.local/state
// resolution runs for real (config_file_test.go's own comment names this
// same resolution). The point is proving that introducing named profiles
// left the pre-existing default install's flat layout, socket and log
// completely unaffected when resolved the way a real, unconfigured
// installation actually would be -- not merely under the DECK_HOME
// convenience every other scenario relies on.
func registerProfileDefaultUnchangedSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a default install already exists at a temp home, with config\.toml marked "([^"]+)" and deck client "([^"]+)" having already created shell session "([^"]+)"$`, defaultInstallSeeded)
	sc.Step(`^deck client "([^"]+)" is started again on that same default install with no argument and no DECK_PROFILE$`, defaultInstallRestarted)
	sc.Step(`^the default install header for deck client "([^"]+)" reads exactly "([^"]+)"$`, defaultInstallHeaderReadsExactly)
	sc.Step(`^the default install session "([^"]+)" is a live tmux session on socket "deck"$`, defaultInstallSessionIsLiveOnSocketDeck)
	sc.Step(`^the default install's config\.toml still contains the marker "([^"]+)"$`, defaultInstallConfigStillContainsMarker)
	sc.Step(`^deck client "([^"]+)" draws ASCII chrome, which only the default install's flat config\.toml turns on$`, defaultInstallClientDrawsASCIIChrome)
	sc.Step(`^the default install session "([^"]+)"'s live pane environment has "([^"]+)" set to "([^"]+)" by the flat config\.toml's \[env\] table$`, defaultInstallPaneEnvironmentHasConfigValue)
	sc.Step(`^the default install's flat state database holds exactly the sessions "([^"]+)" and "([^"]+)"$`, defaultInstallStateDatabaseHoldsExactlySessions)
	sc.Step(`^the default install's flat log directory holds a deck\.jsonl file$`, defaultInstallLogDirectoryHoldsJSONLFile)
	sc.Step(`^no profiles directory exists under the default install's temp config, data or state roots$`, defaultInstallHasNoProfilesDirectories)
}

// defaultInstallConfigRoot/DataRoot/StateRoot mirror internal/config's own
// XDG defaults (SPEC §2's table) applied to home: $HOME/.config,
// $HOME/.local/share, $HOME/.local/state -- the three DIFFERENT roots this
// scenario's own success criteria name, as opposed to the single shared
// root DECK_HOME collapses them into everywhere else in this package.
func defaultInstallConfigRoot(home string) string { return filepath.Join(home, ".config", "deck") }
func defaultInstallDataRoot(home string) string {
	return filepath.Join(home, ".local", "share", "deck")
}
func defaultInstallStateRoot(home string) string {
	return filepath.Join(home, ".local", "state", "deck")
}

// startDefaultInstallClient starts (or restarts) client name against
// h.defaultInstallHome's own $HOME/XDG_* resolution rather than the
// harness's usual DECK_HOME shortcut: DECK_HOME is cleared, HOME points at
// the temp home, every XDG_* variable is cleared alongside it so
// resolvePaths falls all the way through to $HOME's own defaults, and
// DECK_TMUX_SOCKET is cleared too so config.go's own DefaultSocket ("deck")
// applies for real -- this task's own "the session lives on socket deck"
// criterion. The socket is an ephemeral, sibling-container-local tmux
// server (see StartNamedClientForNewProfile's own doc comment for why this
// is never the operator's own machine), registered once in h.extraSockets
// so Close kills and probes it exactly like every other socket this
// harness owns. A prior client already registered under name (e.g. this
// same scenario's own earlier "having already created" seed run, already
// exited by the time this is called again) is torn down and its map entry
// dropped first, mirroring clientIsRestartedWithSize's own "already
// exited" handling, so the same client name can be reused across the
// seed run and the scenario's own real restart.
func startDefaultInstallClient(ctx context.Context, h *ScenarioHarness, name string) error {
	if h.defaultInstallHome == "" {
		return errors.New("default install temp home was not seeded before starting a client on it")
	}
	if existing, ok := h.namedClients[name]; ok {
		select {
		case <-existing.done:
			// already exited (e.g. this scenario's own earlier seed run)
		default:
			if err := existing.Stop(5 * time.Second); err != nil {
				return fmt.Errorf("stop deck client %q before restarting it on the default install: %w", name, err)
			}
		}
		delete(h.namedClients, name)
	}
	registered := false
	for _, socket := range h.extraSockets {
		if socket == "deck" {
			registered = true
			break
		}
	}
	if !registered {
		h.extraSockets = append(h.extraSockets, "deck")
	}
	// DECK_ASCII= clears h.Environment's own DECK_ASCII=1 default (exec
	// keeps the last duplicate key, and config's getenv treats an empty
	// value as unset), so ASCII chrome can only come from the flat
	// config.toml's [ui] ascii = true -- the observable
	// defaultInstallClientDrawsASCIIChrome relies on to prove that file is
	// the one actually loaded, not merely left on disk.
	client, err := h.StartNamedClient(ctx, name,
		"DECK_HOME=", "HOME="+h.defaultInstallHome,
		"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_STATE_HOME=",
		"DECK_TMUX_SOCKET=", "DECK_ASCII=")
	if err != nil {
		return err
	}
	// Ready on the header text rather than the "deck - sessions" title:
	// the title's dash is ASCII only when the flat config.toml's [ui] ascii
	// was loaded, and that is for defaultInstallClientDrawsASCIIChrome to
	// report by name, not for a readiness wait to time out on.
	return client.WaitForFrame(ctx, false, "socket: deck")
}

// defaultInstallSeeded is this scenario's own "Given": it creates the temp
// $HOME, pre-writes a marked config.toml directly at the flat path a real
// install would already have on disk, then runs a real deck client through
// this scenario's own startDefaultInstallClient to create the "prior"
// session for real (through the released binary, not a hand-rolled SQL
// fixture) and exits it -- leaving name's tmux pane alive on socket "deck"
// and a genuinely pre-existing install (config.toml, state.db and log all
// already on disk) for the later "started again" step to resume, rather
// than merely pretending one existed.
func defaultInstallSeeded(ctx context.Context, marker, name, sessionName string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	home := filepath.Join(h.Home, "default-install-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create default install temp home: %w", err)
	}
	h.defaultInstallHome = home
	configDir := defaultInstallConfigRoot(home)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("pre-create default install config directory: %w", err)
	}
	// Two settings deck can only have learned from THIS file: [ui] ascii
	// (the harness's own DECK_ASCII=1 override is cleared for these
	// clients, and deck's default is Unicode chrome) and an [env] entry
	// no other layer supplies, which config [env] hands to every pane deck
	// creates (SPEC §6.1). Both are asserted after the restart.
	content := fmt.Sprintf("# %s\n[ui]\nascii = true\n\n[env]\n%s = %q\n", marker, defaultInstallEnvKey, marker)
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("pre-write default install config.toml: %w", err)
	}
	if err := startDefaultInstallClient(ctx, h, name); err != nil {
		return err
	}
	if err := clientCreatesShellSession(ctx, name, sessionName); err != nil {
		return err
	}
	return clientExitsCleanly(ctx, name)
}

// defaultInstallRestarted is this scenario's "When": the real, literal
// success-criterion action -- plain `deck`, no positional argument, no
// DECK_PROFILE -- resumed against the SAME temp home defaultInstallSeeded
// already populated.
func defaultInstallRestarted(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	return startDefaultInstallClient(ctx, h, name)
}

// defaultInstallHeaderReadsExactly is the "header reads exactly" half of
// this task's own success criteria: a bare "screen contains" cannot tell
// "socket: deck" apart from some wider line that merely contains it (for
// example a regression that grew a "profile: default · " prefix in front,
// which SPEC never wants for the default profile). This instead finds the
// one sidebar row whose FULL trimmed cell text is want, byte for byte,
// splitting on the ASCII vertical divider "|" (box().vertical,
// internal/tui/panel.go) that the flat config.toml's [ui] ascii = true
// turns on for these clients (DECK_ASCII is cleared for them).
func defaultInstallHeaderReadsExactly(ctx context.Context, name, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	for _, line := range strings.Split(frame, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		if text := strings.TrimSpace(cells[1]); text == want {
			return nil
		}
	}
	return fmt.Errorf("deck client %q: no sidebar row reads exactly %q:\n%s", name, want, frame)
}

// openDefaultInstallDatabase opens the scenario's own default install
// state.db directly (openObservedDatabase's own busy-timeout policy,
// pointed at the flat XDG data root instead of DECK_HOME=h.Home).
func openDefaultInstallDatabase(h *ScenarioHarness) (*sql.DB, error) {
	db, err := sql.Open("sqlite", filepath.Join(defaultInstallDataRoot(h.defaultInstallHome), "state.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure default install state database: %w", err)
	}
	return db, nil
}

// defaultInstallSessionSlug reads session name's own slug from the default
// install's flat state.db, the column deck's own tmux target naming
// ("deck_"+slug) is built from.
func defaultInstallSessionSlug(ctx context.Context, h *ScenarioHarness, name string) (string, error) {
	db, err := openDefaultInstallDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var slug string
	if err := db.QueryRowContext(ctx, `SELECT slug FROM sessions WHERE name = ?`, name).Scan(&slug); err != nil {
		return "", fmt.Errorf("read session %q slug from default install state database: %w", name, err)
	}
	return slug, nil
}

// defaultInstallSessionIsLiveOnSocketDeck asserts name is a real, live tmux
// session on the LITERAL socket "deck" -- this task's own "the session
// lives on socket deck" criterion, queried directly rather than merely
// trusting whatever socket the scenario happened to compute.
func defaultInstallSessionIsLiveOnSocketDeck(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := defaultInstallSessionSlug(ctx, h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", "deck", "has-session", "-t", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux -L deck has-session -t %s: %w: %s", target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// defaultInstallConfigStillContainsMarker asserts the flat config.toml
// defaultInstallSeeded pre-wrote is still on disk, at the exact flat path a
// real install uses, and still carries the marker -- it was never moved or
// replaced by some profiles/-nested file. It is only the "kept" half: the
// "actually loaded" half is defaultInstallClientDrawsASCIIChrome and
// defaultInstallPaneEnvironmentHasConfigValue, which observe settings deck
// could only have read from this file.
func defaultInstallConfigStillContainsMarker(ctx context.Context, marker string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(defaultInstallConfigRoot(h.defaultInstallHome), "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read default install config.toml at its flat path %q: %w", path, err)
	}
	if !strings.Contains(string(data), marker) {
		return fmt.Errorf("default install config.toml at %q does not contain marker %q:\n%s", path, marker, data)
	}
	return nil
}

// defaultInstallEnvKey is the [env] key defaultInstallSeeded writes into
// the flat config.toml; nothing else in the harness or the sibling
// container's environment defines it.
const defaultInstallEnvKey = "DECK_DEFAULT_INSTALL_MARKER"

// defaultInstallClientDrawsASCIIChrome proves the running client LOADED
// the flat config.toml rather than merely leaving it on disk: these
// clients run with DECK_ASCII cleared, deck's own default is Unicode
// box-drawing (internal/tui/panel.go box()), so the ASCII "+"/"|" chrome
// appears only if [ui] ascii = true was read from $HOME/.config/deck's
// config.toml. The frame must hold the ASCII corner and divider and none
// of the Unicode border glyphs.
func defaultInstallClientDrawsASCIIChrome(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "─", "│"} {
		if strings.Contains(frame, glyph) {
			return fmt.Errorf("deck client %q draws Unicode border glyph %q, so [ui] ascii = true from the flat config.toml was not applied:\n%s", name, glyph, frame)
		}
	}
	if !strings.Contains(frame, "+") || !strings.Contains(frame, "|") {
		return fmt.Errorf("deck client %q frame has no ASCII \"+\"/\"|\" chrome from the flat config.toml's [ui] ascii = true:\n%s", name, frame)
	}
	return nil
}

// defaultInstallPaneEnvironmentHasConfigValue reads the live pane
// process's own /proc/<pid>/environ for session name on the literal socket
// "deck" and requires key=want: a value only the flat config.toml's [env]
// table supplies, so a pane created by the restarted client carrying it
// proves that client resolved and loaded $HOME/.config/deck/config.toml.
func defaultInstallPaneEnvironmentHasConfigValue(ctx context.Context, name, key, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if value, ok := os.LookupEnv(key); ok {
		return fmt.Errorf("test process already defines %s=%q, so the pane's value could not prove the flat config.toml supplied it", key, value)
	}
	slug, err := defaultInstallSessionSlug(ctx, h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", "deck", "list-panes", "-t", target, "-F", "#{pane_pid}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux -L deck list-panes -t %s: %w: %s", target, err, strings.TrimSpace(string(output)))
	}
	pidText := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return fmt.Errorf("parse pane_pid %q for default install session %q: %w", pidText, name, err)
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return fmt.Errorf("read /proc/%d/environ for default install session %q: %w", pid, name, err)
	}
	for _, entry := range strings.Split(string(raw), "\x00") {
		if k, v, ok := strings.Cut(entry, "="); ok && k == key {
			if v != want {
				return fmt.Errorf("default install session %q pane has %s=%q, want %q from the flat config.toml's [env]", name, key, v, want)
			}
			return nil
		}
	}
	return fmt.Errorf("default install session %q pane environment has no %s at all, so the flat config.toml's [env] table was not loaded", name, key)
}

// defaultInstallStateDatabaseHoldsExactlySessions asserts the flat state.db
// holds exactly two rows, named a and b -- the seeded "prior" row surviving
// the restart AND the newly created "second" row landing in the SAME flat
// database, never a fresh one somewhere else.
func defaultInstallStateDatabaseHoldsExactlySessions(ctx context.Context, a, b string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openDefaultInstallDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		return fmt.Errorf("count default install sessions: %w", err)
	}
	if count != 2 {
		return fmt.Errorf("default install flat state database has %d session rows, want exactly 2", count)
	}
	for _, name := range []string{a, b} {
		if _, err := defaultInstallSessionSlug(ctx, h, name); err != nil {
			return fmt.Errorf("default install flat state database missing session %q: %w", name, err)
		}
	}
	return nil
}

// defaultInstallLogDirectoryHoldsJSONLFile asserts the flat log directory
// (SPEC §2: $XDG_STATE_HOME/deck/log, never DECK_HOME's own shortcut) holds
// a non-empty deck.jsonl -- the flat log path is the one actually written
// to, not merely resolved and left untouched.
func defaultInstallLogDirectoryHoldsJSONLFile(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(defaultInstallStateRoot(h.defaultInstallHome), "log", "deck.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat default install flat log file %q: %w", path, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("default install flat log file %q is empty, want at least one audit record", path)
	}
	return nil
}

// defaultInstallHasNoProfilesDirectories is this task's own "no profiles/
// directory exists" criterion, checked under EACH of the three temp roots
// this scenario resolved separately (config, data, state) -- never just
// one shared root, unlike every DECK_HOME-based scenario elsewhere in this
// package where the three would collapse into one path anyway.
func defaultInstallHasNoProfilesDirectories(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	roots := map[string]string{
		"config": defaultInstallConfigRoot(h.defaultInstallHome),
		"data":   defaultInstallDataRoot(h.defaultInstallHome),
		"state":  defaultInstallStateRoot(h.defaultInstallHome),
	}
	for kind, root := range roots {
		path := filepath.Join(root, "profiles")
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("a profiles/ directory exists under the default install's temp %s root: %s", kind, path)
			}
			return fmt.Errorf("stat default install temp %s root's profiles/ directory: %w", kind, err)
		}
	}
	return nil
}
