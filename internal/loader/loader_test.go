package loader_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/compose"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	"github.com/robgonnella/minienv/internal/loader"
	"github.com/robgonnella/minienv/internal/transport"
)

const testdataDir = "testdata"

func fixture(name string) string {
	return filepath.Join(testdataDir, name)
}

var _ = Describe("Loader", func() {
	var (
		mockImage *imagemocks.MockClient
		mockGit   *gitmocks.MockClient
	)

	// ProjectDirectory is pinned to testdata so the repo's own .env cannot leak
	// into these specs.
	newLoader := func(files ...string) *loader.Loader {
		return loader.New(&loader.LoaderOpts{
			Files:            files,
			ProjectDirectory: testdataDir,
			ProjectName:      "test-project",
			DryRun:           true,
			ImageClient:      mockImage,
			GitClient:        mockGit,
		})
	}

	BeforeEach(func() {
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
	})

	Describe("LoadCore", func() {
		It("builds a core from a compose file with a k8s extension", func() {
			core, err := newLoader(fixture("valid.compose.yml")).LoadCore(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(core).ToNot(BeNil())
		})

		It("errors when the compose file does not exist", func() {
			_, err := newLoader(fixture("nope.compose.yml")).LoadCore(context.Background())

			Expect(err).To(MatchError(compose.ErrProjectLoad))
		})

		It("errors when the compose file has no x-minienv extension", func() {
			_, err := newLoader(fixture("no-extension.compose.yml")).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrNoExtension))
		})

		It("errors when x-minienv configures no deploy target", func() {
			_, err := newLoader(fixture("no-deployer.compose.yml")).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrNoActiveDeployer))
		})

		It("errors when the x-minienv extension cannot be decoded", func() {
			_, err := newLoader(
				fixture("malformed-extension.compose.yml"),
			).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrExtensionDecode))
		})

		It("builds a core from a docker extension with an ssh transport", func() {
			core, err := newLoader(
				fixture("docker-ssh.compose.yml"),
			).LoadCore(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(core).ToNot(BeNil())
		})

		// Counting configured targets before building any of them keeps this
		// ahead of whatever a half-configured deployer would complain about.
		It("errors when both deploy targets are configured", func() {
			_, err := newLoader(
				fixture("multiple-deployers.compose.yml"),
			).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrMultipleDeployers))
		})

		It("errors when docker configures no transport", func() {
			_, err := newLoader(
				fixture("docker-no-transport.compose.yml"),
			).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrNoActiveDockerTransport))
		})

		// The transport validates itself as it is built, so a half-configured
		// ssh block fails the load rather than the deploy.
		It("surfaces an ssh block missing its required fields", func() {
			_, err := newLoader(
				fixture("docker-bad-ssh.compose.yml"),
			).LoadCore(context.Background())

			Expect(err).To(MatchError(transport.ErrInvalidExtension))
		})
	})
})
