package loader_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
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
	newLoaderIn := func(dir string, files ...string) *loader.Loader {
		return loader.New(&loader.LoaderOpts{
			Files:            files,
			ProjectDirectory: dir,
			ProjectName:      "test-project",
			DryRun:           true,
			ImageClient:      mockImage,
			GitClient:        mockGit,
		})
	}

	newLoader := func(files ...string) *loader.Loader {
		return newLoaderIn(testdataDir, files...)
	}

	absFixture := func(parts ...string) string {
		GinkgoHelper()

		abs, err := filepath.Abs(filepath.Join(
			append([]string{testdataDir}, parts...)...,
		))
		Expect(err).ShouldNot(HaveOccurred())

		return abs
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

	Describe("host environment", func() {
		hostEnv := func(dir, file string) deployer.HostEnv {
			GinkgoHelper()

			subject := newLoaderIn(dir, file)

			project, err := subject.LoadComposeProject(context.Background())
			Expect(err).ShouldNot(HaveOccurred())

			env, err := subject.LoadHostEnv(context.Background(), project)
			Expect(err).ShouldNot(HaveOccurred())

			return env
		}

		BeforeEach(func() {
			GinkgoT().Setenv("HOST_VALUE", "from-host")
			GinkgoT().Setenv("PASSTHROUGH", "passed")
			GinkgoT().Setenv("INHERITED", "yes")
		})

		Context("with a list-form environment", func() {
			var env deployer.HostEnv

			BeforeEach(func() {
				env = hostEnv(testdataDir, fixture("hostenv.compose.yml"))
			})

			It("attributes interpolated, pass-through and env_file keys to the host", func() {
				Expect(env.Keys("api")).To(Equal([]string{
					"FROM_FILE",
					"FROM_HOST",
					"PASSTHROUGH",
					"WITH_DEFAULT",
				}))
			})

			It("attributes map-form templates and null entries to the host", func() {
				Expect(env.Keys("worker")).To(Equal([]string{
					"INHERITED",
					"TOKEN",
				}))
			})

			It("lists nothing for a service whose values are all literal", func() {
				Expect(env).ToNot(HaveKey("plain"))
			})
		})

		It("classifies a service declared in an included file", func() {
			includeDir := filepath.Join(testdataDir, "include", "envfile")

			env := hostEnv(includeDir, filepath.Join(includeDir, "compose.yml"))

			Expect(env).To(Equal(deployer.HostEnv{
				"sub": []string{"DEEPER_PATH"},
			}))
		})
	})

	Describe("service directories", func() {
		includeDir := filepath.Join(testdataDir, "include")

		serviceDirs := func(dir, file string) deployer.ServiceDirs {
			GinkgoHelper()

			subject := newLoaderIn(dir, file)

			project, err := subject.LoadComposeProject(context.Background())
			Expect(err).ShouldNot(HaveOccurred())

			dirs, err := subject.LoadServiceDirs(context.Background(), project)
			Expect(err).ShouldNot(HaveOccurred())

			return dirs
		}

		It("maps every service to the project directory without include", func() {
			dirs := serviceDirs(testdataDir, fixture("valid.compose.yml"))

			Expect(dirs).To(Equal(deployer.ServiceDirs{
				"hello": absFixture(),
			}))
		})

		Context("with included files", func() {
			var dirs map[string]string

			BeforeEach(func() {
				dirs = serviceDirs(
					filepath.Join(includeDir, "main"),
					filepath.Join(includeDir, "main", "compose.yml"),
				)
			})

			It("maps a top-level service to the project directory", func() {
				Expect(dirs).To(HaveKeyWithValue(
					"web", absFixture("include", "main"),
				))
			})

			It("maps an included service to the included file's directory", func() {
				Expect(dirs).To(HaveKeyWithValue(
					"api", absFixture("include", "other"),
				))
			})

			It("follows an include nested inside an included file", func() {
				Expect(dirs).To(HaveKeyWithValue(
					"worker", absFixture("include", "other", "deeper"),
				))
			})

			It("honours an include's project_directory", func() {
				Expect(dirs).To(HaveKeyWithValue(
					"job", absFixture("include", "nested", "base"),
				))
			})

			It("covers every service compose loaded", func() {
				Expect(dirs).To(HaveLen(4))
			})
		})

		It("attributes a service declared in both files to the includer", func() {
			dirs := serviceDirs(
				filepath.Join(includeDir, "override"),
				filepath.Join(includeDir, "override", "compose.yml"),
			)

			Expect(dirs).To(HaveKeyWithValue(
				"api", absFixture("include", "override"),
			))
			Expect(dirs).To(HaveKeyWithValue(
				"worker", absFixture("include", "other", "deeper"),
			))
		})

		It("interpolates an included file with the include's env_file", func() {
			dirs := serviceDirs(
				filepath.Join(includeDir, "envfile"),
				filepath.Join(includeDir, "envfile", "compose.yml"),
			)

			Expect(dirs).To(Equal(deployer.ServiceDirs{
				"web":  absFixture("include", "envfile"),
				"sub":  absFixture("include", "envfile", "sub"),
				"leaf": absFixture("include", "envfile", "sub", "deeper"),
			}))
		})

		It("errors for a service the walk never saw", func() {
			subject := newLoader(fixture("valid.compose.yml"))

			project, err := subject.LoadComposeProject(context.Background())
			Expect(err).ShouldNot(HaveOccurred())

			project.Services["ghost"] = config.ComposeService{Name: "ghost"}

			_, err = subject.LoadServiceDirs(context.Background(), project)

			Expect(err).To(MatchError(loader.ErrServiceDirMissing))
		})

		It("builds a core from a project with included files", func() {
			core, err := newLoaderIn(
				filepath.Join(includeDir, "main"),
				filepath.Join(includeDir, "main", "compose.yml"),
			).LoadCore(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(core).ToNot(BeNil())
		})
	})
})
