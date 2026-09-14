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

// The happy-path specs shell out to real git against this checkout; the error
// branch runs it from a directory outside any repository.
var _ = Describe("GitClient", func() {
	var subject *git.GitClient

	BeforeEach(func() {
		subject = git.NewGitClient()
	})

	Describe("ShortSha", func() {
		It("returns the abbreviated hash of HEAD", func() {
			sha, err := subject.ShortSha(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(MatchRegexp(`^[0-9a-f]{7,40}$`))
		})

		It("strips the newline git appends to its output", func() {
			sha, err := subject.ShortSha(context.Background())

			// Callers substitute this straight into an image tag, so a
			// trailing newline would travel into the rendered chart values.
			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(strings.TrimSpace(sha)))
		})

		It("resolves a single sha across concurrent callers", func() {
			const callers = 8

			results := make(chan string, callers)

			var wg sync.WaitGroup

			for range callers {
				wg.Add(1)

				go func() {
					defer GinkgoRecover()
					defer wg.Done()

					sha, err := subject.ShortSha(context.Background())

					Expect(err).ShouldNot(HaveOccurred())

					results <- sha
				}()
			}

			wg.Wait()
			close(results)

			shas := map[string]struct{}{}
			for sha := range results {
				shas[sha] = struct{}{}
			}

			Expect(shas).To(HaveLen(1))
		})

		Context("when a sha has already been resolved", func() {
			It("returns it without running git", func() {
				// The directory is outside any repository, so git could only
				// fail here: a sha coming back proves it never ran.
				cached := git.NewGitClientWithCachedSha(
					GinkgoT().TempDir(),
					"cafe123",
				)

				sha, err := cached.ShortSha(context.Background())

				Expect(err).ShouldNot(HaveOccurred())
				Expect(sha).To(Equal("cafe123"))
			})
		})

		Context("outside a git repository", func() {
			var outside *git.GitClient

			BeforeEach(func() {
				// TempDir lives under the OS temp dir and is removed on
				// cleanup, so it is never inside this checkout.
				outside = git.NewGitClientInDir(GinkgoT().TempDir())
			})

			It("returns a git error and no sha", func() {
				sha, err := outside.ShortSha(context.Background())

				Expect(err).To(MatchError(git.ErrShortSha))
				Expect(sha).To(BeEmpty())
			})

			It("keeps git's exit status reachable as the cause", func() {
				_, err := outside.ShortSha(context.Background())

				// config wraps this again for the +git tag convention, so the
				// cause has to survive an Unwrap chain, not just one level.
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
			})
		})
	})
})
