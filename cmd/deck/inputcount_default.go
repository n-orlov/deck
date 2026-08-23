//go:build !deckinputcount

package main

import tea "github.com/charmbracelet/bubbletea"

// wrapForInputCounting is the no-op the released binary always sees:
// `go build ./...`, `go vet ./...`, `go test ./...` and every real deck
// install use this file. It exists so I-1's investigation can ask, from
// outside the process, "how many keys did the running deck program actually
// receive" without adding any surface -- env var, key or otherwise -- to the
// shipped product. See inputcount_hook.go, which is compiled in only when a
// test explicitly builds `-tags deckinputcount`, for the counting wiring
// that proof needs.
func wrapForInputCounting(model tea.Model) tea.Model { return model }
