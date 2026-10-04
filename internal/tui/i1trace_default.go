//go:build !deckinputcount

package tui

import tea "github.com/charmbracelet/bubbletea"

// i1Trace is the no-op every ordinary build sees. See i1trace_hook.go,
// compiled in only under `-tags deckinputcount`, for the tracing this
// investigation-only instrument needs.
func i1Trace(_ string, _ tea.Msg, _ sidebarCursor) {}

func i1TraceSessions(_ Model) {}
