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

	"github.com/cucumber/godog"
)

// registerSelfUpdateSteps backs features/self_update.feature (R296). The
// scenario builds its own deck with a release version (a plain build is `dev`
// and refuses to update), runs it with a hermetic environment and an empty
// PATH, and points it at a loopback fake release server.
func registerSelfUpdateSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a deck binary at version "([^"]+)" installed in a scratch directory$`, selfUpdateInstallBinary)
	sc.Step(`^a fake release server whose latest release is "([^"]+)"$`, selfUpdateStartServer)
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

const selfUpdateReleaseBody = "the release's deck binary\n"

type selfUpdateScenario struct {
	dir, home, exe string
	before         string // sha256 of the installed binary before the verb
	server         *httptest.Server
	url            string
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
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	body := []byte(selfUpdateReleaseBody)
	if err := tw.WriteHeader(&tar.Header{Name: "deck", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := tw.Write(body); err != nil {
		return err
	}
	if err := errors.Join(tw.Close(), zw.Close()); err != nil {
		return err
	}
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+latest, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+asset):
			_, _ = w.Write(archive)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	})
	s.server = httptest.NewServer(mux)
	s.url = s.server.URL
	return nil
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
	empty := filepath.Join(s.dir, "empty-path")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(s.exe, fields[1:]...)
	cmd.Env = []string{"PATH=" + empty, "HOME=" + s.home, "DECK_UPDATE_URL=" + s.url}
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
	sum := sha256.Sum256([]byte(selfUpdateReleaseBody))
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
