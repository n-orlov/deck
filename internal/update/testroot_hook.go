//go:build deckupdatetest

package update

import "os"

// This file is compiled in only when a test builds the real deck binary with
// `-tags deckupdatetest` (cmd/deck's TestUpdateVerbAgainstFakeServer). A test
// cannot create /usr/bin or /snap, so the managed-prefix refusal of the real
// binary is proven by relocating the tree the prefixes are matched under to a
// temporary directory named in DECK_UPDATE_TEST_ROOT. It is never part of a
// release, `go build ./...` or `go test ./...`.
func init() { testRoot = func() string { return os.Getenv("DECK_UPDATE_TEST_ROOT") } }
