package transport

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/robgonnella/minienv/internal/errs"
)

// A leading ~ is shell syntax rather than part of the path: compose expands
// $VAR but leaves this alone, and the file APIs take it literally.
func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", errs.Errorf(
			ErrUserHomeDir,
			"failed to get the user's home directory: %w",
			err,
		)
	}

	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}
