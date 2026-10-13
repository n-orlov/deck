package update

// testRoot is nil in every shipped build: the managed prefixes are matched
// against the real filesystem root. testroot_hook.go sets it in a build made
// with `-tags deckupdatetest`.
var testRoot func() string
