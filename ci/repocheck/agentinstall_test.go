// agentinstall_test.go: INSTALL-AGENT.md walks a coding agent through what
// install.sh does, one command at a time (#133). The two must not drift: the
// guide names exactly the archives install.sh can fetch (and the release
// build publishes), the same default install directory and the same
// checksums file, and it keeps the immutability rule deck update applies.
package repocheck

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var assetNameRE = regexp.MustCompile(`deck_[a-z0-9]+_[a-z0-9]+\.tar\.gz`)

// installScriptAssets expands install.sh's asset template over the OS and
// architecture values its case statements accept.
func installScriptAssets(t *testing.T, script string) []string {
	t.Helper()
	if !strings.Contains(script, `asset="deck_${os}_${arch}.tar.gz"`) {
		t.Fatal(`install.sh no longer builds the asset as deck_${os}_${arch}.tar.gz; update this test and INSTALL-AGENT.md`)
	}
	osLine := regexp.MustCompile(`(?m)^\s*([a-z]+(?:\s*\|\s*[a-z]+)*)\)\s*;;`).FindStringSubmatch(script)
	if osLine == nil {
		t.Fatal("install.sh: cannot find the supported-OS case arm")
	}
	var oses []string
	for _, o := range strings.Split(osLine[1], "|") {
		oses = append(oses, strings.TrimSpace(o))
	}
	var arches []string
	for _, m := range regexp.MustCompile(`arch=([a-z0-9]+) ;;`).FindAllStringSubmatch(script, -1) {
		arches = append(arches, m[1])
	}
	if len(oses) == 0 || len(arches) == 0 {
		t.Fatalf("install.sh: parsed OSes %v and architectures %v", oses, arches)
	}
	var assets []string
	for _, o := range oses {
		for _, a := range arches {
			assets = append(assets, "deck_"+o+"_"+a+".tar.gz")
		}
	}
	sort.Strings(assets)
	return assets
}

func uniqueSorted(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func TestAgentInstallGuideMatchesInstallScript(t *testing.T) {
	script := readRepoFile(t, "install.sh")
	guide := readRepoFile(t, "INSTALL-AGENT.md")
	want := installScriptAssets(t, script)

	got := uniqueSorted(assetNameRE.FindAllString(guide, -1))
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("INSTALL-AGENT.md names assets %v; install.sh fetches %v", got, want)
	}
	if !strings.Contains(guide, `deck_${os}_${arch}.tar.gz`) {
		t.Error("INSTALL-AGENT.md must build the asset name as deck_${os}_${arch}.tar.gz, like install.sh")
	}

	// Release build: the archives it publishes are the ones install.sh fetches.
	build := readRepoFile(t, "ci/release-build.sh")
	targets := regexp.MustCompile(`(?m)^for target in (.+); do`).FindStringSubmatch(build)
	if targets == nil {
		t.Fatal("ci/release-build.sh: cannot find the target list")
	}
	var built []string
	for _, tgt := range strings.Fields(targets[1]) {
		built = append(built, "deck_"+strings.Replace(tgt, "/", "_", 1)+".tar.gz")
	}
	if strings.Join(uniqueSorted(built), " ") != strings.Join(want, " ") {
		t.Errorf("ci/release-build.sh publishes %v; install.sh fetches %v", uniqueSorted(built), want)
	}

	const dir = `${DECK_INSTALL_DIR:-$HOME/.local/bin}`
	if !strings.Contains(script, `dir="`+dir+`"`) {
		t.Fatalf("install.sh's install directory is no longer %s; update this test and INSTALL-AGENT.md", dir)
	}
	for _, s := range []string{
		dir,               // same default install directory and override
		"DECK_VERSION",    // same pin
		"checksums.txt",   // same checksum file
		`" $asset\$"`,     // same checksum line match
		"--proto =https",  // HTTPS-only downloads
		"/releases/tags/", // the API deck update asks
		`"immutable":`,    // ... and the field it reads
		"DECK_REQUIRE_IMMUTABLE",
		"deck.new", // copy-then-rename, as install.sh does
	} {
		if !strings.Contains(guide, s) {
			t.Errorf("INSTALL-AGENT.md lost %q, which install.sh / deck update rely on", s)
		}
	}
}

func TestReadmeOffersTheAgentInstallGuide(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	const link = "https://github.com/n-orlov/deck/blob/main/INSTALL-AGENT.md"
	if !strings.Contains(readme, link) {
		t.Errorf("README.md's Install section must offer %s", link)
	}
}
