package deployer_test

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/image"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	"github.com/stretchr/testify/mock"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
	helmengine "helm.sh/helm/v3/pkg/engine"
)

// render runs the assembled chart through helm's template engine with the
// given values, returning each rendered template keyed by its path.
func render(
	chart *helmchart.Chart,
	values map[string]any,
) map[string]string {
	GinkgoHelper()

	renderValues, err := helmchartutil.ToRenderValues(
		chart,
		values,
		helmchartutil.ReleaseOptions{
			Name:      chart.Name(),
			Namespace: "namespace",
			IsInstall: true,
		},
		nil,
	)
	Expect(err).ShouldNot(HaveOccurred())

	out, err := helmengine.Render(chart, renderValues)
	Expect(err).ShouldNot(HaveOccurred())

	return out
}

// templateNamed finds a rendered template by its file suffix.
func templateNamed(rendered map[string]string, suffix string) string {
	GinkgoHelper()

	for name, body := range rendered {
		if strings.HasSuffix(name, suffix) {
			return body
		}
	}

	Fail("no rendered template ending in " + suffix)
	return ""
}

// The image-client failure the fake returns. Asserted by identity so the spec
// proves BuildAndPushServiceImages passes the error through untouched.
var errBake = errors.New("bake blew up")

var _ = Describe("Helm", func() {
	var (
		ext          *config.XMiniEnv
		mockImage    *imagemocks.MockClient
		mockGit      *gitmocks.MockClient
		ngrokEnabled bool
		subject      *deployer.Helm
	)

	// Assembled lazily: specs mutate ext, svc and ngrokEnabled in their own
	// bodies before resolving.
	extOpts := func(svc config.ComposeService) config.XMiniEnvK8sServiceOptions {
		return config.XMiniEnvK8sServiceOptions{
			MainExt:      ext,
			Service:      svc,
			GitClient:    mockGit,
			NgrokEnabled: ngrokEnabled,
		}
	}

	// A fixed fake. The real NGROK_AUTHTOKEN is only ever read at the
	// composition root, so no test binary can pick up a live credential and
	// serialize it into a rendered chart or a failure report.
	const fakeNgrokToken = "fake-token-for-tests"

	BeforeEach(func() {
		ext = &config.XMiniEnv{
			K8s: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
		}
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
		ngrokEnabled = false
		subject = deployer.NewHelm(deployer.HelmOptions{
			Ext:         ext,
			ImageClient: mockImage,
			GitClient:   mockGit,
			DryRun:      false,
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
			Expect(subject.Deploy(project)).To(Succeed())
		})

		It("does nothing on Destroy", func() {
			Expect(subject.Destroy(project)).To(Succeed())
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

			Expect(subject.BuildAndPushServiceImages(project)).To(Succeed())
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

			var got []image.ServiceProperties
			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Run(func(s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages(project)).To(Succeed())
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

			var got []image.ServiceProperties
			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Run(func(s []image.ServiceProperties) { got = s }).
				Return(nil).
				Once()

			Expect(subject.BuildAndPushServiceImages(project)).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Name).To(Equal("hello"))
		})

		// mockImage carries no expectation in these two, so any call to
		// BuildAndPush fails the spec on cleanup.
		It("never calls the image client when nothing has a build block", func() {
			project.Services = types.Services{
				"hello": {Name: "hello", Image: "reg/hello:v1"},
			}

			Expect(subject.BuildAndPushServiceImages(project)).To(Succeed())
		})

		It("never calls the image client for an empty project", func() {
			Expect(subject.BuildAndPushServiceImages(project)).To(Succeed())
		})

		It("propagates an image client failure", func() {
			project.Services = types.Services{"hello": buildable("hello")}

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything).
				Return(errBake).
				Once()

			err := subject.BuildAndPushServiceImages(project)

			Expect(err).To(MatchError(errBake))
		})

		It("returns a config error when a service cannot be resolved", func() {
			// No image and no extension: resolution fails before any build.
			project.Services = types.Services{"broken": {Name: "broken"}}

			err := subject.BuildAndPushServiceImages(project)

			// resolveServiceImage accumulates its failures with errors.Join, so
			// the result is a join wrapper. errors.Is walks the join, which lets
			// this assert that *both* validation failures were collected.
			Expect(err).To(MatchError(config.KindImageRepositoryMissing))
			Expect(err).To(MatchError(config.KindImageTagMissing))
		})
	})

	Describe("LoadChart", func() {
		It("assembles every chart file in memory", func() {
			chart, err := subject.LoadChart("hello", "")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Name()).To(Equal("hello"))
			Expect(chart.Metadata.Version).To(Equal("0.1.0"))

			names := []string{}
			for _, f := range chart.Raw {
				names = append(names, f.Name)
			}
			Expect(names).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/deployment.yaml",
				"templates/service.yaml",
				"templates/serviceaccount.yaml",
				"templates/configmap.yaml",
				"templates/secret.yaml",
			))
		})

		It("produces a chart helm can validate", func() {
			chart, err := subject.LoadChart("hello", "")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Validate()).To(Succeed())
		})

		Context("rendered against resolved service values", func() {
			var (
				svc      config.ComposeService
				values   map[string]any
				rendered map[string]string
			)

			BeforeEach(func() {
				svc = config.ComposeService{
					Name:  "hello",
					Image: "reg/hello:v1",
					Ports: []types.ServicePortConfig{
						{Target: 8080, Published: "3000"},
					},
				}
			})

			JustBeforeEach(func() {
				svcExt, err := config.NewXMiniEnvK8sService(extOpts(svc))
				Expect(err).ShouldNot(HaveOccurred())

				values, err = svcExt.ToValuesMap()
				Expect(err).ShouldNot(HaveOccurred())

				chart, err := subject.LoadChart("hello", fakeNgrokToken)
				Expect(err).ShouldNot(HaveOccurred())

				rendered = render(chart, values)
			})

			It("renders the resolved image into the deployment", func() {
				Expect(templateNamed(rendered, "deployment.yaml")).
					To(ContainSubstring("image: \"reg/hello:v1\""))
			})

			It("renders the resolved container port", func() {
				deployment := templateNamed(rendered, "deployment.yaml")

				Expect(deployment).To(ContainSubstring("name: p8080"))
				Expect(deployment).To(ContainSubstring("containerPort: 8080"))
			})

			It("renders the resolved service port", func() {
				service := templateNamed(rendered, "service.yaml")

				Expect(service).To(ContainSubstring("port: 3000"))
				Expect(service).To(ContainSubstring("targetPort: p8080"))
			})

			Context("with replicas set in the extension", func() {
				BeforeEach(func() {
					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"replicas": 3,
						},
					}
				})

				It("renders the configured replica count", func() {
					Expect(templateNamed(rendered, "deployment.yaml")).
						To(ContainSubstring("replicas: 3"))
				})
			})

			Context("with ngrok disabled", func() {
				It("renders no configmap or secret body", func() {
					Expect(
						strings.TrimSpace(templateNamed(rendered, "configmap.yaml")),
					).To(BeEmpty())
					Expect(
						strings.TrimSpace(templateNamed(rendered, "secret.yaml")),
					).To(BeEmpty())
				})
			})

			Context("with ngrok enabled", func() {
				BeforeEach(func() {
					ngrokEnabled = true

					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"ngrok": map[string]any{
								"port":          3000,
								"url":           "https://example.ngrok.app",
								"trafficPolicy": "on_http_request: []",
							},
						},
					}
				})

				It("names the configmap from the shared constant", func() {
					configMap := templateNamed(rendered, "configmap.yaml")

					Expect(configMap).To(ContainSubstring(
						"name: " + config.NGROK_CONFIG_MAP_NAME,
					))
					Expect(configMap).To(ContainSubstring(
						config.NGROK_CONFIG_KEY + ": |",
					))
					Expect(configMap).To(
						ContainSubstring("url: https://example.ngrok.app"),
					)
				})

				It("names the secret from the shared constant", func() {
					Expect(templateNamed(rendered, "secret.yaml")).
						To(ContainSubstring("name: " + config.NGROK_SECRET_NAME))
				})

				It("renders the ngrok sidecar with a real image and mount", func() {
					deployment := templateNamed(rendered, "deployment.yaml")

					Expect(deployment).To(
						ContainSubstring("image: " + config.NGROK_IMAGE),
					)
					Expect(deployment).To(ContainSubstring(
						"mountPath: " + config.NGROK_CONFIG_VOL_MOUNT_PATH,
					))
					Expect(deployment).To(
						ContainSubstring("name: " + config.NGROK_SECRET_NAME),
					)
				})
			})
		})

		It("base64 encodes the ngrok auth token into the secret", func() {
			chart, err := subject.LoadChart("hello", fakeNgrokToken)
			Expect(err).ShouldNot(HaveOccurred())

			var secret string
			for _, f := range chart.Raw {
				if f.Name == "templates/secret.yaml" {
					secret = string(f.Data)
				}
			}

			Expect(secret).To(ContainSubstring(
				base64.StdEncoding.EncodeToString([]byte(fakeNgrokToken)),
			))
			Expect(secret).ToNot(ContainSubstring(fakeNgrokToken))
		})
	})
})
