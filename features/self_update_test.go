package features

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/testrelbin"
)

// registerSelfUpdateSteps backs features/self_update.feature (R296). The
// scenario builds its own deck with a release version (a plain build is `dev`
// and refuses to update), runs it with a hermetic environment and an empty
// PATH, and points it at a loopback fake release server. That server is a
// mirror with no GitHub release API, so the default immutability requirement
// refuses its release; a scenario that installs passes DECK_REQUIRE_IMMUTABLE=0
// through "the operator runs ... with ...". A mirror also requires the build
// attestation (#102), so a scenario that installs puts a fake authenticated gh
// whose attestation check passes alone on PATH.
func registerSelfUpdateSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a deck binary at version "([^"]+)" installed in a scratch directory$`, selfUpdateInstallBinary)
	sc.Step(`^a fake release server whose latest release is "([^"]+)"$`, selfUpdateStartServer)
	sc.Step(`^an authenticated gh that verifies the attestation$`, selfUpdateVerifyingGh)
	sc.Step(`^the operator runs "([^"]+)"$`, selfUpdateRun)
	sc.Step(`^the operator runs "([^"]+)" with "([A-Z_]+=[^"]*)"$`, selfUpdateRunEnv)
	sc.Step(`^the command exits 0$`, selfUpdateExitsZero)
	sc.Step(`^the command exits 1$`, selfUpdateExitsOne)
	sc.Step(`^the output mentions "([^"]+)", "([^"]+)" and "([^"]+)"$`, selfUpdateOutputMentions3)
	sc.Step(`^the output mentions "([^"]+)" and "([^"]+)"$`, selfUpdateOutputMentions2)
	sc.Step(`^the installed binary is the release's binary$`, selfUpdateBinaryIsRelease)
	sc.Step(`^the installed binary is unchanged$`, selfUpdateBinaryUnchanged)
	sc.Step(`^the scratch home directory is still empty$`, selfUpdateHomeEmpty)
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if s := selfUpdateState; s != nil {
			if s.server != nil {
				s.server.Close()
			}
			_ = os.RemoveAll(s.dir)
			selfUpdateState = nil
		}
		return ctx, nil
	})
}

type selfUpdateScenario struct {
	dir, home, exe string
	before         string // sha256 of the installed binary before the verb
	release        []byte // the "deck" binary the fake release serves
	server         *httptest.Server
	url            string
	path           string // PATH for the verb: empty unless a fake gh was installed
	output         string
	code           int
}

// selfUpdateState is the one running scenario's state (scenarios run serially).
var selfUpdateState *selfUpdateScenario

func selfUpdateCurrent() (*selfUpdateScenario, error) {
	if selfUpdateState == nil {
		return nil, errors.New("no deck binary installed yet")
	}
	return selfUpdateState, nil
}

func selfUpdateInstallBinary(ctx context.Context, version string) error {
	root, err := filepath.Abs("..")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "deck-selfupdate-")
	if err != nil {
		return err
	}
	s := &selfUpdateScenario{dir: dir, home: filepath.Join(dir, "home"), exe: filepath.Join(dir, "bin", "deck")}
	selfUpdateState = s
	for _, d := range []string{s.home, filepath.Dir(s.exe)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if output, err := memoGoBuild(ctx, root, s.exe, "./cmd/deck", "-ldflags", "-X main.version="+version); err != nil {
		return fmt.Errorf("build deck %s: %w\n%s", version, err, output)
	}
	s.before, err = fileSHA(s.exe)
	return err
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func selfUpdateStartServer(latest string) error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	asset := "deck_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	// A tag's release binary is a real Go executable whose build info reports
	// that tag; each tag's archive and checksums.txt are built once.
	type served struct {
		archive   []byte
		checksums string
	}
	var mu sync.Mutex
	tags := map[string]served{}
	forTag := func(tag string) (served, error) {
		mu.Lock()
		defer mu.Unlock()
		if sv, ok := tags[tag]; ok {
			return sv, nil
		}
		body, err := testrelbin.Build(tag)
		if err != nil {
			return served{}, err
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(zw)
		if err := tw.WriteHeader(&tar.Header{Name: "deck", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			return served{}, err
		}
		if _, err := tw.Write(body); err != nil {
			return served{}, err
		}
		if err := errors.Join(tw.Close(), zw.Close()); err != nil {
			return served{}, err
		}
		sum := sha256.Sum256(buf.Bytes())
		sv := served{archive: buf.Bytes(), checksums: hex.EncodeToString(sum[:]) + "  " + asset + "\n"}
		tags[tag] = sv
		return sv, nil
	}
	if _, err := forTag(latest); err != nil {
		return err
	}
	body, err := testrelbin.Build(latest)
	if err != nil {
		return err
	}
	s.release = body
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+latest, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		tag, file, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/releases/download/"), "/")
		if !ok || tag == "" || (file != asset && file != "checksums.txt") {
			http.NotFound(w, r)
			return
		}
		sv, err := forTag(tag)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if file == asset {
			_, _ = w.Write(sv.archive)
		} else {
			_, _ = w.Write([]byte(sv.checksums))
		}
	})
	s.server = httptest.NewServer(mux)
	s.url = s.server.URL
	return nil
}

// selfUpdateVerifyingGh puts a fake authenticated gh, whose `attestation
// verify` passes, alone on the verb's PATH.
func selfUpdateVerifyingGh() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	s.path = filepath.Join(s.dir, "gh-path")
	if err := os.MkdirAll(s.path, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.path, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
}

func selfUpdateRun(command string) error { return selfUpdateRunEnv(command, "") }

// selfUpdateRunEnv runs the deck command with the hermetic environment plus
// one NAME=value assignment (none when env is empty).
func selfUpdateRunEnv(command, env string) error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "deck" {
		return fmt.Errorf("unsupported command %q", command)
	}
	path := s.path
	if path == "" {
		path = filepath.Join(s.dir, "empty-path")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(s.exe, fields[1:]...)
	cmd.Env = []string{"PATH=" + path, "HOME=" + s.home, "DECK_UPDATE_URL=" + s.url}
	if env != "" {
		cmd.Env = append(cmd.Env, env)
	}
	out, err := cmd.CombinedOutput()
	s.output = string(out)
	var exit *exec.ExitError
	switch {
	case err == nil:
		s.code = 0
	case errors.As(err, &exit):
		s.code = exit.ExitCode()
	default:
		return err
	}
	return nil
}

func selfUpdateExitsZero() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	if s.code != 0 {
		return fmt.Errorf("exit code %d, output:\n%s", s.code, s.output)
	}
	return nil
}

func selfUpdateExitsOne() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	if s.code != 1 {
		return fmt.Errorf("exit code %d, want 1, output:\n%s", s.code, s.output)
	}
	return nil
}

func selfUpdateOutputMentions3(a, b, c string) error { return selfUpdateOutputMentions(a, b, c) }

func selfUpdateOutputMentions2(a, b string) error { return selfUpdateOutputMentions(a, b) }

func selfUpdateOutputMentions(wants ...string) error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	for _, w := range wants {
		if !strings.Contains(s.output, w) {
			return fmt.Errorf("output %q does not mention %q", s.output, w)
		}
	}
	return nil
}

func selfUpdateBinaryIsRelease() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	got, err := fileSHA(s.exe)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(s.release)
	if want := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("installed binary sha256 %s, want the release's %s", got, want)
	}
	return nil
}

func selfUpdateBinaryUnchanged() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	got, err := fileSHA(s.exe)
	if err != nil {
		return err
	}
	if got != s.before {
		return fmt.Errorf("installed binary changed: sha256 %s, was %s", got, s.before)
	}
	entries, err := os.ReadDir(filepath.Dir(s.exe))
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("install directory holds %d entries, want only deck", len(entries))
	}
	return nil
}

func selfUpdateHomeEmpty() error {
	s, err := selfUpdateCurrent()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.home)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("--check wrote %d entries under the home directory", len(entries))
	}
	return nil
}
