package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	sqlite "modernc.org/sqlite"
)

// busyBoundMargin is how far past the context's deadline a bounded
// transaction's SQLite busy wait may run. SQLite counts in whole milliseconds
// and does not look at the context while it waits, so the wait is set to the
// time left plus this margin: it can never end before the deadline (the caller
// then reads a lock that ran out the bound, not a failure of its own), and
// overshoots it by no more than this.
const busyBoundMargin = 20 * time.Millisecond

// beginWithin starts a write transaction whose wait for SQLite's write lock
// ends with ctx's deadline. The connection's busy_timeout (busyTimeoutMillis)
// does not watch the context, so a caller bounded tighter than that, such as
// the hook's liveness pass (R416), would otherwise wait out the full
// busy_timeout behind another writer and overrun its bound. The shortened wait
// is set on the pool's one connection, held exclusively until the returned
// done runs, which restores busy_timeout and releases the connection; the
// caller runs done after the transaction has committed or rolled back. A
// context with no deadline, or one at least busy_timeout away, takes the
// ordinary path.
func (s *Store) beginWithin(ctx context.Context) (*sql.Tx, func(), error) {
	deadline, bounded := ctx.Deadline()
	remaining := time.Until(deadline)
	if !bounded || remaining <= 0 || remaining+busyBoundMargin >= busyTimeoutMillis*time.Millisecond {
		tx, err := s.db.BeginTx(ctx, nil)
		return tx, func() {}, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	done := func() {
		_, _ = conn.ExecContext(context.Background(), "PRAGMA busy_timeout="+strconv.Itoa(busyTimeoutMillis))
		_ = conn.Close()
	}
	tx, err := beginBeforeDeadline(ctx, conn, deadline)
	if err != nil {
		done()
		return nil, nil, err
	}
	return tx, done, nil
}

// beginBeforeDeadline begins on conn, waiting for the write lock until
// deadline. SQLite's busy wait can end a few milliseconds before the timeout
// it was given, so a busy answer that arrives before the deadline is retried
// with what is left, and the caller never reads a lock that ran out the bound
// before the bound has passed.
func beginBeforeDeadline(ctx context.Context, conn *sql.Conn, deadline time.Time) (*sql.Tx, error) {
	for {
		wait := (time.Until(deadline) + busyBoundMargin).Milliseconds() + 1
		if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout="+strconv.FormatInt(wait, 10)); err != nil {
			return nil, fmt.Errorf("bound busy wait: %w", err)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err == nil || !isBusy(err) || !time.Now().Before(deadline) {
			return tx, err
		}
	}
}

// isBusy reports whether err is SQLite's "database is locked" answer, in any
// of its extended codes.
func isBusy(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3Busy
}

// sqlite3Busy is SQLITE_BUSY, the primary result code of a lock wait that ran
// out busy_timeout.
const sqlite3Busy = 5
