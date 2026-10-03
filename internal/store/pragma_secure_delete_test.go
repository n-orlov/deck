package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	sqlite "modernc.org/sqlite"
)

// TestSecureDeleteIsOnAndJournalSizeLimitIsNotForced: R197/#61's "env/data at
// rest" item. secure_delete=ON must be read back as on, and journal_size_limit
// must not have been forced to 0 -- that would reset (truncate+recreate) the
// WAL on every checkpoint, bringing back the fresh-WAL header fsync persistWAL
// exists to avoid (SPEC section 3.1's 20ms hook write budget).
func TestSecureDeleteIsOnAndJournalSizeLimitIsNotForced(t *testing.T) {
	home := t.TempDir()
	s, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var secureDelete int
	if err := s.DB().QueryRow(`PRAGMA secure_delete`).Scan(&secureDelete); err != nil {
		t.Fatal(err)
	}
	// SQLite reports secure_delete as 1 (on) or 2 (fast, the WAL-mode
	// default elision) depending on build options; either value means the
	// pragma was accepted and is not off (0).
	if secureDelete == 0 {
		t.Fatalf("secure_delete = %d, want ON (nonzero)", secureDelete)
	}

	var journalSizeLimit int64
	if err := s.DB().QueryRow(`PRAGMA journal_size_limit`).Scan(&journalSizeLimit); err != nil {
		t.Fatal(err)
	}
	if journalSizeLimit == 0 {
		t.Fatalf("journal_size_limit = %d, want anything but 0 (0 forces a WAL reset on every checkpoint, reintroducing the fresh-WAL header fsync persistWAL exists to avoid)", journalSizeLimit)
	}
}

// firstConnModes records, for every path OpenPath/OpenPath's underlying
// driver has ever connected to in this test binary, the file mode observed
// the first time a connection to that path opens -- i.e. the mode the file
// had the instant SQLite created it, before any explicit chmod OpenPath
// might run afterward. It is a package-wide sqlite.RegisterConnectionHook,
// so it is registered once here and fires for every connection, cheaply,
// for the life of the test binary.
var (
	firstConnModesMu sync.Mutex
	firstConnModes   = map[string]os.FileMode{}
)

func init() {
	sqlite.RegisterConnectionHook(func(_ sqlite.ExecQuerierContext, dsn string) error {
		p := dsn
		if i := strings.IndexByte(p, '?'); i >= 0 {
			p = p[:i]
		}
		firstConnModesMu.Lock()
		defer firstConnModesMu.Unlock()
		if _, seen := firstConnModes[p]; seen {
			return nil
		}
		info, err := os.Stat(p)
		if err != nil {
			// The file may not exist yet at the instant this fires for some
			// driver-internal probe connection; only record a mode we
			// actually observed.
			return nil
		}
		firstConnModes[p] = info.Mode().Perm()
		return nil
	})
}

// TestANewStateDBIsCreated0600FromTheFirstConnectionEvenUnderAPermissiveUmask:
// R197/#61's "umask set before open, not a chmod after" requirement. Under a
// permissive ambient umask, the pre-task code creates state.db at the
// driver's default create mode and only tightens it to 0600 with an explicit
// chmod afterward -- a window during which a brand-new state.db, holding
// session env values, is on disk looser than 0600. OpenPath must instead
// narrow the umask before the first connection opens, so the file is already
// 0600 the instant SQLite creates it.
func TestANewStateDBIsCreated0600FromTheFirstConnectionEvenUnderAPermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0o000)
	defer syscall.Umask(oldUmask)

	home := t.TempDir()
	path := filepath.Join(home, "state.db")

	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Force the WAL siblings to exist so the hook has observed them too.
	if _, err := s.CreateSession(context.Background(), CreateSessionInput{
		ID: "umask", Name: "umask", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	firstConnModesMu.Lock()
	mode, seen := firstConnModes[path]
	firstConnModesMu.Unlock()
	if !seen {
		t.Fatal("the connection hook never observed state.db's path; cannot assert its mode at creation")
	}
	if mode != 0o600 {
		t.Fatalf("state.db mode at the first connection = %#o, want 0600 (it was created loose under this permissive umask and only chmod-ed afterward)", mode)
	}
}
