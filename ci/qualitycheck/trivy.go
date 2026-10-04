// R190's trivy gate: `trivy fs` over the repo with vuln, secret and
// misconfig scanners at HIGH,CRITICAL, --ignore-unfixed, its database
// cache under the existing /go-cache volume. Exceptions live in
// .trivyignore only: one entry per line, each with a reason and a
// review-by date, and a date that has passed fails the gate (an
// exception must be reviewed, never silently outlive its reason).
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// trivyConfig is the "trivy" section of ci/quality.json. Severity is a
// comma-separated list; it must name at least HIGH and CRITICAL (the
// gate refuses a list that leaves either out, so the severity cannot be
// loosened by editing the config).
type trivyConfig struct {
	Enabled  bool   `json:"enabled"`
	Severity string `json:"severity"`
}

// trivySkipFiles are skipped by exact path (relative to the scan
// target), each with the reason it is skipped: Trivy would otherwise
// read it whole into memory and warn about its size. It is a recorded
// stability-run log, not source, so there is nothing in it to scan.
var trivySkipFiles = []string{
	// 11 MB stability-run summary log under docs/reports; the only file
	// over Trivy's secret-scanner size warning.
	"docs/reports/phase3g-812-stability10/summary.log",
}

// trivySkipDirs are skipped by exact path (relative to the scan target):
// ci-results/ is ci/suite.sh's own output directory (raw test logs, the
// Allure results), generated during the run and gitignored -- not repo
// content, and scanning it would make the gate judge its own run's logs.
var trivySkipDirs = []string{"ci-results"}

// trivyOptions is everything one gate run needs; tests build their own
// (a fake binary, an injected clock) without touching the process env.
type trivyOptions struct {
	Binary     string // trivy executable (default "trivy")
	Target     string // directory to scan
	CacheDir   string // --cache-dir (under /go-cache in the image)
	IgnoreFile string // .trivyignore path; may be absent
	Severity   string
	Now        time.Time
}

// reviewByRe finds the review-by token of a .trivyignore entry.
var reviewByRe = regexp.MustCompile(`(?:^|\s)review-by:(\d{4}-\d{2}-\d{2})(?:\s|$)`)

var validTrivySeverities = map[string]bool{"UNKNOWN": true, "LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

// validateTrivySeverity checks that severity names only known levels and
// includes both HIGH and CRITICAL.
func validateTrivySeverity(severity string) error {
	have := map[string]bool{}
	for _, s := range strings.Split(severity, ",") {
		s = strings.TrimSpace(s)
		if !validTrivySeverities[s] {
			return fmt.Errorf("trivy.severity %q: %q is not a Trivy severity", severity, s)
		}
		have[s] = true
	}
	for _, need := range []string{"HIGH", "CRITICAL"} {
		if !have[need] {
			return fmt.Errorf("trivy.severity %q must include %s", severity, need)
		}
	}
	return nil
}

// checkTrivyIgnore reads the .trivyignore at path (absent is fine: no
// exceptions) and returns one problem per bad line. A comment-only or
// blank line is allowed; every other line must be
//
//	<ID> review-by:YYYY-MM-DD # <reason>
//
// with a non-empty reason, a parseable date, and a date not before now
// (the date itself is the last good day).
func checkTrivyIgnore(path string, now time.Time) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	today := now.UTC().Format("2006-01-02")
	var problems []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, reason, hasReason := strings.Cut(line, "#")
		if !hasReason || strings.TrimSpace(reason) == "" {
			problems = append(problems, fmt.Sprintf("%s:%d: %q has no reason (want \"<ID> review-by:YYYY-MM-DD # <reason>\")", path, n, line))
			continue
		}
		m := reviewByRe.FindStringSubmatch(entry)
		if m == nil {
			problems = append(problems, fmt.Sprintf("%s:%d: %q has no review-by:YYYY-MM-DD date", path, n, line))
			continue
		}
		if _, perr := time.Parse("2006-01-02", m[1]); perr != nil {
			problems = append(problems, fmt.Sprintf("%s:%d: review-by date %q is not a real date", path, n, m[1]))
			continue
		}
		if m[1] < today {
			problems = append(problems, fmt.Sprintf("%s:%d: %q expired on %s -- fix the finding or review the exception and move the date", path, n, strings.Fields(entry)[0], m[1]))
		}
	}
	return problems, sc.Err()
}

// scanTargetHasFiles reports whether dir holds at least one regular
// file and a go.mod, so an empty or wrongly pointed target can never
// read as a clean scan (Trivy itself exits 0 on an empty directory).
func scanTargetHasFiles(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("scan target %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan target %q is not a directory", dir)
	}
	files := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			files++
		}
		return nil
	})
	if files == 0 {
		return fmt.Errorf("scan target %q is empty (no files to scan)", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return fmt.Errorf("scan target %q has no go.mod: a scan of the wrong tree would pass vacuously", dir)
	}
	return nil
}

// trivyArgs builds the trivy command line.
func trivyArgs(o trivyOptions) []string {
	args := []string{"fs",
		"--scanners", "vuln,secret,misconfig",
		"--severity", o.Severity,
		"--ignore-unfixed",
		"--exit-code", "1",
		"--cache-dir", o.CacheDir,
		"--skip-version-check",
	}
	for _, f := range trivySkipFiles {
		args = append(args, "--skip-files", f)
	}
	for _, d := range trivySkipDirs {
		args = append(args, "--skip-dirs", d)
	}
	if _, err := os.Stat(o.IgnoreFile); err == nil {
		args = append(args, "--ignorefile", o.IgnoreFile)
	}
	return append(args, o.Target)
}

// runTrivyGate runs the gate. ok=false with a nil error is a finding (or
// an expired/malformed exception, or an empty target); a non-nil error
// is an input/tooling failure (exit 2): trivy missing, or exiting with
// anything but 0 or 1.
func runTrivyGate(o trivyOptions) (ok bool, output string, err error) {
	if err := validateTrivySeverity(o.Severity); err != nil {
		return false, "", err
	}
	if o.CacheDir == "" {
		return false, "", fmt.Errorf("trivy gate: no cache dir")
	}
	if terr := scanTargetHasFiles(o.Target); terr != nil {
		return false, "trivy gate: " + terr.Error() + "\nwhat to do: point the gate at the repository root.\n", nil
	}
	problems, perr := checkTrivyIgnore(o.IgnoreFile, o.Now)
	if perr != nil {
		return false, "", fmt.Errorf("reading %s: %w", o.IgnoreFile, perr)
	}
	if len(problems) > 0 {
		return false, "trivy gate: .trivyignore is not acceptable:\n" + strings.Join(problems, "\n") +
			"\nwhat to do: every entry needs \"<ID> review-by:YYYY-MM-DD # <reason>\" with a date not yet passed; " +
			"remove the entry once the finding is fixed.\n", nil
	}

	bin := o.Binary
	if bin == "" {
		bin = "trivy"
	}
	cmd := exec.Command(bin, trivyArgs(o)...)
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	if runErr == nil {
		return true, text + "what to do: nothing -- no unfixed-excluded HIGH/CRITICAL vulnerability, secret or misconfiguration.\n", nil
	}
	if ee, isExit := runErr.(*exec.ExitError); isExit && ee.ExitCode() == 1 {
		return false, text + "what to do: bump the named module to its fixed version (go get <module>@<fixed> && go mod tidy), " +
			"remove the secret or misconfiguration, or -- only if neither is possible -- add a dated, reasoned .trivyignore entry.\n", nil
	}
	return false, text, fmt.Errorf("running %s: %w", bin, runErr)
}
