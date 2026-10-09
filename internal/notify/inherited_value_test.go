package notify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// deliver runs req attached (Spawn) or detached (Start) and returns the
// environment the script saw, once it has finished recording it.
func deliver(t *testing.T, detached bool, req Request, dir string) map[string]string {
	t.Helper()
	if detached {
		if err := Start(req); err != nil {
			t.Fatal(err)
		}
		waitFile(t, filepath.Join(dir, "finished"))
	} else if _, err := Spawn(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	return envMap(read(t, filepath.Join(dir, "env")))
}

// assertNoValue fails when any session env value appears anywhere in the
// script's environment, under any name.
func assertNoValue(t *testing.T, env map[string]string, values ...string) {
	t.Helper()
	for name, got := range env {
		for _, value := range values {
			if strings.Contains(got, value) {
				t.Errorf("%s=%q carries the session env value %q", name, got, value)
			}
		}
	}
}

// SPEC §6.4, §10.1: `deck _hook` inherits the agent environment, so a session
// env value can arrive under a name the session env does not define (a copy,
// an alias, a composed option string). The rule is by value, not by key, and
// the attached and the detached spawn apply it identically.
func TestASessionEnvValueInheritedUnderAnotherNameIsNotPassedOn(t *testing.T) {
	cases := []struct {
		name       string
		sessionEnv map[string]string
		inherited  []string
		values     []string
	}{
		{
			name:       "verbatim alias of a long secret",
			sessionEnv: map[string]string{"API_TOKEN": "tok-abcdef-123456"},
			inherited:  []string{"API_TOKEN=tok-abcdef-123456", "AGENT_AUTH_COPY=tok-abcdef-123456"},
			values:     []string{"tok-abcdef-123456"},
		},
		{
			name:       "secret embedded in a composed option string",
			sessionEnv: map[string]string{"DB_PASSWORD": "hunter2-pw"},
			inherited:  []string{"CURL_OPTS=-H Authorization:Bearer hunter2-pw --silent"},
			values:     []string{"hunter2-pw"},
		},
		{
			name:       "one-character value inside another variable",
			sessionEnv: map[string]string{"LEVEL": "q"},
			inherited:  []string{"BUILD_TAG=xqy", "LEVEL=q"},
			values:     []string{"q"},
		},
	}
	for _, tc := range cases {
		for _, detached := range []bool{false, true} {
			name := tc.name + "/attached"
			if detached {
				name = tc.name + "/detached"
			}
			t.Run(name, func(t *testing.T) {
				path, dir := captureScript(t, `echo finished > "$d/finished"`)
				req := baseRequest(path)
				req.SessionEnv = tc.sessionEnv
				req.BaseEnv = append(req.BaseEnv, tc.inherited...)
				env := deliver(t, detached, req, dir)
				assertNoValue(t, env, tc.values...)
				if env["KEEP"] != "yes" {
					t.Errorf("an unrelated inherited variable was dropped: %v", env)
				}
				for _, entry := range tc.inherited {
					if name, _, _ := strings.Cut(entry, "="); env[name] != "" {
						t.Errorf("inherited %s reached the script", entry)
					}
				}
			})
		}
	}
}

// A payload-file failure on the detached path (the temp dir is itself a
// session env value, as it is when `deck _hook` inherits a session-defined
// TMPDIR) is a recordable not-started error, so it is scrubbed too.
func TestStartPayloadFileErrorCarriesNoSessionEnvValue(t *testing.T) {
	for name, tmp := range map[string]string{
		"long value":  "/nonexistent-deck-secret-dir-77",
		"short value": "/j",
	} {
		t.Run(name, func(t *testing.T) {
			path, _ := captureScript(t, "")
			t.Setenv("TMPDIR", tmp)
			req := baseRequest(path)
			req.SessionEnv = map[string]string{"TMPDIR": tmp}
			err := Start(req)
			if err == nil {
				t.Fatal("Start succeeded with an unusable TMPDIR, want a payload-file error")
			}
			if strings.Contains(err.Error(), tmp) {
				t.Errorf("err = %q carries the session env value %q", err, tmp)
			}
			if !strings.Contains(err.Error(), "payload file") {
				t.Errorf("err = %q, want the payload-file failure named", err)
			}
		})
	}
}

func TestCarriesSessionEnvMatchesByKeyOrByValueAnywhereInTheEntry(t *testing.T) {
	sessionEnv := map[string]string{"MODE": "7", "EMPTY": ""}
	values := scrubValues(sessionEnv)
	for entry, want := range map[string]bool{
		"MODE=other": true,  // named like a session key
		"EMPTY=x":    true,  // named like a session key, even when that value is empty
		"N=x7":       true,  // carries the value under another name
		"N=x":        false, // unrelated; an empty session value matches nothing
		"N7=x":       true,  // in the name counts too: the whole entry is checked
		"NOEQUALS":   false,
	} {
		if got := carriesSessionEnv(entry, sessionEnv, values); got != want {
			t.Errorf("carriesSessionEnv(%q) = %v, want %v", entry, got, want)
		}
	}
}
