package deployer_test

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

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

// The per-service failure the injected step returns, asserted by identity for
// the same reason.
var errStep = errors.New("step blew up")

// A fixed fake. The real NGROK_AUTHTOKEN is only ever read at the
// composition root, so no test binary can pick up a live credential and
// serialize it into a rendered chart or a failure report.
const fakeNgrokToken = "fake-token-for-tests"

// dependent builds a bare service that depends_on each named service. Only the
// name and depends_on reach the dependency walk. Required matters: compose-go
// silently drops a dependency that is both unknown and not required, so an
// optional one would not exercise the unknown-dependency branch.
func dependent(name string, deps ...string) config.ComposeService {
	dependsOn := types.DependsOnConfig{}

	for _, dep := range deps {
		dependsOn[dep] = types.ServiceDependency{
			Condition: types.ServiceConditionStarted,
			Required:  true,
		}
	}

	return config.ComposeService{Name: name, DependsOn: dependsOn}
}

// recorder logs the order the walk visited services in. The walk runs services
// concurrently and the suite runs with -race, so the log needs a lock: an
// unguarded append here would be a data race, not an occasional flake.
type recorder struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	ok     []string
	fail   map[string]error
}

func newRecorder(cancel context.CancelFunc) *recorder {
	return &recorder{
		mu:     sync.Mutex{},
		cancel: cancel,
		ok:     []string{},
		fail:   map[string]error{},
	}
}

func (r *recorder) visit(ctx context.Context, svc config.ComposeService) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err, ok := r.fail[svc.Name]; ok {
		r.cancel()
		return err
	}

	r.ok = append(r.ok, svc.Name)

	return nil
}

func (r *recorder) succeeded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.ok)
}

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

	Describe("deployInDependencyOrder", func() {
		var (
			project *config.ComposeProject
			rec     *recorder
		)

		BeforeEach(func() {
			_, cancel := context.WithCancel(context.Background())
			project = &config.ComposeProject{Name: "test-project"}
			rec = newRecorder(cancel)
		})

		It("deploys a chain dependencies first", func() {
			project.Services = types.Services{
				"api":   dependent("api", "cache"),
				"cache": dependent("cache", "db"),
				"db":    dependent("db"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(rec.visit)).
				To(Succeed())

			// The only fully determined order in the suite, so assert it exactly.
			Expect(rec.succeeded()).To(Equal([]string{"db", "cache", "api"}))
		})

		It("deploys a diamond by position, not by sequence", func() {
			project.Services = types.Services{
				"db":     dependent("db"),
				"api":    dependent("api", "db"),
				"worker": dependent("worker", "db"),
				"ui":     dependent("ui", "api", "worker"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(rec.visit)).
				To(Succeed())

			// api and worker are free to run concurrently, so only their position
			// relative to db and ui is guaranteed.
			order := rec.succeeded()
			Expect(order).To(HaveLen(4))
			Expect(order[0]).To(Equal("db"))
			Expect(order[1:3]).To(ConsistOf("api", "worker"))
			Expect(order[3]).To(Equal("ui"))
		})

		It("deploys every unrelated service exactly once", func() {
			project.Services = types.Services{
				"one":   dependent("one"),
				"two":   dependent("two"),
				"three": dependent("three"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(rec.visit)).
				To(Succeed())

			Expect(rec.succeeded()).To(ConsistOf("one", "two", "three"))
		})

		// Both fakes have to be in flight at once for either to return, so a
		// serialized walk cannot satisfy this spec. The ctx guard is what makes
		// that show up as a timeout failure rather than a hung suite.
		It("deploys unrelated services concurrently", func(specCtx SpecContext) {
			project.Services = types.Services{
				"one": dependent("one"),
				"two": dependent("two"),
			}
			subject.SetProject(project)

			var (
				mu       sync.Mutex
				inFlight int
				bothIn   = make(chan struct{})
			)

			deploy := func(ctx context.Context, svc config.ComposeService) error {
				mu.Lock()
				inFlight++
				if inFlight == 2 {
					close(bothIn)
				}
				mu.Unlock()

				select {
				case <-bothIn:
					return nil
				case <-specCtx.Done():
					return ctx.Err()
				}
			}

			Expect(subject.DeployInDependencyOrder(deploy)).To(Succeed())
		}, SpecTimeout(10*time.Second))

		It("propagates a failure from the last service in a chain", func() {
			project.Services = types.Services{
				"api":   dependent("api", "cache"),
				"cache": dependent("cache", "db"),
				"db":    dependent("db"),
			}
			subject.SetProject(project)

			rec.fail["api"] = errStep

			err := subject.DeployInDependencyOrder(rec.visit)

			Expect(err).To(MatchError(errStep))
			Expect(rec.succeeded()).To(Equal([]string{"db", "cache"}))
		})

		It("propagates a failure from a dependency", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db"),
			}
			subject.SetProject(project)

			rec.fail["db"] = errStep

			err := subject.DeployInDependencyOrder(rec.visit)

			// Only the propagation is asserted. Whether api is dispatched before
			// the group's context cancellation lands is a race inside compose-go's
			// traversal, so asserting api was skipped would be asserting a
			// guarantee the walk does not make.
			Expect(err).To(MatchError(errStep))
			Expect(rec.succeeded()).NotTo(ContainElement("db"))
		})

		It("rejects a dependency cycle before deploying anything", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db", "api"),
			}
			subject.SetProject(project)

			err := subject.DeployInDependencyOrder(rec.visit)

			Expect(err).To(MatchError(deployer.KindComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})

		It("rejects a dependency naming an undefined service", func() {
			project.Services = types.Services{
				"api": dependent("api", "missing"),
			}
			subject.SetProject(project)

			err := subject.DeployInDependencyOrder(rec.visit)

			Expect(err).To(MatchError(deployer.KindComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})

		It("deploys nothing for an empty project", func() {
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(rec.visit)).
				To(Succeed())

			Expect(rec.succeeded()).To(BeEmpty())
		})
	})

	Describe("destroyInReverseDependencyOrder", func() {
		var (
			project *config.ComposeProject
			rec     *recorder
		)

		BeforeEach(func() {
			_, cancel := context.WithCancel(context.Background())
			rec = newRecorder(cancel)
			project = &config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"api":   dependent("api", "cache"),
					"cache": dependent("cache", "db"),
					"db":    dependent("db"),
				},
			}
		})

		It("destroys a chain dependents first", func() {
			subject.SetProject(project)
			Expect(subject.DestroyInReverseDependencyOrder(rec.visit)).
				To(Succeed())

			// The exact inverse of the deploy walk: nothing is uninstalled while
			// something still depends on it.
			Expect(rec.succeeded()).To(Equal([]string{"api", "cache", "db"}))
		})

		It("propagates a failure from service that failed to be destroyed", func() {
			subject.SetProject(project)
			rec.fail["api"] = errStep
			err := subject.DestroyInReverseDependencyOrder(rec.visit)
			Expect(err).To(MatchError(errStep))
		})

		It("rejects a dependency cycle before destroying anything", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db", "api"),
			}
			subject.SetProject(project)

			err := subject.DestroyInReverseDependencyOrder(rec.visit)

			Expect(err).To(MatchError(deployer.KindComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})
	})

	// The step the walk dispatches to. Only the branches that return before a
	// helm client is constructed are reachable without a cluster.
	Describe("deployService", func() {
		It("omits a service flagged skip", func() {
			ctx := context.Background()
			svc := config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{"skip": true},
			}

			Expect(subject.InitProject(&config.ComposeProject{
				Name:     "test-project",
				Services: types.Services{"hello": svc},
			})).To(Succeed())

			// mockImage and mockGit carry no EXPECT(), so reaching any client
			// fails this spec on cleanup — as would reaching helm, since Init was
			// never called and the action config is still nil.
			Expect(subject.DeployService(ctx, svc)).To(Succeed())
		})

		// The step reads the map initProject builds rather than resolving the
		// extension itself, so a service the map does not know about is the one
		// failure it still owns — and it means Init was skipped, or the project
		// changed underneath it.
		It("errors for a service that was never resolved by Init", func() {
			ctx := context.Background()

			Expect(subject.InitProject(&config.ComposeProject{
				Name: "test-project",
			})).To(Succeed())

			err := subject.DeployService(ctx, config.ComposeService{Name: "hello"})

			Expect(err).To(MatchError(deployer.KindHelmMissingService))
		})
	})
})
