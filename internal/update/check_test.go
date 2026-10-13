package update

import (
	"context"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v1.2.3", "v1.2.2", true},
		{"v1.10.0", "v1.9.9", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.2", "v1.2.3", false},
		{"v1.2.3", "dev", false},
		{"v1.2.3", "", false},
		{"v1.2.3-rc1", "v1.2.2", false},
		{"nonsense", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.tag, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

func TestCheckReportsNewerAndCurrentWithoutDownloading(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	for current, wantNewer := range map[string]bool{"v1.0.0": true, tagV: false, "v9.0.0": false, "dev": false} {
		res, err := Check(context.Background(), Options{BaseURL: srv.URL, Current: current})
		if err != nil {
			t.Fatalf("Check(%s): %v", current, err)
		}
		if res.Latest != tagV || res.Newer != wantNewer || res.Current != current {
			t.Errorf("Check(%s) = %+v, want latest %s newer %v", current, res, tagV, wantNewer)
		}
	}
}

func TestCheckOfflineIsAnError(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	url := srv.URL
	srv.Close()
	if _, err := Check(context.Background(), Options{BaseURL: url, Current: "v1.0.0"}); err == nil {
		t.Fatal("Check against a closed server succeeded")
	}
	if _, err := Check(context.Background(), Options{BaseURL: "http://example.com", Current: "v1.0.0"}); err == nil {
		t.Fatal("Check accepted a plain-http non-loopback base")
	}
}

func TestOnlyIfNewerLeavesTheBinaryAlone(t *testing.T) {
	rel := newRelease(t)
	srv := rel.serve(t)
	dir, exe := installOld(t)
	res, err := Run(context.Background(), Options{BaseURL: srv.URL, Exe: exe, Current: tagV, OnlyIfNewer: true})
	if err != nil || !res.Unchanged || res.Tag != tagV {
		t.Fatalf("Run = %+v, %v; want Unchanged at %s", res, err, tagV)
	}
	if fileSum(t, exe) != sum([]byte(oldBody)) || len(listing(t, dir)) != 1 {
		t.Error("an up-to-date install touched the directory")
	}
}
