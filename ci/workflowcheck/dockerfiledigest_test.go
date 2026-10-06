// dockerfiledigest_test.go is R214's probe (#71 item 7): every external base
// image in ci/Dockerfile is pinned by sha256 digest, so a re-pushed tag can
// never change what the CI image is built from.
package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var digestRE = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// undigestedFroms returns each FROM line of a Dockerfile whose image is
// neither pinned by digest, `scratch`, nor an earlier stage's name.
func undigestedFroms(dockerfile string) []string {
	var bad []string
	stages := map[string]bool{"scratch": true}
	for _, line := range strings.Split(dockerfile, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		args := fields[1:]
		for len(args) > 0 && strings.HasPrefix(args[0], "--") {
			args = args[1:] // --platform=...
		}
		if len(args) == 0 {
			bad = append(bad, strings.TrimSpace(line))
			continue
		}
		image := args[0]
		if !stages[strings.ToLower(image)] && !digestRE.MatchString(image) {
			bad = append(bad, strings.TrimSpace(line))
		}
		if len(args) >= 3 && strings.EqualFold(args[1], "AS") {
			stages[strings.ToLower(args[2])] = true
		}
	}
	return bad
}

func TestUndigestedFromsFlagsAFromWithoutADigest(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	cases := []struct {
		name, dockerfile string
		want             int
	}{
		{"tag only", "FROM golang:1.25.14-trixie\n", 1},
		{"tag only with stage name", "FROM golang:1.26.4-trixie AS tools\n", 1},
		{"untagged", "FROM golang\n", 1},
		{"short digest", "FROM golang:1@sha256:abc\n", 1},
		{"platform flag does not hide it", "FROM --platform=linux/amd64 golang:1\n", 1},
		{"lowercase instruction", "from golang:1\n", 1},
		{"one digested one not", "FROM golang:1" + digest + " AS a\nFROM golang:2\n", 1},
		{"digested", "FROM golang:1" + digest + "\n", 0},
		{"digested with stage name", "FROM golang:1" + digest + " AS tools\n", 0},
		{"digest without tag", "FROM golang" + digest + "\n", 0},
		{"scratch and earlier stage", "FROM golang:1" + digest + " AS a\nFROM a\nFROM scratch\n", 0},
		{"COPY --from is not a FROM line", "FROM golang:1" + digest + "\nCOPY --from=x /a /b\n", 0},
	}
	for _, tc := range cases {
		if got := undigestedFroms(tc.dockerfile); len(got) != tc.want {
			t.Errorf("%s: flagged %q, want %d line(s)", tc.name, got, tc.want)
		}
	}
}

func TestCIDockerfilePinsEveryBaseImageByDigest(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "ci", "Dockerfile"))
	if err != nil {
		t.Fatalf("read ci/Dockerfile: %v", err)
	}
	if bad := undigestedFroms(string(raw)); len(bad) > 0 {
		t.Fatalf("ci/Dockerfile FROM lines without an @sha256 digest: %q", bad)
	}
	if n := len(regexp.MustCompile(`(?m)^FROM `).FindAllString(string(raw), -1)); n != 2 {
		t.Fatalf("ci/Dockerfile has %d FROM lines, want the 2 the probe was written for", n)
	}
}
