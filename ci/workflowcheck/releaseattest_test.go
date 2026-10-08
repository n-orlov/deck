// releaseattest_test.go: R236. release.yml attests the release tarballs and
// checksums.txt with the GitHub artifact-attestation action, pinned by a
// 40-hex commit SHA with its tag in a trailing comment (R197), and the only
// permissions the job gains for it are id-token: write and attestations: write.
// The checks run against the shipped file and against mutated copies of it, so a
// regression in the checker itself shows up as a fixture that no longer fails.
package workflowcheck

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const attestAction = "actions/attest-build-provenance"

var (
	attestUsesLineRE = regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*` + regexp.QuoteMeta(attestAction) + `@(\S+)(.*)$`)
	pinnedSHARE      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	tagCommentRE     = regexp.MustCompile(`^\s+#\s*v\d+(\.\d+)*\s*$`)
)

// releaseWantPermissions is what the release job may hold: the three it had
// before R236 plus exactly the two the attestation needs.
var releaseWantPermissions = map[string]string{
	"contents":     "write",
	"checks":       "read",
	"actions":      "read",
	"id-token":     "write",
	"attestations": "write",
}

type attestWorkflow struct {
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
		Steps       []struct {
			Name string         `yaml:"name"`
			Uses string         `yaml:"uses"`
			With map[string]any `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// checkReleaseAttestation returns every way raw's release job fails R236.
func checkReleaseAttestation(raw []byte) []string {
	var problems []string
	var wf attestWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return []string{fmt.Sprintf("parse YAML: %v", err)}
	}
	matches := attestUsesLineRE.FindAllStringSubmatch(string(raw), -1)
	if len(matches) != 1 {
		return append(problems, fmt.Sprintf("want exactly one %s step, found %d", attestAction, len(matches)))
	}
	if sha, _, _ := strings.Cut(matches[0][1], "#"); !pinnedSHARE.MatchString(sha) {
		problems = append(problems, fmt.Sprintf("%s is not pinned by a 40-hex commit SHA: @%s", attestAction, sha))
	}
	if !tagCommentRE.MatchString(matches[0][2]) {
		problems = append(problems, fmt.Sprintf("%s pin lacks a trailing `# vX.Y.Z` tag comment: %q", attestAction, matches[0][2]))
	}
	job := wf.Jobs["release"]
	subjects := ""
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, attestAction+"@") {
			subjects, _ = step.With["subject-path"].(string)
		}
	}
	for _, want := range []string{"*.tar.gz", "checksums.txt"} {
		if !strings.Contains(subjects, want) {
			problems = append(problems, fmt.Sprintf("subject-path %q does not cover %s", subjects, want))
		}
	}
	effective := wf.Permissions
	if job.Permissions != nil {
		effective = job.Permissions
	}
	if !reflect.DeepEqual(effective, releaseWantPermissions) {
		problems = append(problems, fmt.Sprintf("release job permissions are %v, want exactly %v", effective, releaseWantPermissions))
	}
	return problems
}

func TestReleaseAttestsTarballsAndChecksumsWithPinnedActionAndTwoExtraPermissions(t *testing.T) {
	if problems := checkReleaseAttestation(readWorkflowFile(t, "release.yml")); len(problems) != 0 {
		t.Fatalf("release.yml: %s", strings.Join(problems, "; "))
	}
}

func TestReleaseAttestationCheckRejectsMutatedWorkflows(t *testing.T) {
	shipped := string(readWorkflowFile(t, "release.yml"))
	sha := regexp.MustCompile(`@[0-9a-f]{40} # v[0-9.]+`)
	cases := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"tag instead of SHA", func(s string) string { return sha.ReplaceAllString(s, "@v4") }, "40-hex"},
		{"short SHA", func(s string) string { return sha.ReplaceAllString(s, "@4d101475 # v4.2.2") }, "40-hex"},
		{"no tag comment", func(s string) string { return sha.ReplaceAllStringFunc(s, func(m string) string { return m[:41] }) }, "tag comment"},
		{"no attest step", func(s string) string { return strings.ReplaceAll(s, attestAction, "actions/other") }, "exactly one"},
		{"checksums not attested", func(s string) string { return strings.Replace(s, "dist/checksums.txt\n", "dist/nothing\n", 1) }, "checksums.txt"},
		{"tarballs not attested", func(s string) string { return strings.Replace(s, "dist/*.tar.gz\n", "dist/nothing\n", 1) }, "tar.gz"},
		{"id-token missing", func(s string) string { return strings.Replace(s, "  id-token: write\n", "", 1) }, "permissions"},
		{"attestations missing", func(s string) string { return strings.Replace(s, "  attestations: write\n", "", 1) }, "permissions"},
		{"top-level permission widened", func(s string) string {
			return strings.Replace(s, "  attestations: write\n", "  attestations: write\n  packages: write\n", 1)
		}, "permissions"},
		{"checks permission widened", func(s string) string { return strings.Replace(s, "  checks: read\n", "  checks: write\n", 1) }, "permissions"},
		{"job-level permissions widen", func(s string) string {
			return strings.Replace(s, "    runs-on: ubuntu-latest\n", "    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n      checks: read\n      actions: read\n      id-token: write\n      attestations: write\n      pull-requests: write\n", 1)
		}, "permissions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.mutate(shipped)
			if mutated == shipped {
				t.Fatal("mutation changed nothing: the fixture no longer matches release.yml")
			}
			problems := strings.Join(checkReleaseAttestation([]byte(mutated)), "; ")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("checker reported %q, want a problem mentioning %q", problems, tc.want)
			}
		})
	}
}
