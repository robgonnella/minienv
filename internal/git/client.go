// Package git reads the repository state minienv embeds into image tags.
package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/robgonnella/minienv/internal/errs"
)

type GitClient struct {
	// dir is the working directory git runs in. Empty means the calling
	// process's own directory, which is what production always uses; specs set
	// it to reach ShortSha's error branch without os.Chdir.
	dir string
}

func NewGitClient() *GitClient {
	return &GitClient{}
}

// ShortSha returns the abbreviated commit hash of HEAD. The trailing newline
// git writes is stripped here so callers can embed the value directly.
func (c *GitClient) ShortSha(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = c.dir

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

	return strings.TrimSpace(string(data)), nil
}
