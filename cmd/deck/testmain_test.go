package main

import (
	"os"
	"testing"

	"github.com/n-orlov/deck/internal/testnet"
)

// Every test in this package, and every deck binary one launches from
// os.Environ, runs behind a rejecting proxy: a fixture that starts deck must
// ask a local fake release server (DECK_UPDATE_URL) or set DECK_UPDATE_CHECK=0,
// or the package fails naming the public host it reached.
func TestMain(m *testing.M) { os.Exit(testnet.Main(m)) }
