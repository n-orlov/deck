// jenkinsfile_test.go probes the Jenkinsfile (deck/ci, deck's CI since the
// move off GitHub Actions) and Jenkinsfile.release (deck/release): the CI
// stages run in the documented order, the extended lane is scheduled on main,
// the quality gates and reports stay wired in, only a fully green main build
// leaves a release candidate, and the release job publishes exactly that
// commit without pushing a tag. Plain text matching; both files are small and
// declarative.
package repocheck

import (
	"regexp"
	"strings"
	"testing"
)

var stageRE = regexp.MustCompile(`stage\('([^']+)'\)\s*\{`)

// jenkinsfileLines returns a Jenkinsfile's lines with // comment lines
// and blank lines dropped.
func jenkinsfileLines(jf string) []string {
	var out []string
	for _, l := range strings.Split(jf, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "//") {
			out = append(out, l)
		}
	}
	return out
}

// jenkinsStages returns the stage names in the order they appear.
func jenkinsStages(jf string) []string {
	var names []string
	for _, m := range stageRE.FindAllStringSubmatch(jf, -1) {
		names = append(names, m[1])
	}
	return names
}

// jenkinsStage returns the body of stage(name), up to its matching brace.
func jenkinsStage(jf, name string) (string, bool) {
	for _, loc := range stageRE.FindAllStringSubmatchIndex(jf, -1) {
		if jf[loc[2]:loc[3]] != name {
			continue
		}
		depth := 0
		for i := loc[1] - 1; i < len(jf); i++ {
			switch jf[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return jf[loc[1]:i], true
				}
			}
		}
	}
	return "", false
}

func stageBefore(jf, first, second string) bool {
	fi, si := -1, -1
	for i, n := range jenkinsStages(jf) {
		if n == first && fi < 0 {
			fi = i
		}
		if n == second && si < 0 {
			si = i
		}
	}
	return fi >= 0 && si >= 0 && fi < si
}

func TestJenkinsfileStagesRunInOrder(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile")
	want := []string{"CI image", "Lint", "Suite", "Allure smoke", "Quality", "Stability"}
	if got := jenkinsStages(jf); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("stages = %q, want %q", got, want)
	}
	image, _ := jenkinsStage(jf, "CI image")
	if !strings.Contains(image, "sh 'ci/jenkins-image.sh'") {
		t.Error("the CI image stage does not run ci/jenkins-image.sh")
	}
	if !strings.Contains(readRepoFile(t, "ci", "jenkins-image.sh"), "docker build -t deck-ci:local -f ci/Dockerfile ci") {
		t.Error("ci/jenkins-image.sh does not build deck-ci:local from ci/Dockerfile")
	}
	suite, _ := jenkinsStage(jf, "Suite")
	if !strings.Contains(suite, "ci/run.sh ci/suite.sh") {
		t.Error("the Suite stage does not run ci/run.sh ci/suite.sh")
	}
}

// TestJenkinsfileNightlyLane: main is built nightly at 03:17, and a nightly
// (or EXTENDED) build adds -race, drops the vulnerability databases and runs
// three stability runs.
func TestJenkinsfileNightlyLane(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile")
	if !strings.Contains(jf, `cron(env.BRANCH_NAME == 'main' ? '17 3 * * *' : '')`) {
		t.Error("the Jenkinsfile does not schedule main nightly at 17 3 * * *")
	}
	if !strings.Contains(jf, "TimerTrigger$TimerTriggerCause") || !strings.Contains(jf, "params.EXTENDED") {
		t.Error("the extended lane is not selected from the timer cause and the EXTENDED parameter")
	}
	if !strings.Contains(jf, "booleanParam(name: 'EXTENDED', defaultValue: false") {
		t.Error("the EXTENDED parameter is missing or not off by default")
	}
	if !strings.Contains(jf, "env.DECK_CI_GO_EXTRA_FLAGS = nightly ? '-race' : ''") {
		t.Error("a nightly build does not set DECK_CI_GO_EXTRA_FLAGS=-race")
	}
	quality, _ := jenkinsStage(jf, "Quality")
	if !strings.Contains(quality, "rm -rf /go-cache/trivy/db /go-cache/trivy/java-db /go-cache/xdg-cache/govulncheck") {
		t.Error("the nightly lane no longer drops the cached vulnerability databases before the quality gates")
	}
	stability, _ := jenkinsStage(jf, "Stability")
	if !strings.Contains(stability, "environment name: 'CI_NIGHTLY', value: 'true'") || !strings.Contains(stability, "ci/stability.sh 3") {
		t.Error("the Stability stage does not run ci/stability.sh 3 on nightly builds only")
	}
}

func TestJenkinsfileRunsTheQualityGates(t *testing.T) {
	quality, ok := jenkinsStage(readRepoFile(t, "Jenkinsfile"), "Quality")
	if !ok {
		t.Fatal("the Jenkinsfile has no Quality stage")
	}
	if !strings.Contains(quality, `ci/run.sh ci/quality.sh "$DECK_CI_OUT"`) {
		t.Error(`the Quality stage does not run ci/run.sh ci/quality.sh "$DECK_CI_OUT"`)
	}
	if !strings.Contains(quality, "catchError(buildResult: 'FAILURE'") {
		t.Error("a failed quality gate no longer fails the build")
	}
}

// TestJenkinsfileNeverReleases: deck/ci builds branches only; releasing is
// deck/release's job. A main build that ends SUCCESS (post success {}, which
// runs after always {}, so an UNSTABLE build never qualifies) archives the
// release candidate, and deck/release may copy it.
func TestJenkinsfileNeverReleases(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile")
	for _, l := range jenkinsfileLines(jf) {
		for _, bad := range []string{"TAG_NAME", "tag pattern", "gh release", "release-build.sh", "git push"} {
			if strings.Contains(l, bad) {
				t.Errorf("Jenkinsfile line %q mentions %q: deck/ci must not release", l, bad)
			}
		}
	}
	_, post, ok := strings.Cut(jf, "\n  post {")
	if !ok {
		t.Fatal("Jenkinsfile has no pipeline-level post { } block")
	}
	_, success, ok := strings.Cut(post, "\n    success {")
	if !ok {
		t.Fatal("the release candidate is not written in post { success { } }")
	}
	for _, want := range []string{
		"env.BRANCH_NAME == 'main'",
		"writeFile file: 'ci-results/release-candidate.json'",
		`'{"commit": "' + env.GIT_COMMIT`,
		`"build_url": "' + env.BUILD_URL`,
		`"build_number": ' + env.BUILD_NUMBER`,
		"archiveArtifacts artifacts: 'ci-results/release-candidate.json'",
	} {
		if !strings.Contains(success, want) {
			t.Errorf("post { success } lacks %q", want)
		}
	}
	if !strings.Contains(jf, "copyArtifactPermission('/deck/release,/deck/ci/*')") {
		t.Error("deck/release (candidate) and deck/ci (Allure history) are not both allowed to copy artifacts")
	}
}

// TestJenkinsfileReleasePublishesTheCandidate: deck/release takes the commit
// from a green deck/ci main build's release candidate, refuses an existing
// tag, builds through ci/release-build.sh and lets gh create the tag at that
// commit; it never pushes a tag itself.
func TestJenkinsfileReleasePublishesTheCandidate(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile.release")
	want := []string{"Candidate", "Checkout", "Version check", "Build", "Publish"}
	if got := jenkinsStages(jf); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("release stages = %q, want %q", got, want)
	}
	checks := map[string][]string{
		"Candidate": {
			`params.VERSION ==~ /^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/`,
			"projectName: '/deck/ci/main', filter: 'ci-results/release-candidate.json'",
			"specific(params.CI_BUILD) : lastSuccessful(stable: true)",
			`env.RELEASE_SHA ==~ /^[0-9a-f]{40}$/`,
		},
		"Checkout": {"checkout scm", `git checkout --quiet --detach "$RELEASE_SHA"`},
		"Version check": {
			"export GIT_TERMINAL_PROMPT=0",
			"git ls-remote --exit-code --tags",
			"already exists on GitHub",
		},
		"Build":   {"sh 'ci/jenkins-image.sh'", `ci/run.sh ci/release-build.sh "$VERSION"`},
		"Publish": {"!params.DRY_RUN", `gh release create "$VERSION" dist/* --repo n-orlov/deck --target "$RELEASE_SHA"`, "--generate-notes"},
	}
	for stage, wants := range checks {
		body, ok := jenkinsStage(jf, stage)
		if !ok {
			t.Errorf("Jenkinsfile.release has no %s stage", stage)
			continue
		}
		for _, w := range wants {
			if !strings.Contains(body, w) {
				t.Errorf("the %s stage lacks %q", stage, w)
			}
		}
	}
	for _, l := range jenkinsfileLines(jf) {
		if strings.Contains(l, "git push") || strings.Contains(l, "git tag") {
			t.Errorf("Jenkinsfile.release line %q creates or pushes a tag itself; gh release create --target does that", l)
		}
		if strings.Contains(l, "echo") && strings.Contains(l, "GH_TOKEN") {
			t.Errorf("Jenkinsfile.release line %q echoes the token", l)
		}
	}
	if !strings.Contains(readRepoFile(t, "ci", "release-build.sh"), `want="deck ${version} linux/amd64"`) {
		t.Error("ci/release-build.sh no longer smoke-checks deck --version")
	}
}

// TestJenkinsfilePublishesReports: test results, the Allure report and the
// CI summary are on every build's page, red or green.
func TestJenkinsfilePublishesReports(t *testing.T) {
	jf := readRepoFile(t, "Jenkinsfile")
	_, post, ok := strings.Cut(jf, "\n  post {")
	if !ok {
		t.Fatal("Jenkinsfile has no pipeline-level post { } block")
	}
	for _, want := range []string{
		"junit testResults: 'ci-results/junit-merged/*.xml'",
		"ci/jenkins-report.sh",
		"reportName: 'Allure'",
		"reportName: 'CI summary'",
		"addSummary(",
	} {
		if !strings.Contains(post, want) {
			t.Errorf("post { always } lacks %q", want)
		}
	}
}
