package tui

import (
	"os"
	"testing"

	"github.com/n-orlov/deck/internal/testnet"
)

// The update-source tests in this package use a local fake release server
// (DECK_UPDATE_URL). Behind a rejecting proxy, any test that forgot it fails
// the package naming the public host it reached.
//
// A loopback mirror has no GitHub release API, so the default immutability
// requirement refuses every install from one. The package baseline is the
// DECK_REQUIRE_IMMUTABLE=0 opt-out, so the install tests exercise the steps
// they are about; the tests of the default (update_immutable_test.go) unset
// it themselves.
func TestMain(m *testing.M) {
	_ = os.Setenv("DECK_REQUIRE_IMMUTABLE", "0")
	os.Exit(testnet.Main(m))
}
