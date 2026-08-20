// Package config resolves deck's documented runtime controls.
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultSocket      = "deck"
	DefaultReconcileMS = 500
	DefaultPreviewMS   = 250
	DefaultStaleAfter  = 45 * time.Second
)

// Paths are the locations used by deck at runtime. DECK_HOME deliberately
// supplies a single root for all mutable state, making isolated runs simple.
type Paths struct {
	Home       string
	DataDir    string
	ConfigFile string
	LogDir     string
	StateDB    string
}

// Settings contains the supported determinism and polling controls.
type Settings struct {
	Paths     Paths
	Socket    string
	Clock     *Clock
	IDs       *IDGenerator
	Reconcile time.Duration
	Preview   time.Duration
	// StaleAfter is the wall-clock age after which an agent verdict may be
	// sampled from its pane. It is loaded from config.toml, not an elapsed
	// process timer, so frozen-clock scenarios can advance it on demand.
	StaleAfter time.Duration
	ASCII      bool
	Animation  bool
	Color      bool
	// AllowYolo mirrors config.toml's top-level allow_yolo key. It defaults to
	// false when the file, or the key within it, is absent: the yolo
	// permission profile stays gated unless an operator opts in explicitly.
	AllowYolo bool
	// Env mirrors config.toml's [env] table: additional environment variables
	// layered under the session env per SPEC §6.1/§6.3. Absent file or absent
	// section both yield a nil map, never an error.
	Env map[string]string
}

// Load reads only documented DECK_ controls from env.
func Load() (Settings, error) { return LoadFrom(os.Getenv, os.UserHomeDir) }

// LoadFrom exists to make environment resolution independently testable.
func LoadFrom(getenv func(string) string, userHome func() (string, error)) (Settings, error) {
	paths, err := resolvePaths(getenv, userHome)
	if err != nil {
		return Settings{}, err
	}
	clock, err := NewClock(getenv("DECK_CLOCK"), getenv("DECK_CLOCK_STEP"))
	if err != nil {
		return Settings{}, err
	}
	// A frozen clock is shared under the resolved data root even when DECK_HOME
	// is unset, so every process using the same normal installation agrees.
	clock.sharedPath = filepath.Join(paths.Home, "clock.now")
	reconcile, err := milliseconds(getenv("DECK_RECONCILE_MS"), DefaultReconcileMS, "DECK_RECONCILE_MS")
	if err != nil {
		return Settings{}, err
	}
	preview, err := milliseconds(getenv("DECK_PREVIEW_MS"), DefaultPreviewMS, "DECK_PREVIEW_MS")
	if err != nil {
		return Settings{}, err
	}
	socket := getenv("DECK_TMUX_SOCKET")
	if socket == "" {
		socket = DefaultSocket
	}
	if strings.ContainsAny(socket, "/\x00") {
		return Settings{}, fmt.Errorf("DECK_TMUX_SOCKET must be a tmux socket name, not a path")
	}
	ascii, err := boolEnv(getenv("DECK_ASCII"), false, "DECK_ASCII")
	if err != nil {
		return Settings{}, err
	}
	animation, err := boolEnv(getenv("DECK_ANIM"), true, "DECK_ANIM")
	if err != nil {
		return Settings{}, err
	}
	color := getenv("NO_COLOR") == ""
	if raw := getenv("DECK_COLOR"); raw != "" {
		color, err = boolEnv(raw, true, "DECK_COLOR")
		if err != nil {
			return Settings{}, err
		}
	}
	allowYolo, staleAfter, env, err := loadConfigFile(paths.ConfigFile)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Paths: paths, Socket: socket, Clock: clock, IDs: NewIDGenerator(getenv("DECK_ID_SEED")),
		Reconcile: reconcile, Preview: preview, StaleAfter: staleAfter,
		ASCII: ascii, Animation: animation, Color: color, AllowYolo: allowYolo, Env: env,
	}, nil
}

func resolvePaths(getenv func(string) string, userHome func() (string, error)) (Paths, error) {
	if root := getenv("DECK_HOME"); root != "" {
		return Paths{Home: root, DataDir: root, ConfigFile: filepath.Join(root, "config.toml"), LogDir: filepath.Join(root, "log"), StateDB: filepath.Join(root, "state.db")}, nil
	}
	home, err := userHome()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	data := getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	config := getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	state := getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	data = filepath.Join(data, "deck")
	return Paths{Home: data, DataDir: data, ConfigFile: filepath.Join(config, "deck", "config.toml"), LogDir: filepath.Join(state, "deck", "log"), StateDB: filepath.Join(data, "state.db")}, nil
}

func milliseconds(raw string, fallback int, name string) (time.Duration, error) {
	if raw == "" {
		return time.Duration(fallback) * time.Millisecond, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer in milliseconds", name)
	}
	return time.Duration(value) * time.Millisecond, nil
}

func boolEnv(raw string, fallback bool, name string) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", name, err)
	}
	return value, nil
}

// Clock freezes wall time when DECK_CLOCK is set. Elapsed intentionally uses
// time.Since, whose monotonic component is unaffected by this frozen wall clock.
type Clock struct {
	frozen     bool
	base       time.Time
	step       time.Duration
	start      time.Time
	sharedPath string
	mu         sync.Mutex
	ticks      uint64
}

func NewClock(wall, step string) (*Clock, error) {
	clock := &Clock{start: time.Now()}
	if step != "" {
		var err error
		clock.step, err = time.ParseDuration(step)
		if err != nil || clock.step <= 0 {
			return nil, errors.New("DECK_CLOCK_STEP must be a positive Go duration")
		}
	}
	if wall == "" {
		return clock, nil
	}
	parsed, err := time.Parse(time.RFC3339, wall)
	if err != nil {
		return nil, fmt.Errorf("DECK_CLOCK must be RFC3339: %w", err)
	}
	clock.frozen, clock.base = true, parsed
	return clock, nil
}

func (c *Clock) Now() time.Time {
	if !c.frozen {
		return time.Now()
	}
	if c.sharedPath != "" {
		if raw, err := os.ReadFile(c.sharedPath); err == nil {
			if shared, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(raw))); err == nil {
				return shared
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base.Add(time.Duration(c.ticks) * c.step)
}

// StepEnabled reports whether an on-demand step has both a frozen base and a
// positive increment. A running deck client exposes that operation through
// SIGUSR1; clocks without both DECK_CLOCK and DECK_CLOCK_STEP do not claim the
// signal or alter normal process signal handling.
func (c *Clock) StepEnabled() bool { return c.frozen && c.step > 0 }

// Advance moves a frozen clock one configured step and returns its new wall time.
// Clocks loaded through Load persist the resulting absolute instant under the
// resolved data root, so already-running clients and later subprocesses agree.
func (c *Clock) Advance() time.Time {
	value, _ := c.AdvanceShared()
	return value
}

// AdvanceShared is the error-reporting form for clock-control tooling and tests.
func (c *Clock) AdvanceShared() (time.Time, error) {
	if !c.frozen || c.step == 0 {
		return c.Now(), nil
	}
	if c.sharedPath == "" {
		c.mu.Lock()
		c.ticks++
		value := c.base.Add(time.Duration(c.ticks) * c.step)
		c.mu.Unlock()
		return value, nil
	}
	if err := os.MkdirAll(filepath.Dir(c.sharedPath), 0o700); err != nil {
		return c.Now(), fmt.Errorf("create shared clock directory: %w", err)
	}
	lock, err := os.OpenFile(c.sharedPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return c.Now(), fmt.Errorf("open shared clock lock: %w", err)
	}
	defer lock.Close()
	if err := lockFile(lock); err != nil {
		return c.Now(), err
	}
	defer unlockFile(lock)
	current := c.Now()
	value := current.Add(c.step)
	temporary := c.sharedPath + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(temporary, []byte(value.Format(time.RFC3339Nano)+"\n"), 0o600); err != nil {
		return current, fmt.Errorf("write shared clock: %w", err)
	}
	if err := os.Rename(temporary, c.sharedPath); err != nil {
		_ = os.Remove(temporary)
		return current, fmt.Errorf("publish shared clock: %w", err)
	}
	return value, nil
}

func lockFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock shared clock: %w", err)
	}
	return nil
}

func unlockFile(file *os.File) { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }

// Elapsed is always an actual monotonic elapsed duration, including with DECK_CLOCK.
func (c *Clock) Elapsed() time.Duration { return time.Since(c.start) }

// IDGenerator produces RFC 4122 version-4-shaped UUIDs. A seed trades random
// entropy for repeatability, solely for deterministic rendering and test runs.
type IDGenerator struct {
	seed    string
	reader  io.Reader
	mu      sync.Mutex
	counter uint64
}

func NewIDGenerator(seed string) *IDGenerator {
	return &IDGenerator{seed: seed, reader: rand.Reader}
}

func (g *IDGenerator) UUID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	bytes := make([]byte, 16)
	if g.seed == "" {
		if _, err := io.ReadFull(g.reader, bytes); err != nil {
			return "", fmt.Errorf("generate UUID: %w", err)
		}
	} else {
		g.counter++
		digest := sha256.Sum256([]byte(g.seed + "\x00" + strconv.FormatUint(g.counter, 10)))
		copy(bytes, digest[:16])
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}
