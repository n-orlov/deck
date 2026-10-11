package agent

import (
	"os"
	"path/filepath"
)

// PiHookExecutableEnv names the environment variable that carries the Pi
// session's hook executable: the absolute path of the deck binary that
// launched it, verbatim and unquoted. The deck-owned Pi extension reads it
// when it starts and runs it with PiHookArgs as separate argv elements, never
// through a shell, so a running Pi keeps the binary of the deck that launched
// it exactly as a running Claude or Codex keeps its own hook command
// (SPEC §8.3, R204).
const PiHookExecutableEnv = "DECK_PI_HOOK_EXECUTABLE"

// PiHookArgs is the argv the extension passes to the hook executable (the
// literal in PiExtensionSource; a test keeps the two equal).
var PiHookArgs = []string{"_hook"}

// PiExtensionPath is where deck keeps the Pi extension that turns Pi's own
// events into `deck _hook` payloads, under the resolved data root deckHome.
// The file is constant (the per-session command arrives through
// PiHookExecutableEnv), so every deck binary writes the same bytes to it.
func PiExtensionPath(deckHome string) string {
	return filepath.Join(deckHome, "pi", "deck-hook.js")
}

// PiExtensionSource is the extension's text. Pi loads it with `-e <path>`
// (jiti, so the ES module syntax needs no build step). Each handler runs the
// session's hook executable with PiHookArgs as its argv, with no shell, and
// one Claude-shaped JSON payload on its stdin. A hook that fails shows its own stderr (for a database newer
// than the hook binary, the R204 restart message) as a warning in Pi's UI, and
// every failure is swallowed, so a hook problem can never stop the agent. The payload names the Pi session id so
// `_hook` addresses the row by conversation first and by DECK_SESSION_ID
// second, as it does for every other kind.
//
// The event mapping is: session_start -> SessionStart, before_agent_start ->
// UserPromptSubmit, agent_settled -> Stop (agent_end can still be retried by
// Pi, so it is not a status boundary) and session_shutdown -> SessionEnd. Pi's
// name travels too (R262): session_start adds the optional session_name when Pi
// already holds a name (--name writes it before session_start), and
// session_info_changed -> session_info_changed carries name and cleared (an
// undefined name is a cleared one). A
// shutdown that only replaces the session inside the same process (new,
// resume, fork, reload) reports the in-session reasons `_hook` already keeps
// from stopping the row.
const PiExtensionSource = `// Written by deck. Turns Pi events into ` + "`deck _hook`" + ` payloads (SPEC 8.1).
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const inSessionReasons = { new: "clear", resume: "resume", fork: "resume", reload: "resume" };

export default function (pi) {
  const executable = process.env.` + PiHookExecutableEnv + `;
  if (!executable) return;
  const fire = (event, ctx, fields) => {
    try {
      const payload = { hook_event_name: event, ...fields };
      const id = ctx && ctx.sessionManager && ctx.sessionManager.getSessionId();
      if (id) payload.session_id = id;
      const run = spawnSync(executable, ["_hook"], {
        input: JSON.stringify(payload),
        stdio: ["pipe", "ignore", "pipe"],
        timeout: 10000,
      });
      const failure = run.error ? String(run.error.message) : "";
      const message = run.status !== 0 ? (run.stderr ? String(run.stderr).trim() : failure) : "";
      if (message && ctx && ctx.hasUI) ctx.ui.notify(message, "warning");
    } catch (_) {
      // a hook failure never reaches the agent
    }
  };
  // The control file (R263): deck appends one JSON line {"name": "..."} per
  // rename and the extension hands each to pi.setSessionName(). It is opened in
  // session_start, keyed by Pi's conversation id, and closed in session_shutdown.
  let control = null;
  const closeControl = () => {
    const open = control;
    control = null;
    if (!open) return;
    try {
      fs.unwatchFile(open.file);
      fs.closeSync(open.fd);
      fs.unlinkSync(open.file);
    } catch (_) {
      // a file that is already gone is closed
    }
  };
  const drainControl = (open) => {
    try {
      const size = fs.fstatSync(open.fd).size;
      if (size <= open.offset) return;
      const chunk = Buffer.alloc(size - open.offset);
      const read = fs.readSync(open.fd, chunk, 0, chunk.length, open.offset);
      open.offset += read;
      open.pending += chunk.toString("utf8", 0, read);
      const lines = open.pending.split("\n");
      open.pending = lines.pop();
      for (const line of lines) applyControlLine(line);
    } catch (_) {
      // an unreadable control file changes nothing
    }
  };
  const applyControlLine = (line) => {
    try {
      const name = JSON.parse(line).name;
      if (typeof name === "string" && name.trim() !== "") pi.setSessionName(name);
    } catch (_) {
      // a line that is not a name is ignored
    }
  };
  const openControl = (id) => {
    closeControl();
    const dir = process.env.` + PiControlDirEnv + `;
    if (!dir || !id || !/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(id)) return;
    try {
      fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
      const file = path.join(dir, id + ".jsonl");
      const open = { file, fd: fs.openSync(file, "w+", 0o600), offset: 0, pending: "" };
      control = open;
      fs.watchFile(file, { interval: 100, persistent: false }, () => drainControl(open));
    } catch (_) {
      // without a control file deck falls back to typing the rename
    }
  };
  pi.on("session_start", (e, ctx) => {
    openControl(ctx && ctx.sessionManager && ctx.sessionManager.getSessionId());
    let name = "";
    try {
      name = pi.getSessionName() || "";
    } catch (_) {
      // an unreadable name is no name
    }
    fire("SessionStart", ctx, name ? { source: e.reason, session_name: name } : { source: e.reason });
  });
  pi.on("before_agent_start", (e, ctx) => fire("UserPromptSubmit", ctx, {}));
  pi.on("agent_settled", (e, ctx) => fire("Stop", ctx, {}));
  pi.on("session_shutdown", (e, ctx) => {
    closeControl();
    fire("SessionEnd", ctx, { reason: inSessionReasons[e.reason] || "other" });
  });
  pi.on("session_info_changed", (e, ctx) => fire("session_info_changed", ctx, { name: e.name || "", cleared: !e.name }));
}
`

// PiHookEvents lists the hook events PiExtensionSource fires, in the order it
// subscribes them. Tests and the fake-pi fixture read it so the three never
// drift apart.
var PiHookEvents = []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd", "session_info_changed"}

// InstrumentFile is a deck-owned file an adapter's instrumentation needs on
// disk before the agent starts.
type InstrumentFile struct {
	Path    string
	Content []byte
	// DirMode is the mode of the file's directory when the launcher has to
	// create it; zero means 0750.
	DirMode os.FileMode
	// Optional marks a file whose installation failing must not fail the
	// launch: the launcher then drops DropArgv from the instrumentation argv
	// and carries on (Copilot's --plugin-dir, whose absence the pane probe
	// covers).
	Optional bool
	DropArgv []string
}

// FileInstrumenter is implemented by an adapter whose Instrument argv names a
// file deck must have written (Pi's extension and Copilot's plugin). Instrument itself
// stays pure; the launcher writes these files, once per launch, in front of
// the argv that names them.
type FileInstrumenter interface {
	InstrumentFiles(in LaunchInput) []InstrumentFile
}

// InstrumentFiles returns the Pi extension, or nothing when the launch names
// no data root to keep it in.
func (Pi) InstrumentFiles(in LaunchInput) []InstrumentFile {
	if in.DeckHome == "" {
		return nil
	}
	return []InstrumentFile{{Path: PiExtensionPath(in.DeckHome), Content: []byte(PiExtensionSource)}}
}
