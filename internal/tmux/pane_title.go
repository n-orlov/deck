package tmux

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// paneTitleFormat reads one free-text field. It is deliberately NOT part of
// paneFactsFormat, which stays free of free text: a title is whatever the
// program in the pane set, so it is read on its own, one call per pane, and
// only ever returned as data.
const paneTitleFormat = "#{pane_title}"

// PaneTitle reads `#{pane_title}` of the active pane of slug's session: the
// title the agent in it set (SPEC §8.1). A vanished session or pane reads as an
// empty title with a nil error. Only the first line is returned, with
// surrounding whitespace removed.
func (c Client) PaneTitle(ctx context.Context, slug string) (string, error) {
	if c.Socket == "" {
		return "", errors.New("tmux socket name is required")
	}
	name, err := sessionName(slug)
	if err != nil {
		return "", err
	}
	data, err := c.run(ctx, "display-message", "-p", "-t", "="+name+":", paneTitleFormat)
	if err != nil {
		if IsTargetAbsent(err) {
			return "", nil
		}
		return "", fmt.Errorf("read pane title of %q: %w", name, err)
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), nil
}
