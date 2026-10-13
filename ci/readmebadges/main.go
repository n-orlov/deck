// Package main implements ci/readmebadges, R291's README badge generator.
//
//	go run ./ci/readmebadges           # rewrite the block in README.md
//	go run ./ci/readmebadges -print    # print the block, change nothing
//
// The README opens with a block between the markers "<!-- badges:start -->"
// and "<!-- badges:end -->". This program is the only author of what sits
// between them. It holds, in order:
//
//   - the ci.yml workflow-status badge for main, linked to that workflow;
//   - the five shields.io endpoint badges for the gates ci/badgegen emits
//     (lint, unit-tests, features, coverage, vulnerabilities), each reading
//     the badges/<name>.json the report job publishes to gh-pages and linked
//     to the ci.yml workflow (the coverage badge links to the report);
//   - static badges for the licence, the Go version (the "go" directive of
//     go.mod) and the published Allure report, the last linked to the report.
//
// The generator reads go.mod only. It has no network access and writes no
// file other than README.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Markers delimit the generated block in README.md.
const (
	StartMarker = "<!-- badges:start -->"
	EndMarker   = "<!-- badges:end -->"
)

const (
	repoURL     = "https://github.com/n-orlov/deck"
	workflowURL = repoURL + "/actions/workflows/ci.yml"
	pagesURL    = "https://n-orlov.github.io/deck/"
	badgesURL   = pagesURL + "badges/"
	// licenseText is what the licence badge states. The repository declares
	// no licence (no LICENSE file; the GitHub API reports none), so the
	// badge says so rather than naming one.
	licenseText = "none declared"
)

// gateBadges are the endpoint badges, in README order: the file stem
// ci/badgegen writes (and gh-pages serves) and the alt text.
var gateBadges = []struct{ Name, Alt string }{
	{"lint", "lint"},
	{"unit-tests", "unit tests"},
	{"features", "features"},
	{"coverage", "coverage"},
	{"vulnerabilities", "vulnerabilities"},
}

var goDirective = regexp.MustCompile(`(?m)^go[ \t]+([0-9][0-9A-Za-z.\-]*)[ \t]*$`)

// GoVersion returns the "go" directive of a go.mod file's contents.
func GoVersion(goMod string) (string, error) {
	m := goDirective.FindStringSubmatch(goMod)
	if m == nil {
		return "", errors.New("no go directive in go.mod")
	}
	return m[1], nil
}

func badge(alt, image, link string) string {
	return fmt.Sprintf("[![%s](%s)](%s)", alt, image, link)
}

// staticImage is a shields.io static badge URL. Dashes and underscores in
// the parts are doubled so shields.io does not read them as separators.
func staticImage(label, message, color string) string {
	esc := func(s string) string {
		s = strings.NewReplacer("-", "--", "_", "__").Replace(s)
		return strings.ReplaceAll(url.PathEscape(s), "%20", "_")
	}
	return "https://img.shields.io/badge/" + esc(label) + "-" + esc(message) + "-" + color
}

// Block returns the marker-delimited badge block for a Go version, without a
// trailing newline.
func Block(goVersion string) string {
	lines := []string{StartMarker}
	lines = append(lines, badge("CI",
		workflowURL+"/badge.svg?branch=main",
		workflowURL+"?query=branch%3Amain"))
	for _, g := range gateBadges {
		link := workflowURL + "?query=branch%3Amain"
		if g.Name == "coverage" {
			link = pagesURL
		}
		image := "https://img.shields.io/endpoint?url=" + url.QueryEscape(badgesURL+g.Name+".json")
		lines = append(lines, badge(g.Alt, image, link))
	}
	lines = append(lines,
		badge("license", staticImage("license", licenseText, "lightgrey"), repoURL),
		badge("Go version", staticImage("go", goVersion, "00ADD8"), "https://go.dev/doc/devel/release"),
		badge("report", staticImage("report", "Allure", "blue"), pagesURL),
		EndMarker)
	return strings.Join(lines, "\n")
}

// Replace swaps the block in a README for want. The block must exist.
func Replace(readme, want string) (string, error) {
	start := strings.Index(readme, StartMarker)
	end := strings.Index(readme, EndMarker)
	if start < 0 || end < start {
		return "", errors.New("README.md has no badges:start .. badges:end block")
	}
	return readme[:start] + want + readme[end+len(EndMarker):], nil
}

// Extract returns the marker-delimited block of a README.
func Extract(readme string) (string, error) {
	start := strings.Index(readme, StartMarker)
	end := strings.Index(readme, EndMarker)
	if start < 0 || end < start {
		return "", errors.New("README.md has no badges:start .. badges:end block")
	}
	return readme[start : end+len(EndMarker)], nil
}

// Insert puts block above the first line of a README that has none yet.
func Insert(readme, block string) string {
	return block + "\n\n" + readme
}

func run(root string, printOnly bool) error {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec // G304: root is a command-line argument naming the repository
	if err != nil {
		return err
	}
	version, err := GoVersion(string(mod))
	if err != nil {
		return err
	}
	block := Block(version)
	if printOnly {
		fmt.Println(block)
		return nil
	}
	path := filepath.Join(root, "README.md")
	readme, err := os.ReadFile(path) //nolint:gosec // G304: path is README.md under the command-line root
	if err != nil {
		return err
	}
	text := string(readme)
	if strings.Contains(text, StartMarker) {
		text, err = Replace(text, block)
		if err != nil {
			return err
		}
	} else {
		text = Insert(text, block)
	}
	return os.WriteFile(path, []byte(text), 0o644) //nolint:gosec // G306: README.md is a public tracked file, 0644 like the rest of the tree
}

func main() {
	printOnly := flag.Bool("print", false, "print the block instead of rewriting README.md")
	root := flag.String("root", ".", "repository root (holds go.mod and README.md)")
	flag.Parse()
	if err := run(*root, *printOnly); err != nil {
		fmt.Fprintln(os.Stderr, "readmebadges:", err)
		os.Exit(2)
	}
}
