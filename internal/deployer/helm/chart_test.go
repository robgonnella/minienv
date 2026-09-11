package helm_test

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
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

// fileNames lists the chart's raw file set, which is what says a chart carries
// only the templates its deployment type needs.
func fileNames(chart *helmchart.Chart) []string {
	names := make([]string, 0, len(chart.Raw))
	for _, f := range chart.Raw {
		names = append(names, f.Name)
	}

	return names
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
		URL         string `yaml:"url"`
		Description string `yaml:"description"`
		Upstream    struct {
			URL string `yaml:"url"`
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

	body, ok := configMap.Data[config.NgrokConfigKey]
	Expect(ok).To(BeTrue(), "configmap carries no %s key", config.NgrokConfigKey)

	var parsed ngrokAgentConfig
	Expect(yaml.Unmarshal([]byte(body), &parsed)).
		To(Succeed(), "rendered %s is not valid yaml", config.NgrokConfigKey)

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

// publishable is one endpoint shaped the way initProject emits them. Built by
// hand so these specs state exactly what reaches the templates; which services
// earn an endpoint, and in what order, is asserted against initProject itself.
func publishable(name string, port uint16) config.NgrokEndpointConfig {
	return config.NgrokEndpointConfig{
		Namespace:    "namespace",
		EndpointName: "namespace-" + name,
		ServiceName:  name,
		Port:         port,
	}
}

var _ = Describe("ChartBuilder", func() {
	var (
		subject *helm.ChartBuilder
		k8sExt  config.XMiniEnvK8s
		mockGit *gitmocks.MockClient
	)

	// Specs render against values production actually produces rather than a
	// hand-written map, so a key the templates read cannot drift from the one
	// ToChartValuesMap writes.
	resolvedValues := func(svc config.ComposeService) map[string]any {
		GinkgoHelper()

		svcExt, err := config.NewXMiniEnvK8sService(
			context.Background(),
			config.XMiniEnvK8sServiceOptions{
				K8sExt:       k8sExt,
				Service:      svc,
				GitClient:    mockGit,
				NgrokEnabled: false,
			},
		)
		Expect(err).ShouldNot(HaveOccurred())

		values, err := svcExt.ToChartValuesMap()
		Expect(err).ShouldNot(HaveOccurred())

		return values
	}

	BeforeEach(func() {
		k8sExt = config.XMiniEnvK8s{
			Context:   "context",
			Namespace: "namespace",
		}
		mockGit = gitmocks.NewMockClient(GinkgoT())
		subject = helm.NewChartBuilder(fakeNgrokToken)
	})

	// The dispatch deployService relies on. It lives here rather than in
	// deployService because that method runs straight on into a cluster call,
	// which leaves the branch untestable anywhere else.
	Describe("Chart", func() {
		It("builds a job chart for a job deployment type", func() {
			chart, err := subject.Chart("hello", config.K8sJobDeploymentType)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(fileNames(chart)).To(ContainElement("templates/job.yaml"))
			Expect(fileNames(chart)).
				ToNot(ContainElement("templates/deployment.yaml"))
		})

		It("builds a service chart for a service deployment type", func() {
			chart, err := subject.Chart("hello", config.K8sServiceDeploymentType)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(fileNames(chart)).
				To(ContainElement("templates/deployment.yaml"))
			Expect(fileNames(chart)).ToNot(ContainElement("templates/job.yaml"))
		})

		// K8sDeploymentType is a string alias, so an unset extension arrives
		// here as "" rather than as the service default.
		It("falls back to a service chart for an unset deployment type", func() {
			chart, err := subject.Chart("hello", "")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(fileNames(chart)).
				To(ContainElement("templates/deployment.yaml"))
		})
	})

	Describe("ServiceChart", func() {
		It("assembles every chart file in memory", func() {
			chart, err := subject.ServiceChart("hello")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Name()).To(Equal("hello"))
			Expect(chart.Metadata.Version).To(Equal("0.1.0"))

			// ngrok lives in its own release for the namespace.
			Expect(fileNames(chart)).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/deployment.yaml",
				"templates/service.yaml",
				"templates/serviceaccount.yaml",
			))
		})

		It("produces a chart helm can validate", func() {
			chart, err := subject.ServiceChart("hello")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Validate()).To(Succeed())
		})

		Context("rendered against resolved service values", func() {
			var (
				svc      config.ComposeService
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
				chart, err := subject.ServiceChart("hello")
				Expect(err).ShouldNot(HaveOccurred())

				rendered = render(chart, resolvedValues(svc))
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
						config.K8sServiceExtension: map[string]any{
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
						config.K8sServiceExtension: map[string]any{
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

	Describe("JobChart", func() {
		It("assembles only the files a job needs", func() {
			chart, err := subject.JobChart("hello")

			Expect(err).ShouldNot(HaveOccurred())

			// No service, and no ngrok configmap or secret: a job serves no
			// traffic, so there is nothing to expose and no token to mount.
			Expect(fileNames(chart)).To(ConsistOf(
				"Chart.yaml",
				"values.yaml",
				"templates/_helpers.tpl",
				"templates/serviceaccount.yaml",
				"templates/job.yaml",
			))
		})

		It("produces a chart that validates", func() {
			chart, err := subject.JobChart("hello")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(chart.Validate()).To(Succeed())
		})

		It("names the chart after the service so destroy can find it", func() {
			first, err := subject.JobChart("hello")
			Expect(err).ShouldNot(HaveOccurred())

			second, err := subject.JobChart("hello")
			Expect(err).ShouldNot(HaveOccurred())

			Expect(first.Name()).To(Equal("hello"))
			Expect(second.Name()).To(Equal(first.Name()))
		})

		// A Job's spec.template and spec.selector are immutable, so an upgrade
		// cannot patch one in place.
		It("renders a new job name on every render", func() {
			chart, err := subject.JobChart("hello")
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

			chart, err := subject.JobChart(longName)
			Expect(err).ShouldNot(HaveOccurred())

			name := jobName(render(chart, map[string]any{}))

			Expect(len(name)).To(BeNumerically("<=", 63))
			Expect(name).To(MatchRegexp(`-[a-z0-9]{8}$`))
		})

		Context("rendered against resolved job values", func() {
			var (
				svc      config.ComposeService
				chart    *helmchart.Chart
				rendered map[string]string
			)

			BeforeEach(func() {
				svc = config.ComposeService{
					Name:    "migrate",
					Image:   "reg/migrate:v1",
					Command: types.ShellCommand{"/bin/sh", "-c", "echo ready"},
					Extensions: types.Extensions{
						config.K8sServiceExtension: map[string]any{
							"deploymentType": "job",
						},
					},
				}
			})

			JustBeforeEach(func() {
				var err error

				chart, err = subject.JobChart("migrate")
				Expect(err).ShouldNot(HaveOccurred())

				rendered = render(chart, resolvedValues(svc))
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

	Describe("NgrokValues", func() {
		It("resolves no values with nothing to publish", func() {
			Expect(subject.NgrokValues(nil)).To(BeNil())
			Expect(subject.NgrokValues([]config.NgrokEndpointConfig{})).To(BeNil())
		})
	})

	Describe("NgrokChart", func() {
		It("assembles only the files the ngrok release needs", func() {
			chart, err := subject.NgrokChart()
			Expect(err).ShouldNot(HaveOccurred())

			// No service.yaml: the agent dials out.
			Expect(fileNames(chart)).To(ConsistOf(
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

		Context("rendered against the endpoints to publish", func() {
			var (
				toPublish []config.NgrokEndpointConfig
				rendered  map[string]string
			)

			renderNgrok := func(
				builder *helm.ChartBuilder,
				endpoints []config.NgrokEndpointConfig,
			) map[string]string {
				GinkgoHelper()

				chart, err := builder.NgrokChart()
				Expect(err).ShouldNot(HaveOccurred())

				return render(chart, builder.NgrokValues(endpoints))
			}

			BeforeEach(func() {
				toPublish = []config.NgrokEndpointConfig{
					publishable("alpha", 3000),
					publishable("beta", 3001),
				}
			})

			JustBeforeEach(func() {
				rendered = renderNgrok(subject, toPublish)
			})

			It("publishes every endpoint from one config", func() {
				cfg := ngrokAgentConfigFrom(rendered)

				Expect(cfg.Version).To(Equal(3))
				Expect(cfg.Endpoints).To(HaveLen(2))

				// The endpoint name is namespaced so two environments cannot
				// collide on the account, while the upstream stays the bare
				// service name that k8s DNS resolves inside the namespace.
				Expect(cfg.Endpoints[0].Name).To(Equal("namespace-alpha"))
				Expect(cfg.Endpoints[0].Upstream.URL).To(Equal("alpha:3000"))
				Expect(cfg.Endpoints[1].Name).To(Equal("namespace-beta"))
				Expect(cfg.Endpoints[1].Upstream.URL).To(Equal("beta:3001"))
			})

			// An upstream is a hostname *and* a port, and the agent dials
			// outbound. compose.yml keeps hello and hello2 both on 8080, and
			// this spec is what says that is deliberate.
			Context("with two endpoints on the same port", func() {
				BeforeEach(func() {
					toPublish = []config.NgrokEndpointConfig{
						publishable("alpha", 8080),
						publishable("beta", 8080),
					}
				})

				It("gives each its own endpoint and upstream", func() {
					cfg := ngrokAgentConfigFrom(rendered)

					Expect(cfg.Endpoints).To(HaveLen(2))
					Expect(cfg.Endpoints[0].Name).To(Equal("namespace-alpha"))
					Expect(cfg.Endpoints[0].Upstream.URL).To(Equal("alpha:8080"))
					Expect(cfg.Endpoints[1].Name).To(Equal("namespace-beta"))
					Expect(cfg.Endpoints[1].Upstream.URL).To(Equal("beta:8080"))
				})
			})

			It("omits the url when no domain is reserved", func() {
				for _, ep := range ngrokAgentConfigFrom(rendered).Endpoints {
					Expect(ep.URL).To(BeEmpty())
				}
			})

			Context("with a reserved domain", func() {
				BeforeEach(func() {
					alpha := publishable("alpha", 3000)
					alpha.URL = "https://example.ngrok.app"
					toPublish = []config.NgrokEndpointConfig{alpha}
				})

				It("pins the endpoint to it", func() {
					cfg := ngrokAgentConfigFrom(rendered)

					Expect(cfg.Endpoints).To(HaveLen(1))
					Expect(cfg.Endpoints[0].URL).
						To(Equal("https://example.ngrok.app"))
				})
			})

			Context("with a multi-line traffic policy", func() {
				BeforeEach(func() {
					alpha := publishable("alpha", 3000)
					alpha.TrafficPolicy = "on_http_request:\n" +
						"  - actions:\n" +
						"      - type: deny\n"
					toPublish = []config.NgrokEndpointConfig{alpha}
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

			It("runs one agent for every endpoint at once", func() {
				deployment := templateNamed(rendered, "deployment.yaml")

				Expect(deployment).To(ContainSubstring("- --all"))
				Expect(deployment).To(ContainSubstring(
					"image: \"" + config.NgrokImageRepo +
						":" + config.NgrokImageTag + "\"",
				))
				Expect(deployment).To(ContainSubstring(
					"mountPath: " + config.NgrokConfigVolMountPath,
				))
				Expect(deployment).To(ContainSubstring(
					"name: " + config.NgrokSecretName,
				))
			})

			// RollingUpdate would overlap two agents claiming the same URLs.
			It("replaces the agent rather than rolling it", func() {
				Expect(templateNamed(rendered, "deployment.yaml")).
					To(ContainSubstring("type: Recreate"))
			})

			// The agent reads ngrok.yml only at startup, so without this the
			// ConfigMap updates and the agent serves the previous set.
			It("changes the config checksum when an endpoint is added", func() {
				before := podAnnotation(rendered, "checksum/config")

				added := append(toPublish, publishable("gamma", 3002))

				Expect(podAnnotation(
					renderNgrok(subject, added), "checksum/config",
				)).ToNot(Equal(before))
			})

			It("changes the config checksum when a port changes", func() {
				before := podAnnotation(rendered, "checksum/config")

				toPublish[0].Port = 3009

				Expect(podAnnotation(
					renderNgrok(subject, toPublish), "checksum/config",
				)).ToNot(Equal(before))
			})

			// A deploy that changed nothing must not replace the pod, or every
			// unreserved URL is reassigned.
			It("keeps the config checksum stable for the same endpoints", func() {
				before := podAnnotation(rendered, "checksum/config")

				for range 5 {
					Expect(podAnnotation(
						renderNgrok(subject, toPublish), "checksum/config",
					)).To(Equal(before))
				}
			})

			// Nothing else in the pod template changes on rotation.
			It("changes the secret checksum when the token rotates", func() {
				before := podAnnotation(rendered, "checksum/secret")

				rotated := helm.NewChartBuilder("a-different-token")

				Expect(podAnnotation(
					renderNgrok(rotated, toPublish), "checksum/secret",
				)).ToNot(Equal(before))
			})
		})
	})
})
