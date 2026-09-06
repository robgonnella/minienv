package deployer_test

import (
	"encoding/base64"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
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

// A fixed fake. The real NGROK_AUTHTOKEN is only ever read at the
// composition root, so no test binary can pick up a live credential and
// serialize it into a rendered chart or a failure report.
const fakeNgrokToken = "fake-token-for-tests"

var _ = Describe("Helm", func() {
	var (
		ext        *config.XMiniEnv
		mockImage  *imagemocks.MockClient
		mockGit    *gitmocks.MockClient
		subject    *deployer.Helm
		ngrokToken string
	)

	// Assembled lazily: specs mutate ext, svc and ngrokToken in their own
	// bodies before resolving.
	extOpts := func(
		svc config.ComposeService,
	) config.XMiniEnvK8sServiceOptions {
		return config.XMiniEnvK8sServiceOptions{
			MainExt:      ext,
			Service:      svc,
			GitClient:    mockGit,
			NgrokEnabled: ngrokToken != "",
		}
	}

	BeforeEach(func() {
		ext = &config.XMiniEnv{
			K8s: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
		}
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
		// Reset explicitly: the suite runs --randomize-all, so a token left set
		// by the ngrok specs would silently enable ngrok for whatever ran next.
		ngrokToken = ""
	})

	JustBeforeEach(func() {
		subject = deployer.NewHelm(deployer.HelmOptions{
			Ext:            ext,
			ImageClient:    mockImage,
			GitClient:      mockGit,
			NgrokAuthToken: ngrokToken,
			DryRun:         false,
		})
	})

	Describe("LoadChart (service)", func() {
		It("assembles every chart file in memory", func() {
			chart, err := subject.LoadChart(
				"hello",
				config.K8sServiceDeploymentType,
			)

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
			chart, err := subject.LoadChart(
				"hello",
				config.K8sServiceDeploymentType,
			)

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

				values, err = svcExt.ToChartValuesMap()
				Expect(err).ShouldNot(HaveOccurred())

				chart, err := subject.LoadChart(
					"hello",
					config.K8sServiceDeploymentType,
				)
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

			// The end of the path resolveServicePorts starts: with no ports there
			// is nothing to route to, so neither resource should exist. This is
			// what the *bool on Create buys, and it only holds if the value
			// reaches the template as a plain false rather than a pointer.
			Context("with no ports declared", func() {
				BeforeEach(func() {
					svc.Ports = nil
				})

				It("renders no service or service account", func() {
					Expect(strings.TrimSpace(
						templateNamed(rendered, "service.yaml"),
					)).To(BeEmpty())
					Expect(strings.TrimSpace(
						templateNamed(rendered, "serviceaccount.yaml"),
					)).To(BeEmpty())
				})
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
					ngrokToken = fakeNgrokToken

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

				It("base64 encodes the ngrok auth token into the secret", func() {
					chart, err := subject.LoadChart(
						"hello",
						config.K8sServiceDeploymentType,
					)
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
	})

	Describe("LoadChart (job)", func() {
		It("assembles only the files a job needs", func() {
			chart, err := subject.LoadChart(
				"hello",
				config.K8sJobDeploymentType,
			)

			Expect(err).ShouldNot(HaveOccurred())

			names := []string{}
			for _, f := range chart.Raw {
				names = append(names, f.Name)
			}

			// No service, and no ngrok configmap or secret: a job serves no
			// traffic, so there is nothing to expose and no token to mount.
			Expect(names).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/serviceaccount.yaml",
				"templates/job.yaml",
			))
		})

		It("produces a chart that validates", func() {
			chart, err := subject.LoadChart(
				"hello",
				config.K8sJobDeploymentType,
			)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Validate()).To(Succeed())
		})

		// A Job's spec.template and spec.selector are immutable, so an upgrade
		// cannot patch one in place. Naming the chart after a throwaway id is
		// what makes every deploy produce a new Job object instead — the same
		// reason upgradeChart sets Force and Recreate.
		It("names the chart a throwaway id rather than the service", func() {
			first, err := subject.LoadChart("hello", config.K8sJobDeploymentType)
			Expect(err).ShouldNot(HaveOccurred())

			second, err := subject.LoadChart("hello", config.K8sJobDeploymentType)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(first.Name()).ToNot(Equal("hello"))
			Expect(first.Name()).To(MatchRegexp(`^[a-z0-9]{8}$`))
			Expect(second.Name()).ToNot(Equal(first.Name()))
		})

		Context("rendered against resolved job values", func() {
			var (
				svc      config.ComposeService
				values   map[string]any
				chart    *helmchart.Chart
				rendered map[string]string
			)

			BeforeEach(func() {
				svc = config.ComposeService{
					Name:    "migrate",
					Image:   "reg/migrate:v1",
					Command: types.ShellCommand{"/bin/sh", "-c", "echo ready"},
					Extensions: types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"deploymentType": "job",
						},
					},
				}
			})

			JustBeforeEach(func() {
				svcExt, err := config.NewXMiniEnvK8sService(extOpts(svc))
				Expect(err).ShouldNot(HaveOccurred())
				Expect(svcExt.DeploymentType).
					To(Equal(config.K8sJobDeploymentType))

				values, err = svcExt.ToChartValuesMap()
				Expect(err).ShouldNot(HaveOccurred())

				chart, err = subject.LoadChart("migrate", svcExt.DeploymentType)
				Expect(err).ShouldNot(HaveOccurred())

				rendered = render(chart, values)
			})

			It("renders a batch job that never restarts", func() {
				job := templateNamed(rendered, "job.yaml")

				Expect(job).To(ContainSubstring("apiVersion: batch/v1"))
				Expect(job).To(ContainSubstring("kind: Job"))
				Expect(job).To(ContainSubstring("backoffLimit: 1"))
				Expect(job).To(ContainSubstring("restartPolicy: Never"))
			})

			It("renders the resolved image", func() {
				Expect(templateNamed(rendered, "job.yaml")).
					To(ContainSubstring("image: \"reg/migrate:v1\""))
			})

			It("renders the command resolved from the compose service", func() {
				job := templateNamed(rendered, "job.yaml")

				Expect(job).To(ContainSubstring("command:"))
				Expect(job).To(ContainSubstring("- /bin/sh"))
				Expect(job).To(ContainSubstring("- echo ready"))
			})

			It("names the container after the generated chart name", func() {
				Expect(templateNamed(rendered, "job.yaml")).
					To(ContainSubstring("name: " + chart.Name()))
			})

			It("creates no service account for a job with no ports", func() {
				Expect(strings.TrimSpace(
					templateNamed(rendered, "serviceaccount.yaml"),
				)).To(BeEmpty())
			})

			It("renders no ports for a job that declares none", func() {
				Expect(templateNamed(rendered, "job.yaml")).
					ToNot(ContainSubstring("containerPort"))
			})

			Context("with an environment and a port", func() {
				BeforeEach(func() {
					svc.Environment = types.MappingWithEquals{
						"DATABASE_URL": new("postgres://db"),
					}
					svc.Ports = []types.ServicePortConfig{
						{Target: 8080, Published: "3000"},
					}
				})

				It("renders the container port", func() {
					job := templateNamed(rendered, "job.yaml")

					Expect(job).To(ContainSubstring("name: p8080"))
					Expect(job).To(ContainSubstring("containerPort: 8080"))
				})

				It("renders the resolved environment", func() {
					job := templateNamed(rendered, "job.yaml")

					Expect(job).To(ContainSubstring("name: DATABASE_URL"))
					Expect(job).To(ContainSubstring("value: postgres://db"))
				})
			})

			Context("with ngrok enabled and configured", func() {
				BeforeEach(func() {
					ngrokToken = fakeNgrokToken

					svc.Ports = []types.ServicePortConfig{
						{Target: 8080, Published: "3000"},
					}
					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"deploymentType": "job",
							"ngrok": map[string]any{
								"port": 3000,
								"url":  "https://example.ngrok.app",
							},
						},
					}
				})

				// A job has no service to route to, so neither the sidecar nor
				// the token it needs may reach the rendered pod, even with ngrok
				// fully configured for the service.
				It("renders no ngrok sidecar and carries no secret", func() {
					Expect(templateNamed(rendered, "job.yaml")).
						ToNot(ContainSubstring(config.NGROK_IMAGE))

					for _, f := range chart.Raw {
						Expect(string(f.Data)).
							ToNot(ContainSubstring(fakeNgrokToken))
						Expect(f.Name).ToNot(Equal("templates/secret.yaml"))
						Expect(f.Name).ToNot(Equal("templates/configmap.yaml"))
					}
				})
			})
		})
	})
})
