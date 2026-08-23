//go:build !deckinputcount

package main

import tea "github.com/charmbracelet/bubbletea"

// rawByteCountingProgramOptions is the no-op the released binary always
// sees: `go build ./...`, `go vet ./...`, `go test ./...` and every real
// deck install use this file. It exists so I-1's investigation can ask,
// from outside the process, "how many raw bytes did bubbletea's own reader
// consume from stdin" without adding any surface to the shipped product.
// See rawbytecount_hook.go, compiled in only under `-tags deckinputcount`,
// for the counting wiring that proof needs.
func rawByteCountingProgramOptions() []tea.ProgramOption { return nil }
