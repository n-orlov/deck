package agent

// claudeClearLine is Ctrl-U, which empties Claude's input line. It is sent as
// literal text (the key-plan sender types every element that is not a tmux key
// name), so a half-typed prompt is not glued onto the rename command.
const claudeClearLine = "\x15"

// PushName is Claude's rename keystroke plan (R256): clear the input line,
// type `/rename <name>`, then a lone Enter. Each element is one send-keys call,
// and Enter is never folded into the text, so Claude sees the command typed
// before it is submitted. The name is cleaned for Claude again here (control
// characters removed, at most 200 runes), so a newline in a name can never
// submit the command early; a name with nothing left is declined.
func (Claude) PushName(in NamePushInput) (keys []string, ok bool) {
	name, ok := SanitisePushName("claude", in.Name)
	if !ok {
		return nil, false
	}
	return []string{claudeClearLine, "/rename " + name, "Enter"}, true
}
