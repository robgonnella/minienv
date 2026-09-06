package deployer_test

import (
	"errors"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/image"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	"github.com/stretchr/testify/mock"
)

// The image-client failure the fake returns. Asserted by identity so the spec
// proves BuildAndPushServiceImages passes the error through untouched.
var errBake = errors.New("bake blew up")

var _ = Describe("Helm", func() {
	var (
		ext       *config.XMiniEnv
		mockImage *imagemocks.MockClient
		mockGit   *gitmocks.MockClient
		subject   *deployer.Helm
	)

	BeforeEach(func() {
		ext = &config.XMiniEnv{
			K8s: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
		}
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
	})

	JustBeforeEach(func() {
		subject = deployer.NewHelm(deployer.HelmOptions{
			Ext:            ext,
			ImageClient:    mockImage,
			GitClient:      mockGit,
			NgrokAuthToken: "",
			DryRun:         false,
		})
	})

	Describe("Deployer interface", func() {
		It("identifies itself as the k8s deployer", func() {
			Expect(subject.ConfigField()).To(Equal("k8s"))
			Expect(subject.String()).To(Equal("Helm"))
		})
	})

	Describe("Active", func() {
		It("is active when both context and namespace are set", func() {
			Expect(subject.Active()).To(BeTrue())
		})

		It("is inactive with no context", func() {
			ext.K8s.Context = ""
			Expect(subject.Active()).To(BeFalse())
		})

		It("is inactive with no namespace", func() {
			ext.K8s.Namespace = ""
			Expect(subject.Active()).To(BeFalse())
		})

		It("is inactive with neither configured", func() {
			ext.K8s = config.XMiniEnvK8s{}
			Expect(subject.Active()).To(BeFalse())
		})
	})

	Describe("when inactive", func() {
		var project *config.ComposeProject

		BeforeEach(func() {
			ext.K8s = config.XMiniEnvK8s{}
			project = &config.ComposeProject{Name: "test-project"}
		})

		// These return before any helm or kubernetes client is constructed, so
		// they are safe to exercise without a cluster.
		It("does nothing on Init", func() {
			Expect(subject.Init(project)).To(Succeed())
		})

		It("does nothing on Deploy", func() {
			Expect(subject.Deploy()).To(Succeed())
		})

		It("does nothing on Destroy", func() {
			Expect(subject.Destroy()).To(Succeed())
		})
	})

	Describe("buildAndPushServiceImages", func() {
		var project *config.ComposeProject

		// A service the build pass should pick up: it has a build block, an
		// image the extension can resolve, and no skip flag.
		buildable := func(name string) config.ComposeService {
			return config.ComposeService{
				Name:  name,
				Image: "reg/" + name + ":v1",
				Build: &types.BuildConfig{
					Context:    "./" + name,
					Dockerfile: name + "/Dockerfile",
					Args: types.MappingWithEquals{
						"VERSION": new("1.2.3"),
					},
				},
			}
		}

		BeforeEach(func() {
			project = &config.ComposeProject{Name: "test-project"}
		})

		It("translates a buildable service into image properties", func() {
			project.Services = types.Services{"hello": buildable("hello")}
			Expect(subject.InitProject(project)).To(Succeed())

			mockImage.
				EXPECT().
				BuildAndPush([]image.ServiceProperties{
					{
						Name:       "hello",
						Registry:   "reg/hello",
						Tag:        "v1",
						Context:    "./hello",
						Dockerfile: "hello/Dockerfile",
						Platforms:  []string{"linux/amd64"},
						Args:       map[string]string{"VERSION": "1.2.3"},
					},
				}).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages()).To(Succeed())
		})

		It("carries the platforms resolved from the extension", func() {
			svc := buildable("hello")
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"image": map[string]any{
						"platforms": []any{"linux/arm64", "linux/amd64"},
					},
				},
			}
			project.Services = types.Services{"hello": svc}
			Expect(subject.InitProject(project)).To(Succeed())

			var got []image.ServiceProperties
			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Run(func(s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages()).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Platforms).
				To(Equal([]string{"linux/arm64", "linux/amd64"}))
		})

		It("excludes a service flagged skip", func() {
			skipped := buildable("skipped")
			skipped.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{"skip": true},
			}
			project.Services = types.Services{
				"hello":   buildable("hello"),
				"skipped": skipped,
			}
			Expect(subject.InitProject(project)).To(Succeed())

			var got []image.ServiceProperties
			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Run(func(s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages()).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Name).To(Equal("hello"))
		})

		// mockImage carries no expectation in these two, so any call to
		// BuildAndPush fails the spec on cleanup.
		It("never calls the image client when nothing has a build block", func() {
			project.Services = types.Services{
				"hello": {Name: "hello", Image: "reg/hello:v1"},
			}
			Expect(subject.InitProject(project)).To(Succeed())

			Expect(subject.BuildAndPushServiceImages()).To(Succeed())
		})

		It("never calls the image client for an empty project", func() {
			Expect(subject.InitProject(project)).To(Succeed())
			Expect(subject.BuildAndPushServiceImages()).To(Succeed())
		})

		It("propagates an image client failure", func() {
			project.Services = types.Services{"hello": buildable("hello")}
			Expect(subject.InitProject(project)).To(Succeed())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Return(errBake).
				Once()

			err := subject.BuildAndPushServiceImages()

			Expect(err).To(MatchError(errBake))
		})
	})

	// The half of Init that runs before a cluster is touched. Everything
	// downstream reads the map it builds, so a service that cannot be resolved
	// has to fail here rather than midway through a deployment.
	Describe("initProject", func() {
		var project *config.ComposeProject

		BeforeEach(func() {
			project = &config.ComposeProject{Name: "test-project"}
		})

		It("returns a config error when a service cannot be resolved", func() {
			// No image and no extension: resolution fails before any build.
			project.Services = types.Services{"broken": {Name: "broken"}}

			err := subject.InitProject(project)

			// resolveServiceImage accumulates its failures with errors.Join, so
			// the result is a join wrapper. errors.Is walks the join, which lets
			// this assert that *both* validation failures were collected.
			Expect(err).To(MatchError(config.KindImageRepositoryMissing))
			Expect(err).To(MatchError(config.KindImageTagMissing))
		})

		It("rejects a service with an unknown deployment type", func() {
			svc := config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"deploymentType": "sevice",
				},
			}
			project.Services = types.Services{"hello": svc}

			Expect(subject.InitProject(project)).
				To(MatchError(config.KindInvalidDeploymentType))
		})

		It("resolves every service before any release is touched", func() {
			project.Services = types.Services{
				"hello": {Name: "hello", Image: "reg/hello:v1"},
				"job":   {Name: "job", Image: "reg/job:v1"},
			}

			Expect(subject.InitProject(project)).To(Succeed())
		})
	})
})
