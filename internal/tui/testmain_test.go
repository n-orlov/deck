package tui

import (
	"os"
	"testing"

	"github.com/n-orlov/deck/internal/testnet"
)

// The update-source tests in this package use a local fake release server
// (DECK_UPDATE_URL). Behind a rejecting proxy, any test that forgot it fails
// the package naming the public host it reached.
func TestMain(m *testing.M) { os.Exit(testnet.Main(m)) }
