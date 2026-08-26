The wrapper script used for every deliberate-delay experiment in this
report. Injected as the front of `PATH` for every deck client the harness
starts (via a one-line, never-committed edit to
`features/lifecycle_test.go`'s `Environment()`, reverted immediately after
each experiment -- `git diff features/lifecycle_test.go` is empty in every
commit this task made). Delays only `resize-window` tmux subcommands (the
one command `tmux.Client.FitWindowToPane`/`resizeWindow`,
`internal/tmux/geometry.go:246-296`, ever issues) by a configurable amount,
then execs the real `tmux` unchanged for everything else -- every other
tmux interaction in the scenario (capture-pane, session/window queries,
send-keys) is completely unaffected.
