package tmux

import (
	"context"
	"errors"
	"fmt"
)

// SendKeyPlan runs an adapter's keystroke plan against slug's live pane, one
// send-keys call per element and in order (SPEC §8 "Pushing a rename"). An
// element that is on the named-key allowlist (`Enter`) is sent as a tmux key
// name; every other element is typed as literal text (a control byte such as
// "\x15", Ctrl-U, is literal text too). The plan stops at the first failing
// element and is never retried here. Like SendKeys, the target is the pane
// id resolved once from the slug, never the session name, and every send is
// identity-verified by the Dispatcher.
func (c Client) SendKeyPlan(ctx context.Context, slug string, plan []string) error {
	if c.Socket == "" {
		return errors.New("tmux socket name is required")
	}
	if len(plan) == 0 {
		return errors.New("send key plan: the plan is empty")
	}
	pane, ok, err := c.PreviewPane(ctx, slug)
	if err != nil {
		return fmt.Errorf("resolve pane for session %q: %w", slug, err)
	}
	if !ok {
		return fmt.Errorf("send key plan to session %q: no live pane", slug)
	}
	dispatcher, err := NewDispatcher(ctx, c, pane.ID)
	if err != nil {
		return fmt.Errorf("send key plan to session %q: %w", slug, err)
	}
	for _, element := range plan {
		if err := sendPlanElement(ctx, dispatcher, element); err != nil {
			return fmt.Errorf("send key plan to session %q: %w", slug, err)
		}
	}
	return nil
}

func sendPlanElement(ctx context.Context, dispatcher *Dispatcher, element string) error {
	if namedKeyAllowlist[element] {
		return dispatcher.SendNamedKey(ctx, element)
	}
	return dispatcher.SendLiteral(ctx, element)
}
