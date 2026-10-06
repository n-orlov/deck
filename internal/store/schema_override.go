package store

import "strconv"

// parseSchemaOverride turns the text of a test build's schema override into a
// schema version. A value that is not a number, or is below 1, yields
// fallback. Only the deckoldschema build tag calls it for a real build; it
// lives outside the tagged file so the default coverage profile exercises it.
func parseSchemaOverride(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
