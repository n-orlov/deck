// Package notify is deck's one extension point for status-change events: the
// no-shell event-hook spawner of SPEC §10.1 and §10.3.
//
// deck has no notification client, no channel types and no templates. Spawn
// runs the user's configured executable (never through a shell), hands it
// the event kind as argv[1], the DECK_SESSION_* environment of §6.1 plus the
// DECK_EVENT_* variables, and the versioned JSON payload on stdin, then
// returns the exit status and a capped tail of its output. It keeps no
// outbox and never retries.
//
// The package deliberately imports neither the store nor the service: the
// caller projects a session row onto Session and passes the session's env
// only so its values can be scrubbed (§6.4), never to export them.
package notify
