#!/usr/bin/env bash
# ci/tmux-guard.sh -- cure-01-01 (task cure-01-01, R158): a hard refusal in
# front of every tmux invocation the test suite makes, so no fixture --
# accidentally or otherwise -- can ever land a command against the
# operator's real "deck"/"deck-*" tmux servers from inside a test run.
#
# This script is installed ahead of the real tmux binary on PATH only for
# the guarded test invocation (see ci/run.sh); it is never used for an
# interactive deck session. It inspects the argv for a "-L <name>" (or
# "-L=<name>") socket-name argument -- tmux's own flag for naming the
# server socket -- and refuses outright, before running anything, if
# <name> is exactly "deck" or starts with "deck-". Everything else execs
# straight through to the real tmux so behaviour is unchanged.
#
# A single, narrow, auditable exemption exists for the handful of feature
# scenarios that deliberately prove SPEC §3.4/R156/R157's own "deck" /
# "deck-<profile>" socket derivation for real (never against the
# operator's server -- always a throwaway one on a sibling test
# container): those call sites set DECK_TEST_ALLOW_NAMESPACED_SOCKET=1 in
# the guarded process's own environment before invoking tmux. Every other
# fixture in the repo now names its sockets outside the deck/deck-*
# namespace, so this exemption should never grow.
set -euo pipefail

log() {
	if [ -n "${DECK_TEST_TMUX_GUARD_LOG:-}" ]; then
		printf '%s\n' "$1" >>"$DECK_TEST_TMUX_GUARD_LOG" 2>/dev/null || true
	fi
}

real_tmux="${DECK_TEST_REAL_TMUX:-}"
if [ -z "$real_tmux" ]; then
	for candidate in /usr/bin/tmux /usr/local/bin/tmux /bin/tmux; do
		if [ -x "$candidate" ]; then
			real_tmux="$candidate"
			break
		fi
	done
fi
if [ -z "$real_tmux" ]; then
	echo "tmux-guard.sh: cannot find the real tmux binary (set DECK_TEST_REAL_TMUX)" >&2
	exit 98
fi

socket=""
prev=""
for arg in "$@"; do
	if [ "$prev" = "-L" ]; then
		socket="$arg"
		break
	fi
	case "$arg" in
	-L=*) socket="${arg#-L=}" ;;
	esac
	prev="$arg"
done

if [ -n "$socket" ]; then
	case "$socket" in
	deck | deck-*)
		if [ "${DECK_TEST_ALLOW_NAMESPACED_SOCKET:-}" = "1" ]; then
			log "tmux-guard.sh: ALLOWED (namespaced-socket exemption) -L $socket: $*"
		else
			log "tmux-guard.sh: DENIED -L $socket: $*"
			echo "REVIEW_GUARD_DENIED: refusing tmux -L $socket (task cure-01-01, R158: never touch the operator's deck/deck-* tmux server)" >&2
			exit 97
		fi
		;;
	esac
fi

exec "$real_tmux" "$@"
