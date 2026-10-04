package tui

import (
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// A toggle field the FullKey switch in settingsToggleValue has no case for
// reads as its schema default (false when the default is absent or not a
// bool), whatever the config holds, and is never written by settingsSetToggle.
func TestSettingsToggleValueUnknownKeyFallsBackToSchemaDefault(t *testing.T) {
	cfg := config.FileConfig{AllowYolo: true, YoloDefault: true, TmuxMouse: true}
	cases := []struct {
		name string
		def  any
		want bool
	}{
		{"default true", true, true},
		{"default false", false, false},
		{"no default", nil, false},
		{"non-bool default", "yes", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := config.Field{Key: "no_such_toggle", Kind: config.KindToggle, Default: tc.def}
			if got := settingsToggleValue(f, cfg); got != tc.want {
				t.Fatalf("settingsToggleValue(unknown key, default %v) = %v, want %v", tc.def, got, tc.want)
			}
			before := cfg
			settingsSetToggle(&cfg, f, !tc.want)
			if !reflect.DeepEqual(cfg, before) {
				t.Fatalf("settingsSetToggle wrote to the config for an unknown key: %+v -> %+v", before, cfg)
			}
		})
	}
}
