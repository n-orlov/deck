// Command prcomment is ci.yml's `pr-comment` job: create or update the
// PR's single Allure-link comment, keyed on the HTML marker
// `<!-- deck-allure-report -->`, never appending a fresh comment on every
// run (R146, review finding B2).
//
// The previous implementation lived inline in ci.yml as an
// actions/github-script step calling octokit's issues.listComments with
// no pagination arguments at all, which only ever returns page one (the
// API's own default page size is 30). Once a PR accumulated more than 30
// ordinary comments -- or the marker comment itself landed beyond
// whatever page one held -- that lookup stopped seeing the existing
// marker comment and started appending a fresh one on every run instead
// of updating in place. This command instead lists every page
// (per_page=100, following the `Link: rel="next"` header exactly like
// ci/releasegate does for check-runs/actions-runs) before deciding
// whether to create or update.
//
//	go run ./ci/prcomment -repo owner/name -pr <number> -sha <head-sha> -link <report-url>
//
// Reads the GitHub token from GITHUB_TOKEN (falling back to GH_TOKEN, the
// gh CLI's own convention) the same way ci/releasegate does.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// githubAPIBase is the root of every GitHub API URL this command builds.
// A var, never a const, purely so the HTTP-level tests below can point it
// at an httptest.Server instead of the real api.github.com -- production
// always leaves it at its zero-value default.
var githubAPIBase = "https://api.github.com"

// marker is the HTML comment string that identifies "the" Allure-link
// comment on a PR among every other comment a human or another bot may
// have left. It must never render visibly in a rendered Markdown comment.
const marker = "<!-- deck-allure-report -->"

// comment is the subset of a GitHub issue-comment object this command
// cares about.
type comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// commentBody renders the marker comment's body, naming both the report
// link and the PR's own head sha so a reader can tell, from the comment
// alone, which commit the linked report was built from.
func commentBody(link, sha string) string {
	return fmt.Sprintf("%s\nAllure report for this PR: %s\n\nBuilt from head commit `%s`.\n", marker, link, sha)
}

// findMarkerComment returns the index of the first comment (in listing
// order, i.e. oldest first, across every page) whose body contains
// marker, or -1 if none of them do.
func findMarkerComment(comments []comment) int {
	for i, c := range comments {
		if strings.Contains(c.Body, marker) {
			return i
		}
	}
	return -1
}

// listComments fetches every issue comment on the given PR/issue,
// following every rel="next" Link header page (per_page=100 requested
// explicitly on the first page; GitHub's own default is 30) -- an
// existing marker comment sitting on page two or later must never become
// invisible to findMarkerComment.
func listComments(repo string, issue int, token string) ([]comment, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments?per_page=100", githubAPIBase, repo, issue)
	var all []comment
	for url != "" {
		body, link, err := doGitHubRequest(http.MethodGet, url, token, nil)
		if err != nil {
			return nil, err
		}
		var page []comment
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("issue %d: could not parse comments response: %w", issue, err)
		}
		all = append(all, page...)
		url = nextPageURL(link)
	}
	return all, nil
}

// createComment posts a brand-new issue comment.
func createComment(repo string, issue int, body, token string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", githubAPIBase, repo, issue)
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return err
	}
	_, _, err = doGitHubRequest(http.MethodPost, url, token, payload)
	return err
}

// updateComment replaces an existing issue comment's body in place.
func updateComment(repo string, commentID int64, body, token string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/comments/%d", githubAPIBase, repo, commentID)
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{Body: body})
	if err != nil {
		return err
	}
	_, _, err = doGitHubRequest(http.MethodPatch, url, token, payload)
	return err
}

// publish is the whole create-or-update decision: list every page of
// existing comments, update the first one whose body carries marker, or
// create a fresh one only when no page held it.
func publish(repo string, issue int, link, sha, token string) error {
	comments, err := listComments(repo, issue, token)
	if err != nil {
		return err
	}
	body := commentBody(link, sha)
	if idx := findMarkerComment(comments); idx >= 0 {
		return updateComment(repo, comments[idx].ID, body, token)
	}
	return createComment(repo, issue, body, token)
}

// doGitHubRequest performs one GitHub API request (GET with no body, or
// POST/PATCH with a JSON body), returning the response body together
// with the raw Link response header so a list call can decide whether to
// follow a rel="next" page.
func doGitHubRequest(method, url, token string, payload []byte) ([]byte, string, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }() // body fully read below; Close cannot lose data

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("%s %s failed: %s: %s", method, url, resp.Status, string(body))
	}
	return body, resp.Header.Get("Link"), nil
}

// nextPageURL extracts the rel="next" target from a GitHub Link response
// header (RFC 5988 form: `<url>; rel="next", <url>; rel="last"`), or ""
// if there is no next page -- the signal to stop paginating. Identical
// to ci/releasegate's own helper; kept as a separate copy rather than a
// shared package because each ci/<tool> command is meant to stay a single
// `go run ./ci/<tool>`-able directory with no intra-ci/ dependency.
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
	fs := flag.NewFlagSet("prcomment", flag.ContinueOnError)
	repo := fs.String("repo", "", "owner/repo (required)")
	issue := fs.Int("pr", 0, "PR (issue) number (required)")
	link := fs.String("link", "", "Allure report link to publish (required)")
	sha := fs.String("sha", "", "PR head sha to name in the comment (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *issue == 0 || *link == "" || *sha == "" {
		return errors.New("prcomment: -repo, -pr, -link and -sha are all required")
	}

	token := getenv("GITHUB_TOKEN")
	if token == "" {
		token = getenv("GH_TOKEN")
	}

	return publish(*repo, *issue, *link, *sha, token)
}

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
