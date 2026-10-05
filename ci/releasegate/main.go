// Command releasegate is release.yml's R147 gate: it queries
// commits/<sha>/check-runs for the "suite" check ci.yml publishes, and
// actions/runs?head_sha=<sha> to learn which workflow run (and hence
// which triggering event) each check run's check_suite belongs to. Per
// SPEC §13.2/R147, only suite runs whose triggering workflow event is
// "push" or "pull_request" ever gate a release: a nightly "schedule" or
// manual "workflow_dispatch" run may alert on red, but it never blocks a
// release for a sha whose own push (or PR) suite run went green, and a
// green schedule/workflow_dispatch run never substitutes for a missing
// push/PR one. Among the gating (push/pull_request) suite runs, the most
// recently started one decides. release.yml runs this before it builds
// anything, so a release is never cut, built or published for a commit
// whose CI suite did not go green on a push or pull request.
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
	"strings"
)

// githubAPIBase is the root of every GitHub API URL this gate builds.
// It is a var, never a const, purely so the HTTP-level tests below can
// point it at an httptest.Server instead of the real api.github.com --
// production always leaves it at its zero-value default.
var githubAPIBase = "https://api.github.com"

// checkRun is the subset of a GitHub check-run object this gate cares
// about. CheckSuite.ID is how a check run is tied back to the workflow
// run (and hence the triggering event) that produced it: a check-runs
// response embeds the parent check_suite's id but never its triggering
// event, so that link has to be resolved separately via actionsRun below.
type checkRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	StartedAt  string `json:"started_at"`
	CheckSuite struct {
		ID int64 `json:"id"`
	} `json:"check_suite"`
}

type checkRunsResponse struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []checkRun `json:"check_runs"`
}

// actionsRun is the subset of a GitHub Actions workflow run object this
// gate cares about. Event is exactly what evaluate needs and the
// check-runs endpoint never reports: "push", "pull_request", "schedule",
// "workflow_dispatch", etc. CheckSuiteID links it to checkRun.CheckSuite.ID.
type actionsRun struct {
	Event        string `json:"event"`
	CheckSuiteID int64  `json:"check_suite_id"`
}

type actionsRunsResponse struct {
	WorkflowRuns []actionsRun `json:"workflow_runs"`
}

// gatingEvents are the only workflow run events whose suite check ever
// gates a release (SPEC §13.2, R147): a push or a pull request. A
// nightly "schedule" run and a manual "workflow_dispatch" run alert on
// red but never gate -- whatever their own conclusion, they are simply
// excluded from consideration below.
var gatingEvents = map[string]bool{
	"push":         true,
	"pull_request": true,
}

// evaluate inspects a commits/<sha>/check-runs API response body
// (checkRunsBody) together with an actions/runs?head_sha=<sha> API
// response body (actionsRunsBody) for the named check, and returns nil
// only when the most recently started run of that check *whose workflow
// run event is "push" or "pull_request"* completed with conclusion
// "success". A schedule or workflow_dispatch run of the same check is
// never consulted, whatever its own conclusion -- it neither blocks nor
// substitutes for a missing push/PR run. Every other case -- no gating
// run at all (whether because there is no run of this check at all, or
// because every run of it belongs to a non-gating event), a gating run
// still queued/in_progress, or a completed gating run whose conclusion is
// not "success" (failure, cancelled, timed_out, ...) -- returns an error
// naming the sha and exactly what was found, so the caller's own message
// (this gate exits non-zero and prints err) already satisfies criterion
// (1) without any extra formatting at the call site.
func evaluate(checkRunsBody, actionsRunsBody []byte, sha, checkName string) error {
	var resp checkRunsResponse
	if err := json.Unmarshal(checkRunsBody, &resp); err != nil {
		return fmt.Errorf("commit %s: could not parse check-runs response: %w", sha, err)
	}

	var runs actionsRunsResponse
	if err := json.Unmarshal(actionsRunsBody, &runs); err != nil {
		return fmt.Errorf("commit %s: could not parse actions-runs response: %w", sha, err)
	}

	latest, namedTotal, nonGating := latestGatingCheckRun(resp.CheckRuns, gatingSuiteIDs(runs.WorkflowRuns), checkName)
	if latest == nil {
		if namedTotal > 0 && nonGating == namedTotal {
			return fmt.Errorf("commit %s: found %d %q check run(s), but all %d belong to a non-gating (schedule/workflow_dispatch) workflow run, not a push or pull_request one", sha, namedTotal, checkName, nonGating)
		}
		return fmt.Errorf("commit %s: no %q check run from a push or pull_request workflow run found (saw %d %q check run(s) total, %d non-gating)", sha, checkName, namedTotal, checkName, nonGating)
	}
	if latest.Status != "completed" {
		return fmt.Errorf("commit %s: %q check run (push/pull_request) is %s, not completed", sha, checkName, latest.Status)
	}
	if latest.Conclusion != "success" {
		return fmt.Errorf("commit %s: %q check run (push/pull_request) concluded %s, not success", sha, checkName, latest.Conclusion)
	}
	return nil
}

// gatingSuiteIDs returns the set of check_suite ids that count as gating:
// a check_suite counts as gating only if some workflow run reports that
// check_suite's id together with a gating event. A check_suite can in
// principle have more than one workflow run associated (a re-run creates
// a new one on the same suite); any of them reporting a gating event is
// enough.
func gatingSuiteIDs(workflowRuns []actionsRun) map[int64]bool {
	gatingSuite := make(map[int64]bool, len(workflowRuns))
	for _, r := range workflowRuns {
		if gatingEvents[r.Event] {
			gatingSuite[r.CheckSuiteID] = true
		}
	}
	return gatingSuite
}

// latestGatingCheckRun picks, among the check runs named checkName, the
// most recently started one whose check_suite is in gatingSuite (nil when
// there is none). It also reports how many runs carried that name in total
// and how many of those belonged to a non-gating suite, so evaluate can
// say exactly what it found.
func latestGatingCheckRun(checkRuns []checkRun, gatingSuite map[int64]bool, checkName string) (latest *checkRun, namedTotal, nonGating int) {
	for i := range checkRuns {
		cr := &checkRuns[i]
		if cr.Name != checkName {
			continue
		}
		namedTotal++
		if !gatingSuite[cr.CheckSuite.ID] {
			nonGating++
			continue
		}
		if latest == nil || cr.StartedAt > latest.StartedAt {
			latest = cr
		}
	}
	return latest, namedTotal, nonGating
}

// fetchCheckRuns performs the actual GitHub API call, and follows every
// rel="next" Link header page GitHub returns, merging every page's
// check_runs into one response before returning -- a later ignored
// schedule/workflow_dispatch check run pushing the real push/PR suite
// check onto a second (or later) page must never make it invisible to
// evaluate. Kept separate from evaluate so the unit tests exercise
// evaluate directly against canned JSON, never a live network call.
func fetchCheckRuns(repo, sha, token string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/%s/check-runs", githubAPIBase, repo, sha)
	var merged checkRunsResponse
	for url != "" {
		body, link, err := fetchGitHubJSON(url, token)
		if err != nil {
			return nil, err
		}
		var page checkRunsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("commit %s: could not parse check-runs response: %w", sha, err)
		}
		merged.TotalCount = page.TotalCount
		merged.CheckRuns = append(merged.CheckRuns, page.CheckRuns...)
		url = nextPageURL(link)
	}
	return json.Marshal(merged)
}

// fetchActionsRuns performs the actual GitHub API call that resolves each
// check_suite on sha to the event that triggered its workflow run (push,
// pull_request, schedule, workflow_dispatch, ...), following every
// rel="next" Link header page the same way fetchCheckRuns does -- a
// later ignored schedule/workflow_dispatch run's event association
// pushing the real push/PR one onto a second page must never make that
// association invisible to evaluate's gatingSuite lookup.
func fetchActionsRuns(repo, sha, token string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runs?head_sha=%s", githubAPIBase, repo, sha)
	var merged actionsRunsResponse
	for url != "" {
		body, link, err := fetchGitHubJSON(url, token)
		if err != nil {
			return nil, err
		}
		var page actionsRunsResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("commit %s: could not parse actions-runs response: %w", sha, err)
		}
		merged.WorkflowRuns = append(merged.WorkflowRuns, page.WorkflowRuns...)
		url = nextPageURL(link)
	}
	return json.Marshal(merged)
}

// fetchGitHubJSON performs one page's GET request, returning its body
// together with the raw Link response header so the caller can decide
// whether to follow a rel="next" page.
func fetchGitHubJSON(url, token string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("request to %s failed: %s: %s", url, resp.Status, string(body))
	}
	return body, resp.Header.Get("Link"), nil
}

// nextPageURL extracts the rel="next" target from a GitHub Link response
// header (RFC 5988 form: `<url>; rel="next", <url>; rel="last"`), or ""
// if there is no next page -- the signal to stop paginating.
func nextPageURL(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}
	for _, part := range strings.Split(linkHeader, ",") {
		segments := strings.Split(part, ";")
		if len(segments) < 2 {
			continue
		}
		urlPart := strings.TrimSpace(segments[0])
		if !strings.HasPrefix(urlPart, "<") || !strings.HasSuffix(urlPart, ">") {
			continue
		}
		for _, rel := range segments[1:] {
			if strings.TrimSpace(rel) == `rel="next"` {
				return urlPart[1 : len(urlPart)-1]
			}
		}
	}
	return ""
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

	checkRunsBody, err := fetchCheckRuns(*repo, *sha, token)
	if err != nil {
		return err
	}
	actionsRunsBody, err := fetchActionsRuns(*repo, *sha, token)
	if err != nil {
		return err
	}
	return evaluate(checkRunsBody, actionsRunsBody, *sha, *check)
}

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
