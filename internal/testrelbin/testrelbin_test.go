package testrelbin

import (
	"bytes"
	"debug/buildinfo"
	"testing"
)

func TestBinaryCarriesTheRequestedVersion(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "v0.9.0"} {
		b := Bytes(t, tag)
		info, err := buildinfo.Read(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: build info unreadable: %v", tag, err)
		}
		if info.Main.Version != tag {
			t.Errorf("Main.Version = %q, want %q", info.Main.Version, tag)
		}
		if info.Main.Path != modulePath {
			t.Errorf("%s: Main.Path = %q, want %q", tag, info.Main.Path, modulePath)
		}
	}
}
