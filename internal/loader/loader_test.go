package loader_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	"github.com/robgonnella/minienv/internal/loader"
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

	// ProjectDirectory is pinned to testdata so the repo's own .env and any
	// COMPOSE_FILE in the environment cannot leak into these specs.
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
			core, err := newLoader(fixture("valid.compose.yml")).LoadCore()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(core).ToNot(BeNil())
		})

		It("errors when the compose file does not exist", func() {
			_, err := newLoader(fixture("nope.compose.yml")).LoadCore()

			Expect(err).To(MatchError(loader.KindProjectLoad))
		})

		It("errors when the compose file has no x-minienv extension", func() {
			_, err := newLoader(fixture("no-extension.compose.yml")).LoadCore()

			Expect(err).To(MatchError(loader.KindNoExtension))
		})

		It("errors when x-minienv configures no deploy target", func() {
			_, err := newLoader(fixture("no-deployer.compose.yml")).LoadCore()

			Expect(err).To(MatchError(loader.KindNoActiveDeployer))
		})

		It("errors when the x-minienv extension cannot be decoded", func() {
			_, err := newLoader(
				fixture("malformed-extension.compose.yml"),
			).LoadCore()

			Expect(err).To(MatchError(loader.KindExtensionDecode))
		})
	})
})
