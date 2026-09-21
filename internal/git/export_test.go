package git

// NewGitClientWithCachedSha builds a client that has already resolved sha for
// dir. Pairing a cached value with a directory outside any repository is what
// makes the short-circuit observable: git itself could only fail there.
func NewGitClientWithCachedSha(dir, sha string) *GitClient {
	return &GitClient{cache: map[string]string{dir: sha}}
}
