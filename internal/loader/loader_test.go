package loader_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
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
			core, err := newLoader(fixture("valid.compose.yml")).LoadCore(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(core).ToNot(BeNil())
		})

		It("errors when the compose file does not exist", func() {
			_, err := newLoader(fixture("nope.compose.yml")).LoadCore(context.Background())

			Expect(err).To(MatchError(loader.ErrProjectLoad))
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
	})

	// A reserved ngrok URL is committed to compose.yml, so it can only differ
	// per developer if compose interpolates it.
	Describe("interpolation", func() {
		BeforeEach(func() {
			GinkgoT().Setenv("MINIENV_NAMESPACE", "alice")
		})

		It("resolves env vars inside a service extension", func() {
			project, err := newLoader(
				fixture("interpolated.compose.yml"),
			).LoadComposeProject(context.Background())
			Expect(err).ShouldNot(HaveOccurred())

			ext := project.Services["api"].
				Extensions[config.K8sServiceExtension].(map[string]any)
			ngrok := ext["ngrok"].(map[string]any)

			Expect(ngrok["url"]).
				To(Equal("https://alice-api.example.ngrok.app"))
		})

		It("resolves env vars inside the top level extension", func() {
			project, err := newLoader(
				fixture("interpolated.compose.yml"),
			).LoadComposeProject(context.Background())
			Expect(err).ShouldNot(HaveOccurred())

			ext := project.
				Extensions[config.TopLevelExtension].(map[string]any)
			k8s := ext["k8s"].(map[string]any)

			Expect(k8s["namespace"]).To(Equal("alice"))
		})
	})
})
