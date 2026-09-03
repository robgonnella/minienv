package git_test

import (
	"errors"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/git"
)

// The happy-path specs shell out to the real git against this repo's own
// checkout, which is the only place ShortSha succeeds. The error branch runs
// git from a directory outside any repository — see export_test.go for why
// that is done via the command's own directory rather than os.Chdir.
var _ = Describe("GitClient", func() {
	var subject *git.GitClient

	BeforeEach(func() {
		subject = git.NewGitClient()
	})

	Describe("ShortSha", func() {
		It("returns the abbreviated hash of HEAD", func() {
			sha, err := subject.ShortSha()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(MatchRegexp(`^[0-9a-f]{7,40}$`))
		})

		It("strips the newline git appends to its output", func() {
			sha, err := subject.ShortSha()

			// Callers substitute this straight into an image tag, so a
			// trailing newline would travel into the rendered chart values.
			Expect(err).ShouldNot(HaveOccurred())
			Expect(sha).To(Equal(strings.TrimSpace(sha)))
		})

		Context("outside a git repository", func() {
			var outside *git.GitClient

			BeforeEach(func() {
				// TempDir lives under the OS temp dir and is removed on
				// cleanup, so it is never inside this checkout.
				outside = git.NewGitClientInDir(GinkgoT().TempDir())
			})

			It("returns a git error and no sha", func() {
				sha, err := outside.ShortSha()

				Expect(err).To(MatchError(git.KindShortSha))
				Expect(sha).To(BeEmpty())
			})

			It("keeps git's exit status reachable as the cause", func() {
				_, err := outside.ShortSha()

				// config wraps this again for the +git tag convention, so the
				// cause has to survive an Unwrap chain, not just one level.
				var exitErr *exec.ExitError
				Expect(errors.As(err, &exitErr)).To(BeTrue())
			})
		})
	})
})
