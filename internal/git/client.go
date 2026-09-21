// Package git reads the repository state minienv embeds into image tags.
package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/robgonnella/minienv/internal/errs"
)

type GitClient struct {
	mu sync.Mutex
	// One git process per directory rather than per service, so services
	// declared from the same directory share a sha.
	cache map[string]string
}

func NewGitClient() *GitClient {
	return &GitClient{cache: map[string]string{}}
}

// ShortSha returns the abbreviated commit hash of HEAD for the repository
// containing dir; empty dir means the calling process's own directory.
func (c *GitClient) ShortSha(ctx context.Context, dir string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if sha, ok := c.cache[dir]; ok {
		return sha, nil
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir

	data, err := cmd.Output()
	if err != nil {
		// An *exec.ExitError reads as "exit status 128" on its own; git writes
		// the reason to stderr, so fold it in or the caller has nothing to act
		// on. Output() captures stderr only for an ExitError.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", errs.Errorf(
				ErrShortSha,
				"git rev-parse failed: %w: %s",
				err,
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}

		return "", errs.Errorf(ErrShortSha, "git rev-parse failed: %w", err)
	}

	sha := strings.TrimSpace(string(data))
	c.cache[dir] = sha

	return sha, nil
}
