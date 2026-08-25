package tmux

import (
	"context"
	"fmt"
	"strings"
)

// SelectionBufferName is the fixed name deck's own drag-to-copy selection
// (SPEC §11.8's "Selection and copy") writes into on deck's OWN tmux
// server. It is fixed rather than per-selection-unique the way send.go's
// literal/multiline stream buffers are: there is exactly one "most recent
// drag-to-copy selection" a user can have at a time, and a scenario
// asserting the copy (`tmux -L <socket> show-buffer -b deck-selection`)
// needs a name it can name up front rather than first discovering
// whatever deck happened to pick.
const SelectionBufferName = "deck-selection"

// SetSelectionBuffer loads text into deck's own named selection buffer via
// `load-buffer` (reading the payload over stdin, exactly like send.go's
// literal/multiline streaming -- no argv-length ceiling, so an
// arbitrarily long drag-select is never truncated by tmux's own ~16 KiB
// internal command-string ceiling the way an argv-based `set-buffer`
// would be, see send.go's literalChunkBytes doc). This never touches any
// pane -- it only stages bytes into a server-side buffer -- so it does
// not go through Dispatcher.Send's pane-identity re-verification; there
// is no pane identity to protect here at all, only a buffer name.
func (c Client) SetSelectionBuffer(ctx context.Context, text string) error {
	if _, err := c.runWithStdin(ctx, strings.NewReader(text), "load-buffer", "-b", SelectionBufferName, "-"); err != nil {
		return fmt.Errorf("write drag-to-copy selection (%d bytes) to buffer %q: %w", len(text), SelectionBufferName, err)
	}
	return nil
}
