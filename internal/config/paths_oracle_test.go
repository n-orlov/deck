package config

import (
	"strings"
	"testing"
)

// realEnvLeakSentinel is written into every environment variable that could
// legitimately drive path resolution (HOME plus the three XDG_* roots plus
// DECK_HOME/DECK_TMUX_SOCKET) on the *real* process environment for the
// duration of each subtest below, via t.Setenv (auto-restored). LoadFrom is
// only ever handed a map-backed getenv and an injected userHome in this
// file, so nothing here should ever see this sentinel land in a resolved
// path -- if it does, resolution reached the process's real environment (or
// os.UserHomeDir, which itself just reads real HOME on this platform)
// instead of the two functions LoadFrom took as parameters, and that is
// exactly the regression this file exists to catch before any change to
// internal/config's path/socket resolution.
const realEnvLeakSentinel = "REALENV-LEAK-MUST-NOT-BE-READ"

// poisonRealEnv overwrites every environment variable path/socket
// resolution could read directly, for the life of the calling (sub)test.
// t.Setenv restores the previous value automatically on test cleanup.
func poisonRealEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HOME", "XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "DECK_HOME", "DECK_TMUX_SOCKET"} {
		t.Setenv(key, realEnvLeakSentinel+"-"+key)
	}
}

// failIfCalled returns a userHome function that fails the test the instant
// it is invoked. Used for the DECK_HOME-set case below, where resolvePaths
// must short-circuit before ever consulting userHome (equivalently,
// os.UserHomeDir in the real Load path).
func failIfCalled(t *testing.T) func() (string, error) {
	t.Helper()
	return func() (string, error) {
		t.Fatal("resolvePaths called userHome (== os.UserHomeDir in the real Load path) even though DECK_HOME was set; it must short-circuit before that call")
		return "", nil
	}
}

// assertNoRealEnvLeak fails the test if any resolved string contains the
// sentinel poisonRealEnv wrote into the real environment -- the explicit,
// named guard the task requires: resolution must never call
// os.UserHomeDir or read the process's real HOME/XDG_*/DECK_* variables,
// only the getenv/userHome functions LoadFrom was actually handed.
func assertNoRealEnvLeak(t *testing.T, label string, values ...string) {
	t.Helper()
	for _, v := range values {
		if strings.Contains(v, realEnvLeakSentinel) {
			t.Fatalf("%s: resolved value %q leaked the poisoned real environment -- path/socket resolution must use only the injected getenv/userHome, never os.UserHomeDir or the process's real HOME", label, v)
		}
	}
}

// TestPathsOracle pins today's LoadFrom-resolved Paths (Home, DataDir,
// ConfigFile, LogDir, StateDB), Socket, and frozen-clock shared path as a
// literal-string oracle table, before any change to R152's target
// (internal/config's default path/socket resolution). Every case injects
// its own userHome and getenv (never the process's real environment, which
// poisonRealEnv actively sabotages for the duration of each case) so a
// resolution bug that starts reading real HOME/XDG/DECK_* instead of the
// injected functions is caught even when this sandbox's real environment
// happens to agree with the literals below.
func TestPathsOracle(t *testing.T) {
	const injectedHome = "/oracle-home"

	type oracleCase struct {
		name       string
		env        map[string]string
		userHome   func(t *testing.T) func() (string, error)
		wantHome   string
		wantData   string
		wantConfig string
		wantLog    string
		wantStateD string
		wantSocket string
		wantShared string
	}

	fixedHome := func(t *testing.T) func() (string, error) {
		return func() (string, error) { return injectedHome, nil }
	}

	cases := []oracleCase{
		{
			name:       "xdg-defaults-from-injected-home",
			env:        map[string]string{},
			userHome:   fixedHome,
			wantHome:   "/oracle-home/.local/share/deck",
			wantData:   "/oracle-home/.local/share/deck",
			wantConfig: "/oracle-home/.config/deck/config.toml",
			wantLog:    "/oracle-home/.local/state/deck/log",
			wantStateD: "/oracle-home/.local/share/deck/state.db",
			wantSocket: "deck",
			wantShared: "/oracle-home/.local/share/deck/clock.now",
		},
		{
			name:       "xdg-data-home-set",
			env:        map[string]string{"XDG_DATA_HOME": "/oracle-xdg-data"},
			userHome:   fixedHome,
			wantHome:   "/oracle-xdg-data/deck",
			wantData:   "/oracle-xdg-data/deck",
			wantConfig: "/oracle-home/.config/deck/config.toml",
			wantLog:    "/oracle-home/.local/state/deck/log",
			wantStateD: "/oracle-xdg-data/deck/state.db",
			wantSocket: "deck",
			wantShared: "/oracle-xdg-data/deck/clock.now",
		},
		{
			name:       "xdg-config-home-set",
			env:        map[string]string{"XDG_CONFIG_HOME": "/oracle-xdg-config"},
			userHome:   fixedHome,
			wantHome:   "/oracle-home/.local/share/deck",
			wantData:   "/oracle-home/.local/share/deck",
			wantConfig: "/oracle-xdg-config/deck/config.toml",
			wantLog:    "/oracle-home/.local/state/deck/log",
			wantStateD: "/oracle-home/.local/share/deck/state.db",
			wantSocket: "deck",
			wantShared: "/oracle-home/.local/share/deck/clock.now",
		},
		{
			name:       "xdg-state-home-set",
			env:        map[string]string{"XDG_STATE_HOME": "/oracle-xdg-state"},
			userHome:   fixedHome,
			wantHome:   "/oracle-home/.local/share/deck",
			wantData:   "/oracle-home/.local/share/deck",
			wantConfig: "/oracle-home/.config/deck/config.toml",
			wantLog:    "/oracle-xdg-state/deck/log",
			wantStateD: "/oracle-home/.local/share/deck/state.db",
			wantSocket: "deck",
			wantShared: "/oracle-home/.local/share/deck/clock.now",
		},
		{
			name:       "deck-home-set",
			env:        map[string]string{"DECK_HOME": "/oracle-deck-home"},
			userHome:   failIfCalled,
			wantHome:   "/oracle-deck-home",
			wantData:   "/oracle-deck-home",
			wantConfig: "/oracle-deck-home/config.toml",
			wantLog:    "/oracle-deck-home/log",
			wantStateD: "/oracle-deck-home/state.db",
			wantSocket: "deck",
			wantShared: "/oracle-deck-home/clock.now",
		},
		{
			name:       "deck-tmux-socket-set",
			env:        map[string]string{"DECK_TMUX_SOCKET": "oracle-socket"},
			userHome:   fixedHome,
			wantHome:   "/oracle-home/.local/share/deck",
			wantData:   "/oracle-home/.local/share/deck",
			wantConfig: "/oracle-home/.config/deck/config.toml",
			wantLog:    "/oracle-home/.local/state/deck/log",
			wantStateD: "/oracle-home/.local/share/deck/state.db",
			wantSocket: "oracle-socket",
			wantShared: "/oracle-home/.local/share/deck/clock.now",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			poisonRealEnv(t)

			settings, err := LoadFrom(environment(tc.env), tc.userHome(t))
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}

			assertNoRealEnvLeak(t, tc.name,
				settings.Paths.Home, settings.Paths.DataDir, settings.Paths.ConfigFile,
				settings.Paths.LogDir, settings.Paths.StateDB, settings.Socket,
				settings.Clock.sharedPath)

			if settings.Paths.Home != tc.wantHome {
				t.Errorf("Home = %q, want %q", settings.Paths.Home, tc.wantHome)
			}
			if settings.Paths.DataDir != tc.wantData {
				t.Errorf("DataDir = %q, want %q", settings.Paths.DataDir, tc.wantData)
			}
			if settings.Paths.ConfigFile != tc.wantConfig {
				t.Errorf("ConfigFile = %q, want %q", settings.Paths.ConfigFile, tc.wantConfig)
			}
			if settings.Paths.LogDir != tc.wantLog {
				t.Errorf("LogDir = %q, want %q", settings.Paths.LogDir, tc.wantLog)
			}
			if settings.Paths.StateDB != tc.wantStateD {
				t.Errorf("StateDB = %q, want %q", settings.Paths.StateDB, tc.wantStateD)
			}
			if settings.Socket != tc.wantSocket {
				t.Errorf("Socket = %q, want %q", settings.Socket, tc.wantSocket)
			}
			if settings.Clock.sharedPath != tc.wantShared {
				t.Errorf("Clock.sharedPath = %q, want %q", settings.Clock.sharedPath, tc.wantShared)
			}
		})
	}
}
