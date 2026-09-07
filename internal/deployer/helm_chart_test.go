package deployer_test

import (
	"encoding/base64"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

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

// The name is randomized per render, so specs need the value itself.
func jobName(rendered map[string]string) string {
	GinkgoHelper()

	match := jobNamePattern.FindStringSubmatch(templateNamed(rendered, "job.yaml"))
	Expect(match).To(HaveLen(2), "no quoted metadata.name in job.yaml")

	return match[1]
}

var jobNamePattern = regexp.MustCompile(`(?m)^  name: "([^"]+)"$`)

// Parsed rather than substring-matched: a value at the wrong indentation still
// contains every expected substring while no longer being valid YAML.
type ngrokAgentConfig struct {
	Version   int `yaml:"version"`
	Endpoints []struct {
		Name        string `yaml:"name"`
		Url         string `yaml:"url"`
		Description string `yaml:"description"`
		Upstream    struct {
			Url string `yaml:"url"`
		} `yaml:"upstream"`
		TrafficPolicy map[string]any `yaml:"traffic_policy"`
	} `yaml:"endpoints"`
}

func ngrokAgentConfigFrom(rendered map[string]string) ngrokAgentConfig {
	GinkgoHelper()

	var configMap struct {
		Data map[string]string `yaml:"data"`
	}

	Expect(yaml.Unmarshal(
		[]byte(templateNamed(rendered, "configmap.yaml")),
		&configMap,
	)).To(Succeed(), "rendered configmap is not valid yaml")

	body, ok := configMap.Data[config.NGROK_CONFIG_KEY]
	Expect(ok).To(BeTrue(), "configmap carries no %s key", config.NGROK_CONFIG_KEY)

	var parsed ngrokAgentConfig
	Expect(yaml.Unmarshal([]byte(body), &parsed)).
		To(Succeed(), "rendered %s is not valid yaml", config.NGROK_CONFIG_KEY)

	return parsed
}

func podAnnotation(rendered map[string]string, key string) string {
	GinkgoHelper()

	var deployment struct {
		Spec struct {
			Template struct {
				Metadata struct {
					Annotations map[string]string `yaml:"annotations"`
				} `yaml:"metadata"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}

	Expect(yaml.Unmarshal(
		[]byte(templateNamed(rendered, "deployment.yaml")),
		&deployment,
	)).To(Succeed(), "rendered deployment is not valid yaml")

	value, ok := deployment.Spec.Template.Metadata.Annotations[key]
	Expect(ok).To(BeTrue(), "pod template carries no %s annotation", key)

	return value
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
			// ngrok lives in its own release for the namespace.
			Expect(names).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/deployment.yaml",
				"templates/service.yaml",
				"templates/serviceaccount.yaml",
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

			Context("with pod annotations set", func() {
				BeforeEach(func() {
					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"podAnnotations": map[string]any{
								"team": "platform",
							},
						},
					}
				})

				It("renders them onto the pod template", func() {
					Expect(podAnnotation(rendered, "team")).
						To(Equal("platform"))
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

		It("names the chart after the service so destroy can find it", func() {
			first, err := subject.LoadChart("hello", config.K8sJobDeploymentType)
			Expect(err).ShouldNot(HaveOccurred())

			second, err := subject.LoadChart("hello", config.K8sJobDeploymentType)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(first.Name()).To(Equal("hello"))
			Expect(second.Name()).To(Equal(first.Name()))
		})

		// A Job's spec.template and spec.selector are immutable, so an upgrade
		// cannot patch one in place.
		It("renders a new job name on every render", func() {
			chart, err := subject.LoadChart("hello", config.K8sJobDeploymentType)
			Expect(err).ShouldNot(HaveOccurred())

			values := map[string]any{}

			first := jobName(render(chart, values))
			second := jobName(render(chart, values))

			Expect(first).To(MatchRegexp(`^hello-[a-z0-9]{8}$`))
			Expect(second).To(MatchRegexp(`^hello-[a-z0-9]{8}$`))
			Expect(second).ToNot(Equal(first))
		})

		// The suffix is appended after generated.fullname already truncated to
		// 63, and the Job controller rejects anything longer.
		It("keeps a long service name within the job name limit", func() {
			longName := strings.Repeat("a", 60)

			chart, err := subject.LoadChart(
				longName,
				config.K8sJobDeploymentType,
			)
			Expect(err).ShouldNot(HaveOccurred())

			name := jobName(render(chart, map[string]any{}))

			Expect(len(name)).To(BeNumerically("<=", 63))
			Expect(name).To(MatchRegexp(`-[a-z0-9]{8}$`))
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
		})
	})

	Describe("NgrokChart", func() {
		var project *config.ComposeProject

		BeforeEach(func() {
			ngrokToken = fakeNgrokToken

			// Out of order and in a map, so any ordering must come from
			// initProject's own sort.
			project = &config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"beta":  exposedService("beta", 3001),
					"alpha": exposedService("alpha", 3000),
				},
			}
		})

		JustBeforeEach(func() {
			Expect(subject.InitProject(project)).To(Succeed())
		})

		It("assembles only the files the ngrok release needs", func() {
			chart, err := subject.NgrokChart()
			Expect(err).ShouldNot(HaveOccurred())

			names := []string{}
			for _, f := range chart.Raw {
				names = append(names, f.Name)
			}

			// No service.yaml: the agent dials out.
			Expect(names).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/deployment.yaml",
				"templates/serviceaccount.yaml",
				"templates/configmap.yaml",
				"templates/secret.yaml",
			))
		})

		It("names the chart so destroy can find the release", func() {
			chart, err := subject.NgrokChart()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Name()).To(Equal("ngrok"))
		})

		It("produces a chart helm can validate", func() {
			chart, err := subject.NgrokChart()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Validate()).To(Succeed())
		})

		Context("with nothing to publish", func() {
			BeforeEach(func() {
				project.Services = types.Services{
					"plain": {Name: "plain", Image: "reg/plain:v1"},
				}
			})

			It("resolves no values", func() {
				Expect(subject.NgrokValues()).To(BeNil())
			})
		})

		Context("with a service flagged skip", func() {
			BeforeEach(func() {
				skipped := exposedService("skipped", 3000)
				skipped.Extensions = types.Extensions{
					config.K8S_SERVICE_EXTENSION: map[string]any{
						"skip":  true,
						"ngrok": map[string]any{"port": 3000},
					},
				}
				project.Services = types.Services{"skipped": skipped}
			})

			// Never installed, so its endpoint would have no upstream.
			It("publishes nothing for it", func() {
				Expect(subject.NgrokValues()).To(BeNil())
			})
		})

		Context("with no auth token", func() {
			BeforeEach(func() {
				ngrokToken = ""
			})

			It("resolves no values even for a configured service", func() {
				Expect(subject.NgrokValues()).To(BeNil())
			})
		})

		Context("with a job carrying ngrok config", func() {
			BeforeEach(func() {
				job := exposedService("job", 3000)
				job.Extensions = types.Extensions{
					config.K8S_SERVICE_EXTENSION: map[string]any{
						"deploymentType": "job",
						"ngrok":          map[string]any{"port": 3000},
					},
				}
				project.Services = types.Services{"job": job}
			})

			// A job renders no Service for an upstream to route to.
			It("publishes nothing for it", func() {
				Expect(subject.NgrokValues()).To(BeNil())
			})
		})

		Context("rendered against the resolved endpoints", func() {
			var rendered map[string]string

			renderNgrok := func() map[string]string {
				GinkgoHelper()

				chart, err := subject.NgrokChart()
				Expect(err).ShouldNot(HaveOccurred())

				return render(chart, subject.NgrokValues())
			}

			JustBeforeEach(func() {
				rendered = renderNgrok()
			})

			It("publishes every configured service from one config", func() {
				cfg := ngrokAgentConfigFrom(rendered)

				Expect(cfg.Version).To(Equal(3))
				Expect(cfg.Endpoints).To(HaveLen(2))

				// The endpoint name is namespaced so two environments cannot
				// collide on the account, while the upstream stays the bare
				// service name that k8s DNS resolves inside the namespace.
				//
				// Sorted, not map order: this feeds the checksum.
				Expect(cfg.Endpoints[0].Name).To(Equal("namespace-alpha"))
				Expect(cfg.Endpoints[0].Upstream.Url).To(Equal("alpha:3000"))
				Expect(cfg.Endpoints[1].Name).To(Equal("namespace-beta"))
				Expect(cfg.Endpoints[1].Upstream.Url).To(Equal("beta:3001"))
			})

			It("omits the url when no domain is reserved", func() {
				for _, ep := range ngrokAgentConfigFrom(rendered).Endpoints {
					Expect(ep.Url).To(BeEmpty())
				}
			})

			// The agent reads ngrok.yml only at startup, so without this the
			// ConfigMap updates and the agent serves the previous set.
			It("changes the config checksum when an endpoint is added", func() {
				before := podAnnotation(rendered, "checksum/config")

				project.Services["gamma"] = exposedService("gamma", 3002)
				Expect(subject.InitProject(project)).To(Succeed())

				Expect(podAnnotation(renderNgrok(), "checksum/config")).
					ToNot(Equal(before))
			})

			It("changes the config checksum when a port changes", func() {
				before := podAnnotation(rendered, "checksum/config")

				project.Services["alpha"] = exposedService("alpha", 3009)
				Expect(subject.InitProject(project)).To(Succeed())

				Expect(podAnnotation(renderNgrok(), "checksum/config")).
					ToNot(Equal(before))
			})

			// A deploy that changed nothing must not replace the pod, or every
			// unreserved URL is reassigned. Repeating exercises the sort.
			It("keeps the config checksum stable across re-resolution", func() {
				before := podAnnotation(rendered, "checksum/config")

				for range 5 {
					Expect(subject.InitProject(project)).To(Succeed())

					Expect(podAnnotation(renderNgrok(), "checksum/config")).
						To(Equal(before))
				}
			})

			// Nothing else in the pod template changes on rotation.
			It("changes the secret checksum when the token rotates", func() {
				before := podAnnotation(rendered, "checksum/secret")

				rotated := deployer.NewHelm(deployer.HelmOptions{
					Ext:            ext,
					ImageClient:    mockImage,
					GitClient:      mockGit,
					NgrokAuthToken: "a-different-token",
					DryRun:         false,
				})
				Expect(rotated.InitProject(project)).To(Succeed())

				chart, err := rotated.NgrokChart()
				Expect(err).ShouldNot(HaveOccurred())

				Expect(podAnnotation(
					render(chart, rotated.NgrokValues()), "checksum/secret",
				)).ToNot(Equal(before))
			})

			It("runs one agent for every endpoint at once", func() {
				deployment := templateNamed(rendered, "deployment.yaml")

				Expect(deployment).To(ContainSubstring("- --all"))
				Expect(deployment).To(ContainSubstring(
					"image: \"" + config.NGROK_IMAGE_REPO +
						":" + config.NGROK_IMAGE_TAG + "\"",
				))
				Expect(deployment).To(ContainSubstring(
					"mountPath: " + config.NGROK_CONFIG_VOL_MOUNT_PATH,
				))
				Expect(deployment).To(ContainSubstring(
					"name: " + config.NGROK_SECRET_NAME,
				))
			})

			// RollingUpdate would overlap two agents claiming the same URLs.
			It("replaces the agent rather than rolling it", func() {
				Expect(templateNamed(rendered, "deployment.yaml")).
					To(ContainSubstring("type: Recreate"))
			})

			It("base64 encodes the ngrok auth token into the secret", func() {
				chart, err := subject.NgrokChart()
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

			// An upstream is a hostname *and* a port, and the agent dials
			// outbound. compose.yml keeps hello and hello2 both on 8080, and
			// this spec is what says that is deliberate.
			Context("with two services publishing the same port", func() {
				BeforeEach(func() {
					project.Services = types.Services{
						"beta":  exposedService("beta", 8080),
						"alpha": exposedService("alpha", 8080),
					}
				})

				It("gives each its own endpoint and upstream", func() {
					cfg := ngrokAgentConfigFrom(rendered)

					Expect(cfg.Endpoints).To(HaveLen(2))
					Expect(cfg.Endpoints[0].Name).To(Equal("namespace-alpha"))
					Expect(cfg.Endpoints[0].Upstream.Url).To(Equal("alpha:8080"))
					Expect(cfg.Endpoints[1].Name).To(Equal("namespace-beta"))
					Expect(cfg.Endpoints[1].Upstream.Url).To(Equal("beta:8080"))
				})
			})

			// The whole point of namespacing the endpoint name: two developers
			// deploying the same compose file must not collide on the shared
			// ngrok account. The upstream is identical in both, which is fine
			// because each resolves inside its own namespace.
			Context("resolved for a different namespace", func() {
				It("names the endpoint after that namespace", func() {
					ext.K8s.Namespace = "other-namespace"
					other := deployer.NewHelm(deployer.HelmOptions{
						Ext:            ext,
						ImageClient:    mockImage,
						GitClient:      mockGit,
						NgrokAuthToken: ngrokToken,
						DryRun:         false,
					})
					Expect(other.InitProject(project)).To(Succeed())

					chart, err := other.NgrokChart()
					Expect(err).ShouldNot(HaveOccurred())

					cfg := ngrokAgentConfigFrom(
						render(chart, other.NgrokValues()),
					)

					Expect(cfg.Endpoints[0].Name).
						To(Equal("other-namespace-alpha"))
					Expect(cfg.Endpoints[0].Upstream.Url).To(Equal("alpha:3000"))
				})
			})

			Context("with a reserved domain", func() {
				BeforeEach(func() {
					svc := exposedService("alpha", 3000)
					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"ngrok": map[string]any{
								"port": 3000,
								"url":  "https://example.ngrok.app",
							},
						},
					}
					project.Services = types.Services{"alpha": svc}
				})

				It("pins the endpoint to it", func() {
					cfg := ngrokAgentConfigFrom(rendered)

					Expect(cfg.Endpoints).To(HaveLen(1))
					Expect(cfg.Endpoints[0].Url).
						To(Equal("https://example.ngrok.app"))
				})
			})

			Context("with a multi-line traffic policy", func() {
				BeforeEach(func() {
					svc := exposedService("alpha", 3000)
					svc.Extensions = types.Extensions{
						config.K8S_SERVICE_EXTENSION: map[string]any{
							"ngrok": map[string]any{
								"port": 3000,
								"trafficPolicy": "on_http_request:\n" +
									"  - actions:\n" +
									"      - type: deny\n",
							},
						},
					}
					project.Services = types.Services{"alpha": svc}
				})

				// The documented form is a block scalar; interpolated flat it
				// breaks out of the ConfigMap's own block.
				It("nests the policy under the endpoint as yaml", func() {
					cfg := ngrokAgentConfigFrom(rendered)

					Expect(cfg.Endpoints).To(HaveLen(1))
					Expect(cfg.Endpoints[0].TrafficPolicy).
						To(HaveKey("on_http_request"))
				})
			})

			Context("with no traffic policy", func() {
				It("omits the traffic policy key", func() {
					for _, ep := range ngrokAgentConfigFrom(rendered).Endpoints {
						Expect(ep.TrafficPolicy).To(BeNil())
					}
				})
			})
		})
	})
})
