package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRepo is the GitHub repository releases are published in.
const DefaultRepo = "n-orlov/deck"

const ghTimeout = 2 * time.Minute

// gh is an authenticated GitHub CLI found on PATH.
type gh struct{ path string }

// findGh returns the gh on PATH when it is authenticated, else nil.
func findGh(ctx context.Context) *gh {
	p, err := exec.LookPath("gh")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, p, "auth", "status").Run(); err != nil { //nolint:gosec // the user's own gh
		return nil
	}
	return &gh{path: p}
}

func (g *gh) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, g.path, args...) //nolint:gosec // the user's own gh
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// download fetches one release asset into dir through gh and returns its bytes.
func (g *gh) download(ctx context.Context, repo, tag, asset, dir string, limit int64) ([]byte, error) {
	if out, err := g.run(ctx, "release", "download", tag, "-R", repo, "-p", asset, "-D", dir, "--clobber"); err != nil {
		return nil, fmt.Errorf("gh release download %s: %w: %w", asset, err, serverString(strings.TrimSpace(out)))
	}
	name := filepath.Join(dir, asset)
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("gh release download %s: larger than %d bytes", asset, limit)
	}
	return os.ReadFile(name) //nolint:gosec // a file in our own temp dir
}
