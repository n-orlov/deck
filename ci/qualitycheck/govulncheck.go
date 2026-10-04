// R190's govulncheck gate: `govulncheck ./...` over the module, with
// GOTOOLCHAIN=local, failing on a vulnerability whose vulnerable code the
// program actually calls (govulncheck's default source mode only exits
// non-zero for those; a vulnerable module that is merely required or
// imported is reported informationally and does not fail the gate).
//
// Two guards keep the gate from passing vacuously:
//
//   - the running `go version` must not be older than go.mod's toolchain
//     line (older than that, the standard library vulnerabilities fixed
//     by the toolchain the module asks for would be reported against a
//     stdlib that is not the one the product ships with, or missed);
//   - the scan target must be a Go module with Go files in it, because
//     govulncheck on a wrong or empty tree has nothing to find.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// govulncheckConfig is the "govulncheck" section of ci/quality.json.
type govulncheckConfig struct {
	Enabled bool `json:"enabled"`
}

// govulncheckOptions is everything one gate run needs.
type govulncheckOptions struct {
	Binary   string // govulncheck executable (default "govulncheck")
	GoBinary string // go executable used for the version check (default "go")
	Target   string // module root to scan
	DB       string // -db override (a file:// URL in tests); empty = the default vuln.go.dev
	CacheDir string // XDG_CACHE_HOME for the run, so the vuln DB cache lands on a writable volume; empty = inherit
}

var goVersionOutputRe = regexp.MustCompile(`\bgo version (go[0-9][^\s]*)`)

// goModMinimum returns the toolchain line of dir/go.mod, falling back to
// the go line when there is no toolchain line, and an error when neither
// exists or the one found is not a valid Go version.
func goModMinimum(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod")) //nolint:gosec // G304: dir is the module directory the gate was configured with; only its go.mod is read
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }() // read-only handle: Close cannot lose data
	var toolchain, goLine string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "toolchain":
			toolchain = fields[1]
		case "go":
			goLine = "go" + strings.TrimPrefix(fields[1], "go")
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	minimum := toolchain
	if minimum == "" {
		minimum = goLine
	}
	if minimum == "" {
		return "", fmt.Errorf("%s/go.mod has neither a toolchain nor a go line", dir)
	}
	if !version.IsValid(minimum) {
		return "", fmt.Errorf("%s/go.mod: %q is not a valid Go version", dir, minimum)
	}
	return minimum, nil
}

// checkGoNotOlderThanToolchain runs `go version` (GOTOOLCHAIN=local, so
// the go command cannot swap itself for another toolchain) and returns a
// problem string when that go is older than go.mod's toolchain line.
func checkGoNotOlderThanToolchain(o govulncheckOptions) (problem string, err error) {
	minimum, err := goModMinimum(o.Target)
	if err != nil {
		return "", err
	}
	goBin := o.GoBinary
	if goBin == "" {
		goBin = "go"
	}
	cmd := exec.Command(goBin, "version") //nolint:gosec // G204: GoBinary is the operator's toolchain override from the gate's own options, default go
	cmd.Dir = o.Target
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running %s version: %w", goBin, err)
	}
	m := goVersionOutputRe.FindStringSubmatch(string(out))
	if m == nil || !version.IsValid(m[1]) {
		return "", fmt.Errorf("cannot read a Go version from %q", strings.TrimSpace(string(out)))
	}
	if version.Compare(m[1], minimum) < 0 {
		return fmt.Sprintf("the running Go is %s, older than go.mod's toolchain line %s", m[1], minimum), nil
	}
	return "", nil
}

// scanTargetHasGoCode reports an error unless dir holds a go.mod and at
// least one .go file (vendor/ and hidden or testdata directories do not
// count), so an empty or wrongly pointed target can never read as clean.
func scanTargetHasGoCode(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("scan target %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan target %q is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return fmt.Errorf("scan target %q has no go.mod", dir)
	}
	goFiles := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && p != dir {
			if n := d.Name(); n == "vendor" || n == "testdata" || strings.HasPrefix(n, ".") {
				return filepath.SkipDir
			}
		}
		if d.Type().IsRegular() && strings.HasSuffix(p, ".go") {
			goFiles++
		}
		return nil
	})
	if goFiles == 0 {
		return fmt.Errorf("scan target %q has no Go files to scan", dir)
	}
	return nil
}

// runGovulncheckGate runs the gate. ok=false with a nil error is a gate
// failure (called vulnerability, old Go, or empty target); a non-nil
// error is a tooling failure (exit 2): govulncheck missing, or exiting
// with anything but 0 or 3.
func runGovulncheckGate(o govulncheckOptions) (ok bool, output string, err error) {
	if terr := scanTargetHasGoCode(o.Target); terr != nil {
		return false, "govulncheck gate: " + terr.Error() +
			"\nwhat to do: point the gate at the repository root.\n", nil
	}
	problem, verr := checkGoNotOlderThanToolchain(o)
	if verr != nil {
		return false, "", fmt.Errorf("govulncheck gate: %w", verr)
	}
	if problem != "" {
		return false, "govulncheck gate: " + problem +
			"\nwhat to do: run the gate from an image whose Go is at least go.mod's toolchain line " +
			"(the deck-ci image), or lower the toolchain line only if the product no longer needs it.\n", nil
	}

	bin := o.Binary
	if bin == "" {
		bin = "govulncheck"
	}
	args := []string{}
	if o.DB != "" {
		args = append(args, "-db", o.DB)
	}
	args = append(args, "./...")
	cmd := exec.Command(bin, args...) //nolint:gosec // G204: the govulncheck binary and database are the operator's gate options, never external input
	cmd.Dir = o.Target
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if o.CacheDir != "" {
		cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+o.CacheDir)
	}
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	if runErr == nil {
		return true, text + "what to do: nothing -- no vulnerability is reachable from the code this module calls.\n", nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) && ee.ExitCode() == 3 {
		return false, text + "what to do: bump the named module (or the toolchain, for a standard-library finding) to the " +
			"\"Fixed in\" version (go get <module>@<fixed> && go mod tidy), or remove the call.\n", nil
	}
	return false, text, fmt.Errorf("running %s: %w", bin, runErr)
}
