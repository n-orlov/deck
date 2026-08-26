#!/bin/sh
# Experiment wrapper: delay resize-window calls to simulate host-load latency
# in the async previewFit path, to prove causation for task 406.
for a in "$@"; do
  if [ "$a" = "resize-window" ]; then
    echo "[tmuxwrap] delaying resize-window 150ms" >> /tmp/406work/tmuxwrap.log
    sleep 0.15
    break
  fi
done
exec /usr/bin/tmux "$@"
