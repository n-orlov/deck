#!/usr/bin/env bash
# ci/tmux-guard.sh -- task cure-01-01 (R158): a hard refusal in front of
# every tmux invocation the test suite makes, so no fixture -- accidentally
# or otherwise -- can ever land a command against the operator's real
# "deck"/"deck-*" tmux servers from inside a test run.
#
# ci/run.sh installs this script as `tmux` ahead of the real binary on PATH
# for every command it runs (including the mandated full
# `go test -p=1 -count=1 ./...`); it is never used for an interactive deck
# session. Before anything is executed it parses tmux's own global options
# (tmux(1): -2CDdhHlNquUvV are bare flags, -c -f -L -S -T take a value, either
# attached as in "-Ldeck" or as the next argument) up to the first
# non-option word, and refuses with exit 97 when the server it would reach
# is in the operator's namespace:
#   - any -L <name> where <name> is exactly "deck" or starts with "deck-";
#   - any -S <path> whose final path component is such a name;
#   - neither -L nor -S given, but $TMUX (the "current server" tmux falls
#     back to) names a socket whose final path component is such a name.
# There is NO exemption variable: a fixture that needs deck's own derived
# "deck"/"deck-<profile>" name to be observable must translate it to a
# private socket itself (see features/socket_alias_test.go), so the guard
# only ever sees private names. Everything else execs straight through to
# the real tmux, so behaviour is unchanged.
set -euo pipefail

log() {
	if [ -n "${DECK_TEST_TMUX_GUARD_LOG:-}" ]; then
		printf '%s\n' "$1" >>"$DECK_TEST_TMUX_GUARD_LOG" 2>/dev/null || true
	fi
}

forbidden_name() {
	case "$1" in
	deck | deck-*) return 0 ;;
	esac
	return 1
}

deny() {
	log "tmux-guard.sh: DENIED $1: $ARGV"
	echo "REVIEW_GUARD_DENIED: refusing tmux $1 (task cure-01-01, R158: never touch the operator's deck/deck-* tmux server)" >&2
	exit 97
}

ARGV="$*"
saw_server_flag=0
args=("$@")
i=0
while [ "$i" -lt "${#args[@]}" ]; do
	arg="${args[$i]}"
	i=$((i + 1))
	case "$arg" in
	--) break ;;
	-?*) ;;
	*) break ;; # first non-option word: the tmux command itself
	esac
	rest="${arg#-}"
	while [ -n "$rest" ]; do
		flag="${rest:0:1}"
		rest="${rest:1}"
		case "$flag" in
		c | f | L | S | T)
			if [ -n "$rest" ]; then
				value="$rest"
				rest=""
			elif [ "$i" -lt "${#args[@]}" ]; then
				value="${args[$i]}"
				i=$((i + 1))
			else
				value=""
			fi
			case "$flag" in
			L)
				saw_server_flag=1
				# "-L=deck" is not tmux syntax (it names socket "=deck"),
				# but refuse it too rather than reason about it.
				forbidden_name "$value" && deny "-L $value"
				forbidden_name "${value#=}" && deny "-L $value"
				;;
			S)
				saw_server_flag=1
				forbidden_name "$(basename -- "${value:-.}")" && deny "-S $value"
				;;
			esac
			;;
		esac
	done
done

if [ "$saw_server_flag" = 0 ] && [ -n "${TMUX:-}" ]; then
	tmux_socket="${TMUX%%,*}"
	forbidden_name "$(basename -- "$tmux_socket")" && deny "via \$TMUX=$tmux_socket"
fi

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

exec "$real_tmux" "$@"
