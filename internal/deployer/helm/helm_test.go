package helm_test

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/image"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	publishingmocks "github.com/robgonnella/minienv/internal/publishing/mocks"
	"github.com/stretchr/testify/mock"
)

// The image-client failure the fake returns. Asserted by identity so the spec
// proves BuildAndPushServiceImages passes the error through untouched.
var errBake = errors.New("bake blew up")

// Asserted by identity, so the spec proves the error passes through untouched.
var errBoom = errors.New("boom")

// The ngrok port matches the container side of the compose mapping, which is
// what resolveNgrok requires.
func exposedService(name string, port int) config.ComposeService {
	return config.ComposeService{
		Name:  name,
		Image: "reg/" + name + ":v1",
		Ports: []types.ServicePortConfig{
			{Target: uint32(port), Published: strconv.Itoa(port)},
		},
		Extensions: types.Extensions{
			config.K8sServiceExtension: map[string]any{
				"ngrok": map[string]any{"port": port},
			},
		},
	}
}

var _ = Describe("Helm", func() {
	var (
		k8sExt      config.XMiniEnvK8s
		mockImage   *imagemocks.MockClient
		mockGit     *gitmocks.MockClient
		mockPublish *publishingmocks.MockClient
		subject     *helm.Helm
		ngrokToken  string
		alphaURL    *url.URL
	)

	BeforeEach(func() {
		k8sExt = config.XMiniEnvK8s{
			Context:   "context",
			Namespace: "namespace",
		}
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
		mockPublish = publishingmocks.NewMockClient(GinkgoT())
		// Reset explicitly: the suite runs --randomize-all, so a token left
		// set by the ngrok specs would silently enable ngrok for whatever
		// ran next.
		ngrokToken = ""

		parsed, err := url.Parse("https://alpha.ngrok.app")
		Expect(err).ShouldNot(HaveOccurred())

		alphaURL = parsed
	})

	JustBeforeEach(func() {
		subject = helm.New(helm.Options{
			K8sExt:         k8sExt,
			ImageClient:    mockImage,
			GitClient:      mockGit,
			PublishClient:  mockPublish,
			NgrokAuthToken: ngrokToken,
			DryRun:         false,
		})
	})

	Describe("Deployer interface", func() {
		It("identifies itself as the k8s deployer", func() {
			Expect(subject.String()).To(Equal("Helm"))
		})
	})

	Describe("when inactive", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			k8sExt = config.XMiniEnvK8s{}
			project = config.ComposeProject{Name: "test-project"}
		})

		// These return before any helm or kubernetes client is constructed, so
		// they are safe to exercise without a cluster.
		It("returns error on Init", func() {
			Expect(subject.Init(context.Background(), project)).NotTo(Succeed())
		})

		It("returns error on Deploy", func() {
			Expect(subject.Deploy(context.Background())).NotTo(Succeed())
		})

		It("returns error on Destroy", func() {
			Expect(subject.Destroy(context.Background())).NotTo(Succeed())
		})
	})

	Describe("buildAndPushServiceImages", func() {
		var project config.ComposeProject

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
			project = config.ComposeProject{Name: "test-project"}
		})

		It("translates a buildable service into image properties", func() {
			project.Services = types.Services{"hello": buildable("hello")}
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, []image.ServiceProperties{
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

			Expect(subject.BuildAndPushServiceImages(context.Background())).To(Succeed())
		})

		It("carries the platforms resolved from the extension", func() {
			svc := buildable("hello")
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"image": map[string]any{
						"platforms": []any{"linux/arm64", "linux/amd64"},
					},
				},
			}
			project.Services = types.Services{"hello": svc}
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			var got []image.ServiceProperties

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Run(func(_ context.Context, s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages(context.Background())).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Platforms).
				To(Equal([]string{"linux/arm64", "linux/amd64"}))
		})

		It("excludes a service flagged skip", func() {
			skipped := buildable("skipped")
			skipped.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{"skip": true},
			}
			project.Services = types.Services{
				"hello":   buildable("hello"),
				"skipped": skipped,
			}
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			var got []image.ServiceProperties

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Run(func(_ context.Context, s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages(context.Background())).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Name).To(Equal("hello"))
		})

		// mockImage carries no expectation in these two, so any call to
		// BuildAndPush fails the spec on cleanup.
		It("never calls the image client when nothing has a build block", func() {
			project.Services = types.Services{
				"hello": {Name: "hello", Image: "reg/hello:v1"},
			}
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			Expect(subject.BuildAndPushServiceImages(context.Background())).To(Succeed())
		})

		It("never calls the image client for an empty project", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())
			Expect(subject.BuildAndPushServiceImages(context.Background())).To(Succeed())
		})

		It("propagates an image client failure", func() {
			project.Services = types.Services{"hello": buildable("hello")}
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(errBake).
				Once()

			err := subject.BuildAndPushServiceImages(context.Background())

			Expect(err).To(MatchError(errBake))
		})
	})

	// Needs no cluster, so an unresolvable service fails here rather than
	// leaving a half-built map behind.
	Describe("initProject", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			project = config.ComposeProject{Name: "test-project"}
		})

		It("returns a config error when a service cannot be resolved", func() {
			// No image and no extension: resolution fails before any build.
			project.Services = types.Services{"broken": {Name: "broken"}}

			err := subject.InitProject(context.Background(), project)

			// resolveServiceImage accumulates its failures with errors.Join, so
			// the result is a join wrapper. errors.Is walks the join, which lets
			// this assert that *both* validation failures were collected.
			Expect(err).To(MatchError(config.ErrImageRepositoryMissing))
			Expect(err).To(MatchError(config.ErrImageTagMissing))
		})

		It("rejects a service with an unknown deployment type", func() {
			svc := config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					// Deliberately misspelled: the point is that an
					// unrecognized value is rejected rather than guessed at.
					//nolint:misspell // intentional typo under test
					"deploymentType": "sevice",
				},
			}
			project.Services = types.Services{"hello": svc}

			Expect(subject.InitProject(context.Background(), project)).
				To(MatchError(config.ErrInvalidDeploymentType))
		})

		It("resolves every service before any release is touched", func() {
			project.Services = types.Services{
				"hello": {Name: "hello", Image: "reg/hello:v1"},
				"job":   {Name: "job", Image: "reg/job:v1"},
			}

			Expect(subject.InitProject(context.Background(), project)).To(Succeed())
		})

		// initProject runs before any chart is installed, so a typo cannot
		// leave half a project up.
		Context("with an ngrok port matching no service port", func() {
			BeforeEach(func() {
				ngrokToken = "fake-token-for-tests"
			})

			It("rejects the project", func() {
				svc := config.ComposeService{
					Name:  "hello",
					Image: "reg/hello:v1",
					Ports: []types.ServicePortConfig{
						{Target: 8080, Published: "3000"},
					},
					Extensions: types.Extensions{
						config.K8sServiceExtension: map[string]any{
							"ngrok": map[string]any{"port": 9999},
						},
					},
				}
				project.Services = types.Services{"hello": svc}

				Expect(subject.InitProject(context.Background(), project)).
					To(MatchError(config.ErrNgrokPortMismatch))
			})
		})

		// helm uninstalls from the manifest it stored at install time, so a
		// file deleted since must not strand a namespace no one can remove.
		It("resolves a service whose manifest is no longer on disk", func() {
			svc := config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []string{"does-not-exist.yaml"},
				},
			}
			project.Services = types.Services{"hello": svc}

			Expect(subject.InitProject(context.Background(), project)).
				To(Succeed())
		})
	})

	Describe("service directories", func() {
		var (
			project  config.ComposeProject
			svcDir   string
			declared config.ComposeService
		)

		BeforeEach(func() {
			svcDir = GinkgoT().TempDir()
			declared = config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			declared.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []string{"k8s/missing.yaml"},
				},
			}
			project = config.ComposeProject{
				Name:     "test-project",
				Services: types.Services{"hello": declared},
			}
		})

		It("reads a service's files from the directory it is mapped to", func() {
			subject = helm.New(helm.Options{
				K8sExt:      k8sExt,
				ServiceDirs: map[string]string{"hello": svcDir},
				ImageClient: mockImage,
				GitClient:   mockGit,
			})

			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			err := subject.DeployService(context.Background(), declared)

			Expect(err).To(MatchError(helm.ErrManifestRead))
			Expect(err.Error()).To(ContainSubstring(
				filepath.Join(svcDir, "k8s/missing.yaml"),
			))
		})
	})

	Describe("PublishedServiceUrls", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			ngrokToken = "fake-token-for-tests"
			project = config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"beta":  exposedService("beta", 3001),
					"alpha": exposedService("alpha", 3000),
					"plain": {Name: "plain", Image: "reg/plain:v1"},
				},
			}
		})

		// The api lists every endpoint on the account, so the names asked
		// about are what keep another project's URLs out of the result.
		It("asks only about the services it exposed", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			var asked []string

			mockPublish.
				EXPECT().
				ServiceUrls(mock.Anything, mock.Anything).
				Run(func(_ context.Context, names []string) { asked = names }).
				Return(nil, nil).
				Once()

			_, err := subject.PublishedServiceUrls(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
			Expect(asked).To(ConsistOf("namespace-alpha", "namespace-beta"))
		})

		It("reports the url the api gives for each service", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			mockPublish.
				EXPECT().
				ServiceUrls(mock.Anything, mock.Anything).
				Return(map[string]url.URL{"namespace-alpha": *alphaURL}, nil).
				Once()

			Expect(subject.PublishedServiceUrls(context.Background())).
				To(Equal(map[string]url.URL{"namespace-alpha": *alphaURL}))
		})

		It("propagates an api failure", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			mockPublish.
				EXPECT().
				ServiceUrls(mock.Anything, mock.Anything).
				Return(nil, errBoom).
				Once()

			_, err := subject.PublishedServiceUrls(context.Background())

			Expect(err).To(MatchError(errBoom))
		})

		Context("with no auth token", func() {
			BeforeEach(func() {
				ngrokToken = ""
			})

			// resolveNgrok drops every ngrok block without a token, so there
			// is nothing to ask about — and a missing key is now an error.
			It("resolves nothing without calling the api", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())

				Expect(subject.PublishedServiceUrls(context.Background())).To(BeEmpty())

				mockPublish.AssertNotCalled(GinkgoT(), "ServiceUrls")
			})
		})
	})

	Describe("the endpoints it resolves to publish", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			ngrokToken = "fake-token-for-tests"
			// Out of order and in a map, so any ordering in the result has to
			// come from initProject's own sort.
			project = config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"beta":  exposedService("beta", 3001),
					"alpha": exposedService("alpha", 3000),
				},
			}
		})

		// The endpoint name is namespaced so two developers deploying the same
		// compose file cannot collide on the shared account, while the upstream
		// stays the bare service name k8s DNS resolves inside the namespace.
		It("namespaces each endpoint and keeps the bare service name", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			Expect(subject.ServicesToPublish()).To(HaveExactElements(
				config.NgrokEndpointConfig{
					Namespace:    "namespace",
					EndpointName: "namespace-alpha",
					ServiceName:  "alpha",
					Port:         3000,
				},
				config.NgrokEndpointConfig{
					Namespace:    "namespace",
					EndpointName: "namespace-beta",
					ServiceName:  "beta",
					Port:         3001,
				},
			))
		})

		// This ordering is what the chart's config checksum hashes. Unstable, it
		// replaces the agent pod on a deploy that changed nothing, reassigning
		// every unreserved url.
		It("resolves the same order every time", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())
			first := subject.ServicesToPublish()

			for range 5 {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())
				Expect(subject.ServicesToPublish()).To(Equal(first))
			}
		})

		// An upstream is a hostname *and* a port, and the agent dials outbound,
		// so a shared port is not a collision.
		It("gives two services on the same port their own endpoints", func() {
			project.Services = types.Services{
				"beta":  exposedService("beta", 8080),
				"alpha": exposedService("alpha", 8080),
			}

			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			Expect(subject.ServicesToPublish()).To(HaveExactElements(
				HaveField("EndpointName", "namespace-alpha"),
				HaveField("EndpointName", "namespace-beta"),
			))
		})

		Context("resolved for a different namespace", func() {
			BeforeEach(func() {
				k8sExt.Namespace = "other-namespace"
			})

			It("names each endpoint after that namespace", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())

				Expect(subject.ServicesToPublish()).To(ContainElement(
					HaveField("EndpointName", "other-namespace-alpha"),
				))
			})
		})

		Context("with a service declaring no ngrok block", func() {
			BeforeEach(func() {
				project.Services = types.Services{
					"plain": {Name: "plain", Image: "reg/plain:v1"},
				}
			})

			It("publishes nothing for it", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())
				Expect(subject.ServicesToPublish()).To(BeEmpty())
			})
		})

		Context("with a service flagged skip", func() {
			BeforeEach(func() {
				skipped := exposedService("skipped", 3000)
				skipped.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"skip":  true,
						"ngrok": map[string]any{"port": 3000},
					},
				}
				project.Services = types.Services{"skipped": skipped}
			})

			// Never installed, so its endpoint would have no upstream.
			It("publishes nothing for it", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())
				Expect(subject.ServicesToPublish()).To(BeEmpty())
			})
		})

		Context("with a job carrying ngrok config", func() {
			BeforeEach(func() {
				job := exposedService("job", 3000)
				job.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"deploymentType": "job",
						"ngrok":          map[string]any{"port": 3000},
					},
				}
				project.Services = types.Services{"job": job}
			})

			// A job renders no Service for an upstream to route to.
			It("publishes nothing for it", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())
				Expect(subject.ServicesToPublish()).To(BeEmpty())
			})
		})

		Context("with no auth token", func() {
			BeforeEach(func() {
				ngrokToken = ""
			})

			It("publishes nothing even for a configured service", func() {
				Expect(subject.InitProject(context.Background(), project)).To(Succeed())
				Expect(subject.ServicesToPublish()).To(BeEmpty())
			})
		})

		// ngrok binds only a domain reserved on the account, and an assigned
		// url is ephemeral, so only a configured one may reach the config.
		It("never adopts the url the api reports", func() {
			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			Expect(subject.ServicesToPublish()).To(HaveEach(
				HaveField("URL", BeEmpty()),
			))
			mockPublish.AssertNotCalled(GinkgoT(), "ServiceUrls")
		})

		It("keeps a configured url", func() {
			svc := exposedService("alpha", 3000)
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"ngrok": map[string]any{
						"port": 3000,
						"url":  "https://reserved.ngrok.app",
					},
				},
			}
			project.Services = types.Services{"alpha": svc}

			Expect(subject.InitProject(context.Background(), project)).To(Succeed())

			Expect(subject.ServicesToPublish()).To(ConsistOf(
				HaveField("URL", "https://reserved.ngrok.app"),
			))
			mockPublish.AssertNotCalled(GinkgoT(), "ServiceUrls")
		})
	})
})
