package features

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerBookmarksClearSteps backs features/bookmarks_clear.feature (R288):
// the Alt+M then Delete clear action, and archive clearing the slots a
// session holds. Alt+M is ESC m on the wire and Delete is CSI 3 ~, the bytes
// a real terminal sends.
func registerBookmarksClearSteps(sc *godog.ScenarioContext) {
	sc.Step(`^every deck client in this scenario has the bookmarks setting on$`, bookmarksSettingIsOn)
	sc.Step(`^deck client "([^"]+)" bookmarks its selected session in slot ([0-9])$`, clientBookmarksSelectedSessionInSlot)
	sc.Step(`^deck client "([^"]+)" presses Alt\+M then Delete on its selected session, which holds slot ([0-9])$`, clientClearsSelectedSessionBookmark)
	sc.Step(`^the state database holds bookmark slot ([0-9]) for session "([^"]+)"$`, databaseHoldsBookmarkForSession)
	sc.Step(`^the state database holds no bookmark slot ([0-9])$`, databaseHoldsNoBookmark)
}

func bookmarksSettingIsOn(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	h.clientEnv = append(h.clientEnv, "DECK_BOOKMARKS=1")
	return nil
}

func clientBookmarksSelectedSessionInSlot(ctx context.Context, name string, slot int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x1bm"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WaitForFrame(wait, false, "Bookmark: press a digit 0-9"); err != nil {
		return err
	}
	return client.Send(fmt.Sprint(slot))
}

func clientClearsSelectedSessionBookmark(ctx context.Context, name string, slot int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\x1bm"); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WaitForFrame(wait, false, fmt.Sprintf("Press Delete to remove bookmark %d", slot)); err != nil {
		return err
	}
	if err := client.Send("\x1b[3~"); err != nil {
		return err
	}
	return client.WaitForFrame(wait, false, fmt.Sprintf("Removed bookmark %d", slot))
}

// bookmarkSessionName reads the name of the session slot points at; ok is
// false when the slot is empty.
func bookmarkSessionName(ctx context.Context, slot int) (name string, ok bool, err error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return "", false, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", false, err
	}
	defer db.Close()
	err = db.QueryRowContext(ctx, `SELECT s.name FROM bookmarks b JOIN sessions s ON s.id = b.session_id WHERE b.slot = ?`, slot).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("observe bookmark slot %d: %w", slot, err)
	}
	return name, true, nil
}

func databaseHoldsBookmarkForSession(ctx context.Context, slot int, session string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		name, ok, err := bookmarkSessionName(ctx, slot)
		if err != nil {
			return err
		}
		if ok && name == session {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bookmark slot %d holds %q (present=%v), want session %q", slot, name, ok, session)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func databaseHoldsNoBookmark(ctx context.Context, slot int) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		name, ok, err := bookmarkSessionName(ctx, slot)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bookmark slot %d still holds %q, want it empty", slot, name)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
