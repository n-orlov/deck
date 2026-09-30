//go:build !race

// Package racebuild exposes a single build-tagged constant that lets tests
// and production code tell whether the current binary was built with the
// Go race detector enabled (`go test -race` / `go build -race`).
package racebuild

// Enabled is false when this binary was built without `-race`.
const Enabled = false
