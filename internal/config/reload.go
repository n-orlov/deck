package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultConfigPollMS is the production cadence of the running TUI's
// config reload poll (SPEC §6.5): 30 s, the upper bound R226 allows. A
// shorter interval is reachable only through DECK_CONFIG_POLL_MS, a
// test-only override (SPEC §13.1) production never sets.
const DefaultConfigPollMS = 30000

// Fingerprint is the cheap, comparable summary of everything a reload
// would read: config.toml's mtime and size and the user theme directory's
// entry names, mtimes and sizes (SPEC §6.5). Two equal fingerprints mean
// nothing a reload reads has changed, so nothing is re-parsed.
type Fingerprint string

// ComputeFingerprint stats configFile and lists themesDir. It only ever
// stats and lists -- it never opens a file for writing, creates, or touches
// anything -- and a missing file or directory is a fingerprint value of its
// own ("absent"), not an error, so creating or deleting either is a change.
func ComputeFingerprint(configFile, themesDir string) Fingerprint {
	var b strings.Builder
	b.WriteString("config:")
	b.WriteString(statSummary(configFile))
	b.WriteString("\nthemes:")
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		b.WriteString("absent")
		return Fingerprint(b.String())
	}
	for _, entry := range entries { // os.ReadDir returns entries sorted by name
		fmt.Fprintf(&b, "\n%s:%s", entry.Name(), statSummary(filepath.Join(themesDir, entry.Name())))
	}
	return Fingerprint(b.String())
}

// statSummary is "mtime-ns:size" of path (following symlinks, as the theme
// loader does), or "absent" when it cannot be stat'ed.
func statSummary(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

// Reloader remembers the last-seen Fingerprint and re-runs load only when
// it moved. It is read-only by construction: it holds no writer, and load
// is a Load*-shaped function (SPEC §6.5: the reload never writes).
type Reloader struct {
	mu         sync.Mutex // guards last: Poll runs off the update loop, MarkOwnWrite on it
	configFile string
	themesDir  string
	last       Fingerprint
	load       func() (Settings, error)
}

// NewReloader starts watching the files settings was loaded from. The
// current fingerprint is taken as the last-seen value, so the first Poll
// after a quiet start re-parses nothing. load re-reads the configuration.
func NewReloader(settings Settings, load func() (Settings, error)) *Reloader {
	r := &Reloader{configFile: settings.Paths.ConfigFile, themesDir: settings.ThemesDir, load: load}
	r.last = ComputeFingerprint(r.configFile, r.themesDir)
	return r
}

// Poll compares the current fingerprint with the last-seen one. Unchanged:
// it returns changed=false without calling load. Changed: it records the
// new fingerprint first (so an invalid file is parsed once, not on every
// poll, and the next write is a new fingerprint again), then calls load
// exactly once and returns its result. The fingerprint is taken before the
// parse, so a write landing mid-parse is simply seen by the next Poll.
func (r *Reloader) Poll() (settings Settings, changed bool, err error) {
	now := ComputeFingerprint(r.configFile, r.themesDir)
	r.mu.Lock()
	if now == r.last {
		r.mu.Unlock()
		return Settings{}, false, nil
	}
	r.last = now
	r.mu.Unlock()
	settings, err = r.load()
	return settings, true, err
}

// MarkOwnWrite records the config.toml this process has just written as the
// last-seen one, so the next Poll does not report its own save back as an
// external change (SPEC §6.5: an instance's own save triggers no second
// apply). Only the config.toml part of the fingerprint moves: a user theme
// file that changed in the meantime is still a change the next Poll reports.
func (r *Reloader) MarkOwnWrite() {
	own := ComputeFingerprint(r.configFile, r.themesDir)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = Fingerprint(configPart(string(own)) + themesPart(string(r.last)))
}

const fingerprintThemesMarker = "\nthemes:"

func configPart(fp string) string {
	if i := strings.Index(fp, fingerprintThemesMarker); i >= 0 {
		return fp[:i]
	}
	return fp
}

func themesPart(fp string) string {
	if i := strings.Index(fp, fingerprintThemesMarker); i >= 0 {
		return fp[i:]
	}
	return ""
}
