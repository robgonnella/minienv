package git

// NewGitClientInDir builds a client that runs git in dir. ShortSha's error
// branch is only reachable from a working directory outside a repository, and
// os.Chdir is process-global — unsafe under `just test`'s -p --randomize-all.
// Setting the command's directory reaches the same branch without it.
func NewGitClientInDir(dir string) *GitClient {
	return &GitClient{dir: dir}
}

// NewGitClientWithCachedSha builds a client in dir that has already resolved
// sha. Pairing a cached value with a directory outside any repository is what
// makes the short-circuit observable: git itself could only fail there.
func NewGitClientWithCachedSha(dir, sha string) *GitClient {
	return &GitClient{dir: dir, cachedSha: sha}
}
