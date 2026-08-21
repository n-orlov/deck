//go:build !decktestpanic

package main

import tea "github.com/charmbracelet/bubbletea"

// wrapForDeliberateTestPanic is the no-op the released binary always sees:
// `go build ./...`, `go vet ./...`, `go test ./...` and every real deck
// install use this file. It exists so requirement 36's "mouse reporting is
// disabled on every exit path, including a panic" can be proven end to end
// through a real PTY without adding any surface — env var, key or otherwise
// — to the shipped product. See testpanic_hook.go, which is compiled in
// only when a test explicitly builds `-tags decktestpanic`, for the
// deliberate-panic wiring that proof needs.
func wrapForDeliberateTestPanic(model tea.Model) tea.Model { return model }
