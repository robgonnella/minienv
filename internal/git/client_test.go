package git_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/git"
)

// Each repo is created fresh under TempDir with a pinned identity and no
// user or system config, so a developer's gpgsign or hooks cannot leak in.
func runGit(dir string, args ...string) string {
	cmd := exec.CommandContext(
		context.Background(),
		"git",
		append([]string{
			"-c", "user.name=minienv",
			"-c", "user.email=minienv@example.com",
			"-c", "commit.gpgsign=false",
		}, args...)...,
	)
	cmd.Dir = dir
	cmd.Env = append(
		cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)

	out, err := cmd.CombinedOutput()
	Expect(err).ShouldNot(HaveOccurred(), string(out))

	return strings.TrimSpace(string(out))
}

func commit(dir, message string) string {
	runGit(dir, "commit", "-q", "--allow-empty", "-m", message)

	return runGit(dir, "rev-parse", "--short", "HEAD")
}

// The message carries the path so two repos created within the same second
// do not hash to the same commit.
func newRepo() (string, string) {
	dir := GinkgoT().TempDir()
	runGit(dir, "init", "-q")

	return dir, commit(dir, "init "+dir)
}

// The happy-path specs shell out to real git against this checkout or a repo
// created under TempDir; the error branch runs it outside any repository.
var _ = Describe("GitClient", func() {
	var subject *git.GitClient

	BeforeEach(func() {
		subject = git.NewGitClient()
	})

	Describe("ShortSha", func() {
		It("returns the abbreviated hash of HEAD", func() {
			sha, err := subject.ShortSha(context.Background(), "")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(MatchRegexp(`^[0-9a-f]{7,40}$`))
		})

		It("strips the newline git appends to its output", func() {
			sha, err := subject.ShortSha(context.Background(), "")

			// Callers substitute this straight into an image tag, so a
			// trailing newline would travel into the rendered chart values.
			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(strings.TrimSpace(sha)))
		})

		It("resolves the repository containing dir, not the process's", func() {
			repo, want := newRepo()

			own, err := subject.ShortSha(context.Background(), "")
			Expect(err).ShouldNot(HaveOccurred())

			sha, err := subject.ShortSha(context.Background(), repo)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(want))
			Expect(sha).ToNot(Equal(own))
		})

		It("keeps a sha per directory", func() {
			repoA, wantA := newRepo()
			repoB, wantB := newRepo()

			shaA, err := subject.ShortSha(context.Background(), repoA)
			Expect(err).ShouldNot(HaveOccurred())

			shaB, err := subject.ShortSha(context.Background(), repoB)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(shaA).To(Equal(wantA))
			Expect(shaB).To(Equal(wantB))
			Expect(shaA).ToNot(Equal(shaB))
		})

		It("reuses the sha already resolved for a directory", func() {
			repo, first := newRepo()

			sha, err := subject.ShortSha(context.Background(), repo)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(first))

			// HEAD moves on, so a second sha coming back equal to the first
			// proves git was not run again.
			second := commit(repo, "second")
			Expect(second).ToNot(Equal(first))

			sha, err = subject.ShortSha(context.Background(), repo)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(first))
		})

		It("resolves one sha per directory across concurrent callers", func() {
			const callers = 8

			repoA, wantA := newRepo()
			repoB, wantB := newRepo()
			repos := []string{repoA, repoB}

			type result struct{ dir, sha string }

			results := make(chan result, callers)

			var wg sync.WaitGroup

			for i := range callers {
				wg.Add(1)

				go func(dir string) {
					defer GinkgoRecover()
					defer wg.Done()

					sha, err := subject.ShortSha(context.Background(), dir)

					Expect(err).ShouldNot(HaveOccurred())

					results <- result{dir: dir, sha: sha}
				}(repos[i%len(repos)])
			}

			wg.Wait()
			close(results)

			shas := map[string]map[string]struct{}{}
			for r := range results {
				if shas[r.dir] == nil {
					shas[r.dir] = map[string]struct{}{}
				}

				shas[r.dir][r.sha] = struct{}{}
			}

			Expect(shas[repoA]).To(Equal(map[string]struct{}{wantA: {}}))
			Expect(shas[repoB]).To(Equal(map[string]struct{}{wantB: {}}))
		})

		Context("when a sha has already been resolved", func() {
			It("returns it without running git", func() {
				// The directory is outside any repository, so git could only
				// fail here: a sha coming back proves it never ran.
				dir := GinkgoT().TempDir()
				cached := git.NewGitClientWithCachedSha(dir, "cafe123")

				sha, err := cached.ShortSha(context.Background(), dir)

				Expect(err).ShouldNot(HaveOccurred())
				Expect(sha).To(Equal("cafe123"))
			})
		})

		Context("outside a git repository", func() {
			var outside string

			BeforeEach(func() {
				// TempDir lives under the OS temp dir and is removed on
				// cleanup, so it is never inside this checkout.
				outside = GinkgoT().TempDir()
			})

			It("returns a git error and no sha", func() {
				sha, err := subject.ShortSha(context.Background(), outside)

				Expect(err).To(MatchError(git.ErrShortSha))
				Expect(sha).To(BeEmpty())
			})

			It("keeps git's exit status reachable as the cause", func() {
				_, err := subject.ShortSha(context.Background(), outside)

				// config wraps this again for the +git tag convention, so the
				// cause has to survive an Unwrap chain, not just one level.
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
			})
		})
	})
})
