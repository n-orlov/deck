package service

import "time"

// issueDiscount opens a discount of the given kind the way a deck caller does:
// issuedAt is the instant it took BEFORE it issued the tmux call or write
// (R410), and the clock's own Now is when the call returned. The span starts at
// issuedAt.
func issueDiscount(c *OutputClock, id, kind string, issuedAt time.Time) {
	switch kind {
	case "resize":
		c.Discount(id, DiscountResize, issuedAt)
	case "key":
		c.Discount(id, DiscountKey, issuedAt)
	case "attach-start":
		c.AttachStarted(id, issuedAt)
	case "attach-end":
		c.AttachEnded(id, issuedAt)
	default:
		panic("unknown discount kind " + kind)
	}
}
