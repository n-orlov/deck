package theme

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed builtin/*.toml
var builtinFS embed.FS

// builtinFiles lists the embedded theme files, one registry entry each,
// as §11.6 requires ("adding a built-in is a one-file drop plus one
// registry entry"). One dark, one light, as task 008 requires as a floor;
// more can be added the same way.
var builtinFiles = []string{
	"builtin/empire.toml",
	"builtin/daylight.toml",
}

var builtins map[string]*Theme

func init() {
	builtins = make(map[string]*Theme, len(builtinFiles))
	for _, path := range builtinFiles {
		data, err := builtinFS.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("theme: embedded built-in %s: %v", path, err))
		}
		t, err := Parse(data, "<builtin:"+path+">")
		if err != nil {
			panic(fmt.Sprintf("theme: embedded built-in %s: %v", path, err))
		}
		if _, dup := builtins[t.Name]; dup {
			panic(fmt.Sprintf("theme: duplicate built-in name %q", t.Name))
		}
		builtins[t.Name] = t
	}
}

// Builtin returns the built-in theme named name, or false if there is no
// such built-in.
func Builtin(name string) (*Theme, bool) {
	t, ok := builtins[name]
	return t, ok
}

// Builtins returns every embedded built-in theme, sorted by name for
// deterministic iteration (the picker in task 018 lists them in this
// order for the built-in portion of its list).
func Builtins() []*Theme {
	out := make([]*Theme, 0, len(builtins))
	for _, t := range builtins {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DefaultName is the built-in selected when no theme is configured, or
// when a configured name cannot be resolved (task 009/019 handle the
// fallback-with-reason behaviour; this is just the name they fall back
// to).
const DefaultName = "empire"

// Default returns the default built-in theme. It panics if the embedded
// default is somehow missing, which would only happen if builtinFiles
// above were edited to drop it without updating DefaultName — a bug
// caught immediately by every test in this package.
func Default() *Theme {
	t, ok := Builtin(DefaultName)
	if !ok {
		panic("theme: default built-in " + DefaultName + " is not embedded")
	}
	return t
}
