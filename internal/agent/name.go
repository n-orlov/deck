package agent

// NameLaunch says how an adapter hands deck's session name to its agent when
// it starts one (SPEC §8). The zero value is NameLaunchNone.
type NameLaunch string

const (
	// NameLaunchNone means the agent takes no name at launch.
	NameLaunchNone NameLaunch = "none"
	// NameLaunchFlag means a launch flag carries the name, on every launch and resume.
	NameLaunchFlag NameLaunch = "flag"
	// NameLaunchFlagNewSessionOnly means a launch flag carries the name for a new
	// conversation only, never for a resume.
	NameLaunchFlagNewSessionOnly NameLaunch = "flag_new_session_only"
	// NameLaunchSendKeysAfterReady means the name is typed into the running agent
	// once it is ready.
	NameLaunchSendKeysAfterReady NameLaunch = "sendkeys_after_ready"
	// NameLaunchExtension means an injected extension sets the name.
	NameLaunchExtension NameLaunch = "extension"
)

// NamePush says how a deck rename reaches a running agent. The zero value is
// NamePushNone.
type NamePush string

const (
	// NamePushNone means a running agent cannot be renamed from deck.
	NamePushNone NamePush = "none"
	// NamePushSendKeys means the adapter's PushName keystroke plan renames it.
	NamePushSendKeys NamePush = "sendkeys"
	// NamePushExtension means an injected extension renames it.
	NamePushExtension NamePush = "extension"
)

// Name observation sources an adapter may list in NameCaps.ObserveName.
const (
	ObserveHook       = "hook"
	ObservePaneTitle  = "pane_title"
	ObserveTranscript = "transcript"
	ObserveSQLite     = "sqlite"
	ObserveYAML       = "yaml"
	ObserveExtension  = "extension"
)

// NameCaps declares how an agent takes, generates, accepts and reports a
// session name. It is queried, never assumed: ReadName is asked only of an
// adapter whose ObserveName is non-empty, PushName only of one whose
// PushRename is NamePushSendKeys.
type NameCaps struct {
	// LaunchName says how the name reaches the agent at launch.
	LaunchName NameLaunch
	// AutoTitle reports whether the agent generates its own title.
	AutoTitle bool
	// PushRename says how a deck rename reaches a running agent.
	PushRename NamePush
	// ObserveName lists the sources the agent's current name can be read
	// from (the Observe* constants).
	ObserveName []string
}

// ClaimsRead reports whether the adapter declares any source ReadName can use.
func (c NameCaps) ClaimsRead() bool { return len(c.ObserveName) > 0 }

// ClaimsPush reports whether the adapter declares a keystroke-plan rename.
func (c NameCaps) ClaimsPush() bool { return c.PushRename == NamePushSendKeys }

// NameProbeInput identifies the conversation whose current harness name
// ReadName is asked for. It is independent of internal/store, like
// TranscriptInput.
type NameProbeInput struct {
	// Home is the resolved $HOME to search under.
	Home string
	// CWD is the session's working directory.
	CWD string
	// ConversationID is the conversation whose name is read. Always known:
	// deck never asks about a conversation it has not bound.
	ConversationID string
	// Env carries the resolved session environment values the adapter's
	// Caps.TranscriptEnvKeys named, as in TranscriptInput.Env.
	Env map[string]string
}

// NamePushInput carries the name a PushName keystroke plan renames to.
type NamePushInput struct {
	// Name is the already-sanitised new name; never blank.
	Name string
}

// NameReader is the optional interface of an adapter whose NameCaps claim an
// observation source. ReadName returns the agent's best current name for the
// conversation; explicit reports that the agent says a human chose it (as
// opposed to an auto-title); ok=false means no name could be read.
type NameReader interface {
	ReadName(in NameProbeInput) (name string, explicit bool, ok bool)
}

// NamePusher is the optional interface of an adapter whose PushRename is
// NamePushSendKeys. PushName returns the keystroke plan, one send-keys call
// per element, that renames the running agent; ok=false declines.
type NamePusher interface {
	PushName(in NamePushInput) (keys []string, ok bool)
}

// ReadName is the dispatcher every caller goes through: it asks a.ReadName
// only when a's caps claim an observation source, so an adapter that
// implements the method without claiming it is never called.
func ReadName(a Adapter, in NameProbeInput) (name string, explicit bool, ok bool) {
	reader, isReader := a.(NameReader)
	if !isReader || !a.Capabilities().Name.ClaimsRead() {
		return "", false, false
	}
	return reader.ReadName(in)
}

// PushName is ReadName's counterpart for the keystroke plan: it asks a.PushName
// only when a's caps declare PushRename=sendkeys.
func PushName(a Adapter, in NamePushInput) (keys []string, ok bool) {
	pusher, isPusher := a.(NamePusher)
	if !isPusher || !a.Capabilities().Name.ClaimsPush() {
		return nil, false
	}
	return pusher.PushName(in)
}
