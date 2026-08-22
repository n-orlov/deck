package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateValidationSteps backs requirement 15: every rejection the
// create modal's own submitCreate/validateCreateFields (internal/tui/tui.go)
// can produce -- duplicate name, slug collision, non-existent cwd, a cwd
// that exists but is not a directory, a malformed env entry, malformed
// launch_args JSON -- names the specific problem in-modal and retains
// exactly what was typed rather than closing the modal or clearing a
// field; and esc abandons the modal, creating nothing, proven against the
// store's own row count rather than only the screen.
func registerCreateValidationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" attempts to create shell session "([^"]+)" with a fresh working directory labelled "([^"]+)", expecting rejection$`, clientAttemptsShellSessionExpectingRejection)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the create modal name field$`, clientTypesIntoCreateNameField)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the create modal env field$`, clientTypesIntoCreateEnvField)
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the create modal launch args field$`, clientTypesIntoCreateLaunchArgsField)
	sc.Step(`^deck client "([^"]+)" types a nonexistent path labelled "([^"]+)" into the create modal cwd field$`, clientTypesNonexistentPathIntoCreateCWDField)
	sc.Step(`^deck client "([^"]+)" types a file path labelled "([^"]+)" into the create modal cwd field$`, clientTypesFilePathIntoCreateCWDField)
	sc.Step(`^deck client "([^"]+)" submits the create modal expecting rejection$`, clientSubmitsCreateModalExpectingRejection)
	sc.Step(`^the state database has zero sessions$`, stateDatabaseHasZeroSessions)
	sc.Step(`^deck client "([^"]+)" screen contains, allowing word-wrap, "([^"]+)"$`, clientScreenContainsDewrapped)
}

// tabCreateModalNTimes sends n individual tab keystrokes with a short pause
// between each, matching the pacing every other repeated-keystroke step in
// this package already needs (features/assertions_test.go's own comment on
// this: a pty write burst of the same raw byte can coalesce/drop all but
// the first). This is what walks create-modal focus from the name field
// (0) to a later field without landing two tabs in the same write.
func tabCreateModalNTimes(client *ScreenDriver, n int) error {
	for i := 0; i < n; i++ {
		if err := client.Send("\t"); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// clientAttemptsShellSessionExpectingRejection is
// createShellSessionInLabelledCWD's rejected counterpart: it drives the
// exact same keystrokes (open the modal, type the name, tab to cwd, type
// the fresh directory, enter) but waits for the create modal's own
// rejection text rather than for a new row reaching "starting" -- the
// modal must still be open and named the specific problem, not have
// created a second row.
func clientAttemptsShellSessionExpectingRejection(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(sessionName); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + dir + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Cannot create session")
}

// clientTypesIntoCreateNameField types directly into the create modal's
// name field (field 0, where "n" leaves focus, per createFieldRows) --- no
// tabbing needed.
func clientTypesIntoCreateNameField(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send(text); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientTypesIntoCreateEnvField tabs from the name field (0) to the env
// field (5: name, cwd, agent, permission profile, launch_args, env --
// createFieldRows' own order) and types text there. The cwd field is
// deliberately left untouched at whatever it already holds (a real,
// existing directory), so the one intervening tab through it (task 012's
// bash-completion contract) finds nothing to complete or list and falls
// straight through to its ordinary field-advance meaning.
func clientTypesIntoCreateEnvField(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := tabCreateModalNTimes(client, 5); err != nil {
		return err
	}
	if err := client.Send(text); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientTypesIntoCreateLaunchArgsField is clientTypesIntoCreateEnvField's
// counterpart for the launch_args field (4).
func clientTypesIntoCreateLaunchArgsField(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := tabCreateModalNTimes(client, 4); err != nil {
		return err
	}
	if err := client.Send(text); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientTypesNonexistentPathIntoCreateCWDField registers label under a
// path that is never created on disk, then types it (via a single tab from
// the name field) into the cwd field -- validateCreateFields' os.Stat call
// is what turns this into the "does not exist" message.
func clientTypesNonexistentPathIntoCreateCWDField(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-validation-missing-"+label)
	registerNamedDirectory(h, label, dir)
	if err := client.Send("\t" + dir); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientTypesFilePathIntoCreateCWDField creates a plain file (not a
// directory) under label and types its path into the cwd field --
// validateCreateFields' info.IsDir() check is what turns this into the
// "is not a directory" message.
func clientTypesFilePathIntoCreateCWDField(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "create-validation-file-"+label)
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		return fmt.Errorf("create file labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, path)
	if err := client.Send("\t" + path); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientSubmitsCreateModalExpectingRejection presses enter and waits for
// the create modal's own rejection text (m.createError, rendered as
// "Cannot create session: ...") rather than for the modal to close --
// unlike clientSubmitsCreateModalWithBlankName (features/create_blank_name_test.go),
// which waits for the opposite: the title leaving the frame on success.
func clientSubmitsCreateModalExpectingRejection(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Cannot create session")
}

// stateDatabaseHasZeroSessions asserts the sessions table is empty --
// requirement 15's esc-abandons-creates-nothing proof, read from the store
// rather than only the screen.
func stateDatabaseHasZeroSessions(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		if err == sql.ErrNoRows {
			count = 0
		} else {
			return fmt.Errorf("count sessions: %w", err)
		}
	}
	if count != 0 {
		return fmt.Errorf("sessions table has %d row(s), want zero", count)
	}
	return nil
}

// dewrapFrame strips the framedDialog box border ("|" plus its padding) from
// every line of a rendered frame and joins what remains with single spaces,
// reconstructing text that createView's own word-wrap (fitting a long
// rejection message, e.g. a quoted absolute path, inside the dialog's fixed
// width) split across two or more screen rows -- word-wrap breaks only at a
// space, so re-joining with one space per line boundary is lossless for the
// substring checks this package needs, without having to keep every
// scenario's fixture paths short enough to never wrap.
func dewrapFrame(frame string) string {
	var b strings.Builder
	for _, line := range strings.Split(frame, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "|"))
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString(" ")
	}
	return b.String()
}

// clientScreenContainsDewrapped is clientScreenContains' word-wrap-tolerant
// counterpart (see dewrapFrame): it polls the same way, but checks the
// dewrapped frame rather than the raw one, so a message whose exact wrap
// point depends on a scenario's own fixture path length (e.g. requirement
// 15's "does not exist"/"is not a directory" messages, which quote the
// full resolved cwd) can still be asserted as one contiguous phrase.
func clientScreenContainsDewrapped(ctx context.Context, name, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		joined := dewrapFrame(client.Frame(false))
		if strings.Contains(joined, want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q dewrapped screen does not contain %q:\ndewrapped: %s\nframe:\n%s", name, want, joined, client.Frame(false))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
