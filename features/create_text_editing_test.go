package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// This file is task 013's (R178, SPEC §11.4/§11.11) real-tmux contract for the
// create modal's name field: left and right are the shared line editor's
// caret keys there, and the Agent selection the dialog contract used to cycle
// with them does not move.

func registerCreateTextEditingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" types "([^"]*)" into the create name field$`, clientSendsTextToCreateNameField)
	sc.Step(`^deck client "([^"]+)" presses (left|right) (\d+) times in the create name field$`, clientPressesCaretKeyInCreateNameField)
	sc.Step(`^deck client "([^"]+)" create name caret is on "([^"]+)"$`, clientCreateNameCaretIsOn)
	sc.Step(`^deck client "([^"]+)" create name caret is after "([^"]+)"$`, clientCreateNameCaretIsAfter)
}

// clientSendsTextToCreateNameField types into the name field, which holds focus
// when the modal opens, and waits for nothing: the scenario's next step reads
// the text and the caret cell off the frame that draws them.
func clientSendsTextToCreateNameField(ctx context.Context, clientName, text string) error {
	client, err := assertionClient(ctx, clientName)
	if err != nil {
		return err
	}
	return client.Send(text)
}

// clientPressesCaretKeyInCreateNameField sends n real left or right keys, each
// its own write so every one reaches the field as its own key message. There
// is nothing on screen to wait on for a caret move alone: the scenario's next
// step reads the caret cell itself.
func clientPressesCaretKeyInCreateNameField(ctx context.Context, clientName, direction string, n int) error {
	client, err := assertionClient(ctx, clientName)
	if err != nil {
		return err
	}
	seq := map[string]string{"left": "\x1b[D", "right": "\x1b[C"}[direction]
	for i := 0; i < n; i++ {
		if err := client.Send(seq); err != nil {
			return err
		}
	}
	return nil
}

// createNameCaret reads the Name row off the emulator grid: the text after
// "Name: " up to its right edge, and the columns of every reverse-video cell
// in it (the caret, §11.11).
func createNameCaret(client *ScreenDriver) (text string, reversed []int, firstCol int, err error) {
	row, col, err := client.FindText("Name: ")
	if err != nil {
		return "", nil, 0, err
	}
	cols, _ := client.GridSize()
	firstCol = col + len("Name: ")
	var b strings.Builder
	for x := firstCol; x < cols; x++ {
		cell := client.CellAt(x, row)
		if cell == nil {
			break
		}
		if has, err := cellHasAttr(cell, "reverse"); err == nil && has {
			reversed = append(reversed, x-firstCol)
		}
		content := cell.Content
		if content == "" {
			content = " "
		}
		b.WriteString(content)
	}
	return b.String(), reversed, firstCol, nil
}

// waitCreateNameCaret polls the grid, on every frame update, until pred holds
// for the Name row's text and caret offsets.
func waitCreateNameCaret(ctx context.Context, clientName, what string, pred func(text string, reversed []int) bool) error {
	client, err := assertionClient(ctx, clientName)
	if err != nil {
		return err
	}
	var last string
	_, err = client.WaitForFrameFunc(ctx, false, func(string) bool {
		text, reversed, _, rerr := createNameCaret(client)
		if rerr != nil {
			return false
		}
		last = fmt.Sprintf("row %q, reverse-video cells at offsets %v", strings.TrimRight(text, " "), reversed)
		return pred(text, reversed)
	})
	if err != nil {
		return fmt.Errorf("client %q: want the create name caret %s; last read: %s: %w", clientName, what, last, err)
	}
	return nil
}

// clientCreateNameCaretIsOn asserts exactly one reverse-video cell on the Name
// row and that it covers the character want.
func clientCreateNameCaretIsOn(ctx context.Context, clientName, want string) error {
	return waitCreateNameCaret(ctx, clientName, "on "+want, func(text string, reversed []int) bool {
		return len(reversed) == 1 && strings.HasPrefix(text[reversed[0]:], want)
	})
}

// clientCreateNameCaretIsAfter asserts exactly one reverse-video cell on the
// Name row, a blank sitting immediately after the text want.
func clientCreateNameCaretIsAfter(ctx context.Context, clientName, want string) error {
	return waitCreateNameCaret(ctx, clientName, "after "+want, func(text string, reversed []int) bool {
		return len(reversed) == 1 && reversed[0] == len(want) && strings.HasPrefix(text, want+" ")
	})
}
