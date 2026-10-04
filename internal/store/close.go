package store

import (
	"database/sql"
	"errors"
)

// rollbackTx abandons tx on every return path of the function that began it.
// After a successful Commit the rollback reports sql.ErrTxDone, the expected
// outcome. Any other rollback failure means the connection is already broken,
// and database/sql then discards that connection; the error the caller is
// returning is the one worth reporting, so the rollback's own is dropped here.
func rollbackTx(tx *sql.Tx) { _ = tx.Rollback() }

// closeRows releases a result set once the caller is done with it. Every read
// loop checks rows.Err() after iterating, which is where a failed read
// surfaces; a Close error afterwards carries nothing the caller can act on.
func closeRows(rows *sql.Rows) { _ = rows.Close() }

// closeAfter closes db on an error path of Open and returns err, joined with
// the close failure when there is one, so neither cause is lost. err is
// returned unchanged when the close succeeds.
func closeAfter(db *sql.DB, err error) error {
	if closeErr := db.Close(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return err
}
