package tmux

import "testing"

// TestCIVerifyAlwaysRed is a throwaway probe for task 019 (C.7): it fails on
// every invocation, on purpose, to turn a ci-verify/* PR's suite check red.
// Never merged to main -- this file lives only on this throwaway branch and
// is removed by the next commit before the PR closes.
func TestCIVerifyAlwaysRed(t *testing.T) {
	t.Fatal("ci-verify: intentional always-failing test (task 019, R145/R146)")
}
