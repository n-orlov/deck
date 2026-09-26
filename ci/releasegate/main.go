// Command releasegate is release.yml's R147 gate: it queries
// commits/<sha>/check-runs for the "suite" check ci.yml publishes and
// refuses (non-zero exit, message naming the sha and what it found)
// unless that check's most recently started run completed with
// conclusion "success". release.yml runs this before it builds anything,
// so a release is never cut, built or published for a commit whose CI
// suite did not go green.
//
//	go run ./ci/releasegate -repo owner/name -sha <sha> [-check suite]
//
// The check name defaults to "suite" -- ci.yml's suite job has no
// explicit `name:` override beyond its own job id, so GitHub reports its
// check run under that same name. Reads the GitHub token from
// GITHUB_TOKEN (falling back to GH_TOKEN, the gh CLI's own convention);
// an empty token still works against a public repository's check-runs
// endpoint, just at the lower unauthenticated rate limit.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

// checkRun is the subset of a GitHub check-run object this gate cares about.
type checkRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	StartedAt  string `json:"started_at"`
}

type checkRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []checkRun `json:"check_runs"`
}

// evaluate inspects a commits/<sha>/check-runs API response body for the
// named check and returns nil only when its most recently started run
// completed with conclusion "success". Every other case -- no matching
// run at all, a run still queued/in_progress, or a completed run whose
// conclusion is not "success" (failure, cancelled, timed_out, ...) --
// returns an error naming the sha and exactly what was found, so the
// caller's own message (this gate exits non-zero and prints err) already
// satisfies criterion (1) without any extra formatting at the call site.
func evaluate(body []byte, sha, checkName string) error {
	var resp checkRunsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("commit %s: could not parse check-runs response: %w", sha, err)
	}

	var latest *checkRun
	for i := range resp.CheckRuns {
		cr := &resp.CheckRuns[i]
		if cr.Name != checkName {
			continue
		}
		if latest == nil || cr.StartedAt > latest.StartedAt {
			latest = cr
		}
	}

	if latest == nil {
		return fmt.Errorf("commit %s: no %q check run found (saw %d check run(s) total)", sha, checkName, len(resp.CheckRuns))
	}
	if latest.Status != "completed" {
		return fmt.Errorf("commit %s: %q check run is %s, not completed", sha, checkName, latest.Status)
	}
	if latest.Conclusion != "success" {
		return fmt.Errorf("commit %s: %q check run concluded %s, not success", sha, checkName, latest.Conclusion)
	}
	return nil
}

// fetchCheckRuns performs the actual GitHub API call; kept separate from
// evaluate so the unit tests exercise evaluate directly against canned
// JSON, never a live network call.
func fetchCheckRuns(repo, sha, token string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s/check-runs", repo, sha)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check-runs request failed: %s: %s", resp.Status, string(body))
	}
	return body, nil
}

func run(args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("releasegate", flag.ContinueOnError)
	repo := fs.String("repo", "", "owner/repo (required)")
	sha := fs.String("sha", "", "commit sha to check (required)")
	check := fs.String("check", "suite", "check run name to require (must match ci.yml's job name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *sha == "" {
		return errors.New("releasegate: -repo and -sha are required")
	}

	token := getenv("GITHUB_TOKEN")
	if token == "" {
		token = getenv("GH_TOKEN")
	}

	body, err := fetchCheckRuns(*repo, *sha, token)
	if err != nil {
		return err
	}
	return evaluate(body, *sha, *check)
}

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
