package resolver_test

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/resolver"
)

func duration(d time.Duration) *types.Duration {
	converted := types.Duration(d)
	return &converted
}

func makeK8sExt() config.XMiniEnvK8s {
	return config.XMiniEnvK8s{
		Context:   "context",
		Namespace: "namespace",
	}
}

// The git failure the fake returns; asserted by identity so the spec proves
// resolveServiceImage preserved the cause.
var errNotARepo = errors.New("not a repo")

var _ = Describe("K8sService", func() {
	var (
		k8sExt       config.XMiniEnvK8s
		svc          compose.Service
		raw          compose.RawService
		dir          string
		mockGit      *gitmocks.MockClient
		ngrokEnabled bool
	)

	// Options are assembled lazily rather than in BeforeEach because specs
	// mutate k8sExt, svc and ngrokEnabled in their own bodies before
	// resolving.
	newSvcExt := func() (*resolver.K8sService, error) {
		return resolver.NewK8sService(
			context.Background(),
			resolver.K8sServiceOptions{
				K8sExt:       k8sExt,
				Service:      svc,
				Raw:          raw,
				Dir:          dir,
				GitClient:    mockGit,
				NgrokEnabled: ngrokEnabled,
			})
	}

	BeforeEach(func() {
		k8sExt = makeK8sExt()
		mockGit = gitmocks.NewMockClient(GinkgoT())
		ngrokEnabled = false
		raw = compose.RawService{}
		dir = ""
		svc = compose.Service{
			Name:  "test-service",
			Image: "reg/test-service:v1",
		}
	})

	Describe("required configuration", func() {
		It("returns a config error if k8s is not configured", func() {
			k8sExt = config.XMiniEnvK8s{}
			svc = compose.Service{Name: "test-service"}

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrK8sNotConfigured))
		})

		It("errors if the service has neither an image nor an extension", func() {
			svc = compose.Service{Name: "test-service"}

			_, err := newSvcExt()

			// resolveServiceImage accumulates both failures with errors.Join;
			// errors.Is walks the join, so this asserts both were collected.
			Expect(err).To(MatchError(resolver.ErrImageRepositoryMissing))
			Expect(err).To(MatchError(resolver.ErrImageTagMissing))
		})
	})

	It("keeps the compose service it was resolved from", func() {
		result, err := newSvcExt()

		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.Compose).To(Equal(svc))
	})

	Describe("image resolution", func() {
		It("falls back to the compose image repo and tag", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Image.Repository).To(Equal("reg/test-service"))
			Expect(result.Image.Tag).To(Equal("v1"))
		})

		It("defaults platforms to linux/amd64", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Image.Platforms).To(Equal([]string{"linux/amd64"}))
		})

		It("prefers the extension image over the compose image", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"image": map[string]any{
						"repository": "reg/override",
						"tag":        "v2",
						"platforms":  []any{"linux/arm64"},
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Image.Repository).To(Equal("reg/override"))
			Expect(result.Image.Tag).To(Equal("v2"))
			Expect(result.Image.Platforms).To(Equal([]string{"linux/arm64"}))
		})

		Context("when the tag contains +git", func() {
			BeforeEach(func() {
				svc.Image = "reg/test-service:+git"
			})

			// git.Client is responsible for handing back a bare sha; the
			// trailing-newline contract is pinned in internal/git's own specs.
			It("substitutes the short sha into the tag", func() {
				mockGit.EXPECT().ShortSha(mock.Anything).Return("abc1234", nil).Once()

				result, err := newSvcExt()

				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Image.Tag).To(Equal("abc1234"))
			})

			It("returns a config error when git fails", func() {
				mockGit.
					EXPECT().
					ShortSha(mock.Anything).
					Return("", errNotARepo).
					Once()

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrGitShortSha))
				Expect(err).To(MatchError(errNotARepo))
			})
		})
	})

	Describe("port resolution", func() {
		It("derives service ports from the compose port mappings", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Ports).To(Equal([]config.ChartServicePort{
				{
					ContainerPortName: "p8080",
					ContainerPort:     8080,
					Protocol:          "TCP",
				},
			}))
		})

		It("upper-cases an explicit protocol", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000", Protocol: "udp"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Ports[0].Protocol).To(Equal("UDP"))
		})

		It("accepts ports above the signed 16-bit range", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 40000, Published: "65535"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Ports[0].ContainerPort).To(BeEquivalentTo(40000))
		})

		It("rejects a container port beyond the 16-bit range", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 70000, Published: "3000"},
			}

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrInvalidPort))
		})

		It("uses container port for a mapping with no published port", func() {
			svc.Ports = []types.ServicePortConfig{{Target: 8080}}

			result, err := newSvcExt()

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Service.Ports).To(HaveLen(1))
			Expect(result.Service.Ports[0].ContainerPort).To(Equal(uint16(8080)))
		})

		It("does not duplicate a port already declared in the extension", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"service": map[string]any{
						"ports": []any{
							map[string]any{
								"containerPortName": "http",
								"containerPort":     8080,
								"protocol":          "TCP",
							},
						},
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Ports).To(HaveLen(1))
			Expect(result.Service.Ports[0].ContainerPortName).To(Equal("http"))
		})

		It("skips service and service account creation with no ports", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Create).To(HaveValue(BeFalse()))
			Expect(result.ServiceAccount.Create).To(HaveValue(BeFalse()))
		})

		// The whole reason Create is a *bool: unset has to stay distinguishable
		// from an explicit false, so the chart's own values.yaml default is what
		// decides when minienv has no opinion.
		It("leaves service creation unset when there are ports", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Service.Create).To(BeNil())
			Expect(result.ServiceAccount.Create).To(BeNil())
		})
	})

	Describe("environment resolution", func() {
		It("merges compose environment into the chart values", func() {
			svc.Environment = types.MappingWithEquals{
				"FOO": new("bar"),
			}
			raw.Environment = []any{"FOO=bar"}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{
					"name":  "FOO",
					"value": "bar",
				},
			}))
		})

		It("lets the extension env win over the compose env", func() {
			svc.Environment = types.MappingWithEquals{
				"FOO": new("from-compose"),
				"BAZ": new("qux"),
			}
			raw.Environment = []any{"FOO=from-compose", "BAZ=qux"}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"env": []map[string]any{
						{
							"name":  "FOO",
							"value": "from-extension",
						},
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{
					"name":  "FOO",
					"value": "from-extension",
				},
				{
					"name":  "BAZ",
					"value": "qux",
				},
			}))
		})

		It("orders compose variables by name", func() {
			svc.Environment = types.MappingWithEquals{
				"CHARLIE": new("3"),
				"ALPHA":   new("1"),
				"BRAVO":   new("2"),
			}
			raw.Environment = map[string]any{
				"CHARLIE": "3",
				"ALPHA":   "1",
				"BRAVO":   "2",
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{"name": "ALPHA", "value": "1"},
				{"name": "BRAVO", "value": "2"},
				{"name": "CHARLIE", "value": "3"},
			}))
		})

		It("passes a valueFrom entry through untouched", func() {
			svc.Environment = types.MappingWithEquals{
				"DB_PASSWORD": new("from-compose"),
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"env": []map[string]any{
						{
							"name": "DB_PASSWORD",
							"valueFrom": map[string]any{
								"configMapKeyRef": map[string]any{
									"name": "app-config",
									"key":  "password",
								},
							},
						},
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{
					"name": "DB_PASSWORD",
					"valueFrom": map[string]any{
						"configMapKeyRef": map[string]any{
							"name": "app-config",
							"key":  "password",
						},
					},
				},
			}))
		})

		It("decodes envFrom sources", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"envFrom": []map[string]any{
						{
							"configMapRef": map[string]any{"name": "app-config"},
						},
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.EnvFrom).To(Equal([]map[string]any{
				{
					"configMapRef": map[string]any{"name": "app-config"},
				},
			}))
		})

		It("omits unresolved pass-through variables instead of panicking", func() {
			// `environment: [FOO]` that compose could not resolve from the host
			// arrives as a nil *string.
			svc.Environment = types.MappingWithEquals{
				"FOO":      nil,
				"RESOLVED": new("value"),
			}
			raw.Environment = []any{"FOO", "RESOLVED=value"}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{
					"name":  "RESOLVED",
					"value": "value",
				},
			}))
		})

		It("writes no secret values when every value is literal", func() {
			svc.Environment = types.MappingWithEquals{
				"FOO": new("bar"),
			}
			raw.Environment = []any{"FOO=bar"}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.SecretEnv).To(BeEmpty())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("secretEnv"))
		})

		Context("with a list-form environment taking a value from the host", func() {
			BeforeEach(func() {
				svc.Environment = types.MappingWithEquals{
					"DB_PASSWORD": new("hunter2"),
					"LOG_LEVEL":   new("debug"),
					"MISSING":     nil,
				}
				raw.Environment = []any{
					"DB_PASSWORD=${DB_PASSWORD}",
					"LOG_LEVEL=debug",
					"MISSING",
				}
			})

			It("moves it out of env and into the secret values", func() {
				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Env).To(Equal([]map[string]any{
					{
						"name":  "LOG_LEVEL",
						"value": "debug",
					},
				}))
				Expect(result.SecretEnv).To(Equal(map[string]string{
					"DB_PASSWORD": "hunter2",
				}))

				values, err := result.ChartValues()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(values["secretEnv"]).To(Equal(map[string]string{
					"DB_PASSWORD": "hunter2",
				}))
			})

			It("moves an extension value of that name into the secret", func() {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"env": []map[string]any{
							{
								"name":  "DB_PASSWORD",
								"value": "from-extension",
							},
						},
					},
				}
				raw.Extensions = map[string]any{
					config.K8sServiceExtension: map[string]any{
						"env": []any{
							map[string]any{
								"name":  "DB_PASSWORD",
								"value": "${OTHER_SECRET}",
							},
						},
					},
				}

				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Env).To(Equal([]map[string]any{
					{
						"name":  "LOG_LEVEL",
						"value": "debug",
					},
				}))
				Expect(result.SecretEnv).To(Equal(map[string]string{
					"DB_PASSWORD": "from-extension",
				}))
			})

			It("keeps a literal extension value of that name inline", func() {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"env": []map[string]any{
							{
								"name":  "DB_PASSWORD",
								"value": "not-a-secret",
							},
						},
					},
				}
				raw.Extensions = map[string]any{
					config.K8sServiceExtension: map[string]any{
						"env": []any{
							map[string]any{
								"name":  "DB_PASSWORD",
								"value": "not-a-secret",
							},
						},
					},
				}

				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Env).To(Equal([]map[string]any{
					{
						"name":  "DB_PASSWORD",
						"value": "not-a-secret",
					},
					{
						"name":  "LOG_LEVEL",
						"value": "debug",
					},
				}))
				Expect(result.SecretEnv).To(BeEmpty())
			})

			It("passes an extension valueFrom of that name through", func() {
				ref := map[string]any{
					"secretKeyRef": map[string]any{
						"name": "other",
						"key":  "token",
					},
				}
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"env": []map[string]any{
							{
								"name":      "DB_PASSWORD",
								"valueFrom": ref,
							},
						},
					},
				}
				raw.Extensions = map[string]any{
					config.K8sServiceExtension: map[string]any{
						"env": []any{
							map[string]any{
								"name":      "DB_PASSWORD",
								"valueFrom": ref,
							},
						},
					},
				}

				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Env).To(Equal([]map[string]any{
					{
						"name":      "DB_PASSWORD",
						"valueFrom": ref,
					},
					{
						"name":  "LOG_LEVEL",
						"value": "debug",
					},
				}))
				Expect(result.SecretEnv).To(BeEmpty())

				values, err := result.ChartValues()
				Expect(err).ShouldNot(HaveOccurred())
				Expect(values).ToNot(HaveKey("secretEnv"))
			})
		})

		It("moves a map-form value taken from the host into the secret", func() {
			svc.Environment = types.MappingWithEquals{
				"TOKEN":     new("from-host"),
				"PORT":      new("8080"),
				"INHERITED": new("yes"),
			}
			raw.Environment = map[string]any{
				"TOKEN":     "${TOKEN}",
				"PORT":      8080,
				"INHERITED": nil,
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{"name": "PORT", "value": "8080"},
			}))
			Expect(result.SecretEnv).To(Equal(map[string]string{
				"INHERITED": "yes",
				"TOKEN":     "from-host",
			}))
		})

		It("moves a value declared nowhere in the file into the secret", func() {
			svc.Environment = types.MappingWithEquals{
				"FROM_FILE": new("file-value"),
				"LITERAL":   new("plain"),
			}
			raw.Environment = []any{"LITERAL=plain"}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal([]map[string]any{
				{"name": "LITERAL", "value": "plain"},
			}))
			Expect(result.SecretEnv).To(Equal(map[string]string{
				"FROM_FILE": "file-value",
			}))
		})

		It("moves an extension-only value taken from the host into the secret", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"env": []map[string]any{
						{
							"name":  "API_KEY",
							"value": "from-host",
						},
					},
				},
			}
			raw.Extensions = map[string]any{
				config.K8sServiceExtension: map[string]any{
					"env": []any{
						map[string]any{
							"name":  "API_KEY",
							"value": "${API_KEY}",
						},
					},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(BeEmpty())
			Expect(result.SecretEnv).To(Equal(map[string]string{
				"API_KEY": "from-host",
			}))

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["secretEnv"]).To(Equal(map[string]string{
				"API_KEY": "from-host",
			}))
		})

		It("writes no env values without compose or extension environment", func() {
			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("env"))
		})
	})

	Describe("healthcheck resolution", func() {
		BeforeEach(func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}
		})

		It("turns a CMD-SHELL check into an exec probe", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test:     types.HealthCheckTest{"CMD-SHELL", "pg_isready -U postgres"},
				Interval: duration(10 * time.Second),
				Timeout:  duration(5 * time.Second),
				Retries:  func() *uint64 { r := uint64(3); return &r }(),
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.LivenessProbe).To(HaveKeyWithValue("exec", map[string]any{
				"command": []string{"/bin/sh", "-c", "pg_isready -U postgres"},
			}))
			Expect(result.LivenessProbe).To(HaveKeyWithValue("periodSeconds", 10))
			Expect(result.LivenessProbe).To(HaveKeyWithValue("timeoutSeconds", 5))
			Expect(result.LivenessProbe).To(HaveKeyWithValue("failureThreshold", 3))
		})

		It("turns a localhost curl check into an httpGet probe", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test: types.HealthCheckTest{
					"CMD", "curl", "-f", "http://localhost:8080/health",
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ReadinessProbe).To(HaveKeyWithValue("httpGet", map[string]any{
				"path": "/health",
				"port": "p8080",
			}))
		})

		It("falls back to an exec probe for a non-url CMD check", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test: types.HealthCheckTest{"CMD", "pg_isready", "-U", "postgres"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.StartupProbe).To(HaveKeyWithValue("exec", map[string]any{
				"command": []string{"/bin/sh", "-c", "pg_isready -U postgres"},
			}))
		})

		It("creates no probes for a NONE check", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test: types.HealthCheckTest{"NONE"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.StartupProbe).To(BeNil())
			Expect(result.LivenessProbe).To(BeNil())
			Expect(result.ReadinessProbe).To(BeNil())
		})

		It("creates no probes for a disabled check", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test:    types.HealthCheckTest{"CMD", "true"},
				Disable: true,
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.LivenessProbe).To(BeNil())
		})

		It("tolerates a healthcheck with no test command", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Interval: duration(10 * time.Second),
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.StartupProbe).To(BeNil())
			Expect(result.LivenessProbe).To(BeNil())
			Expect(result.ReadinessProbe).To(BeNil())
		})

		It("tolerates a CMD-SHELL check with no command", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test: types.HealthCheckTest{"CMD-SHELL"},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.StartupProbe).To(BeNil())
			Expect(result.LivenessProbe).To(BeNil())
			Expect(result.ReadinessProbe).To(BeNil())
		})

		It("does not overwrite probes set explicitly in the extension", func() {
			svc.HealthCheck = &types.HealthCheckConfig{
				Test: types.HealthCheckTest{"CMD", "true"},
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"livenessProbe": map[string]any{"custom": true},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.LivenessProbe).To(Equal(map[string]any{"custom": true}))
		})
	})

	Describe("deployment timeout precedence", func() {
		It("uses the default when nothing is configured", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentTimeout).
				To(Equal(config.HelmDefaultDeploymentTimeout))
		})

		It("prefers the top-level timeout over the default", func() {
			k8sExt.DeploymentTimeout = "5m"

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentTimeout).To(Equal("5m"))
		})

		It("prefers the service timeout over the top-level timeout", func() {
			k8sExt.DeploymentTimeout = "5m"
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"deploymentTimeout": "90s",
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentTimeout).To(Equal("90s"))
		})
	})

	Describe("ngrok", func() {
		BeforeEach(func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}
		})

		It("inherits the top-level traffic policy", func() {
			ngrokEnabled = true
			k8sExt.Ngrok = &config.NgrokTopLevel{
				TrafficPolicy: "top-level-policy",
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"ngrok": map[string]any{"port": 8080},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Ngrok.TrafficPolicy).To(Equal("top-level-policy"))
		})

		It("lets the service traffic policy win", func() {
			ngrokEnabled = true
			k8sExt.Ngrok = &config.NgrokTopLevel{
				TrafficPolicy: "top-level-policy",
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"ngrok": map[string]any{
						"port":          8080,
						"trafficPolicy": "service-policy",
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Ngrok.TrafficPolicy).To(Equal("service-policy"))
		})

		It("wires nothing when no auth token is set", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"ngrok": map[string]any{"port": 8080},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Volumes).To(BeEmpty())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("ngrok"))
		})

		It("clears all ngrok config when no auth token is available", func() {
			k8sExt.Ngrok = &config.NgrokTopLevel{
				TrafficPolicy: "top-level-policy",
			}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"ngrok": map[string]any{
						"port":          8080,
						"url":           "https://example.ngrok.app",
						"trafficPolicy": "service-policy",
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Ngrok).To(Equal(config.NgrokServiceLevel{}))
		})

		Context("with an auth token set", func() {
			BeforeEach(func() {
				ngrokEnabled = true
			})

			// The endpoint's upstream targets a container port, so a port
			// matching none of them publishes a URL that cannot route.
			It("errors when the ngrok port matches no container port", func() {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"ngrok": map[string]any{"port": 9999},
					},
				}

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrNgrokPortMismatch))
			})

			It("accepts a port matching a mapped container port", func() {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"ngrok": map[string]any{"port": 8080},
					},
				}

				result, err := newSvcExt()

				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Ngrok.Port).To(Equal(uint16(8080)))
			})

			It("rejects the host side of a port mapping", func() {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"ngrok": map[string]any{"port": 3000},
					},
				}

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrNgrokPortMismatch))
			})
		})
	})

	Describe("deployment type", func() {
		It("defaults to a service deployment", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentType).
				To(Equal(config.K8sServiceDeploymentType))
		})

		It("reads an explicit job deployment from the extension", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"deploymentType": "job",
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentType).To(Equal(config.K8sJobDeploymentType))
		})

		// K8sDeploymentType is a string alias, so neither the compiler nor
		// mapstructure rejects a typo. Without this branch the deployer has to
		// guess at what an unrecognized value meant.
		It("returns a config error for an unrecognized deployment type", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					// Deliberately misspelled: the point is that an
					// unrecognized value is rejected rather than guessed at.
					//nolint:misspell // intentional typo under test
					"deploymentType": "sevice",
				},
			}

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrInvalidDeploymentType))
		})
	})

	Describe("container command", func() {
		It("derives the command from the compose service", func() {
			svc.Command = types.ShellCommand{"/bin/sh", "-c", "echo ready"}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Command).To(Equal([]string{"/bin/sh", "-c", "echo ready"}))
		})

		It("prefers a command set in the extension", func() {
			svc.Command = types.ShellCommand{"compose"}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"command": []any{"extension"},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Command).To(Equal([]string{"extension"}))
		})

		It("leaves the command unset when neither declares one", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Command).To(BeNil())
		})
	})

	Describe("manifests", func() {
		It("carries the declared paths through untouched", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []any{
						"k8s/configmap.yaml",
						"k8s/ingressroute.yaml",
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Manifests).To(Equal([]string{
				"k8s/configmap.yaml",
				"k8s/ingressroute.yaml",
			}))
		})

		It("leaves them empty when none are declared", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Manifests).To(BeEmpty())
		})

		// The declared path names the chart file it becomes, so the name has
		// to be settled before anything reads it.
		It("cleans each declared path", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []any{"./k8s/../k8s/configmap.yaml"},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Manifests).To(Equal([]string{"k8s/configmap.yaml"}))
		})

		It("resolves each path against the declaring directory", func() {
			dir = filepath.Join("proj", "api")
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []any{"k8s/configmap.yaml"},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ManifestPaths()).To(Equal([]string{
				filepath.Join("proj", "api", "k8s", "configmap.yaml"),
			}))
		})

		It("resolves no paths when none are declared", func() {
			dir = filepath.Join("proj", "api")

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ManifestPaths()).To(BeEmpty())
		})

		DescribeTable(
			"rejects a path that leaves the project",
			func(declared string) {
				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"manifests": []any{declared},
					},
				}

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrManifestPath))
			},
			Entry("an absolute path", "/etc/k8s/configmap.yaml"),
			Entry("a path climbing out", "../configmap.yaml"),
			Entry("a path climbing out after cleaning", "k8s/../../cm.yaml"),
		)

		It("rejects two files sharing a base name", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []any{
						"k8s/configmap.yaml",
						"other/configmap.yaml",
					},
				},
			}

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrManifestPath))
		})

		// These name chart files rather than chart values, so leaking one into
		// the values map would put an unknown key in front of every template.
		It("keeps them out of the chart values", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"manifests": []any{"k8s/configmap.yaml"},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("manifests"))
		})
	})

	Describe("configMapFrom", func() {
		It("resolves each path against the declaring directory", func() {
			dir = filepath.Join("proj", "db")
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"configMapFrom": []any{"init/01-schema.sql"},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ConfigMapFromPaths()).To(Equal([]string{
				filepath.Join("proj", "db", "init", "01-schema.sql"),
			}))
		})

		It("carries each declared path through cleaned", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"configMapFrom": []any{
						"./db/init/../init/01-schema.sql",
						"db/init/02-seed.sql",
					},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ConfigMapFrom).To(Equal([]string{
				"db/init/01-schema.sql",
				"db/init/02-seed.sql",
			}))
		})

		It("leaves them empty when none are declared", func() {
			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.ConfigMapFrom).To(BeEmpty())
		})

		DescribeTable(
			"rejects paths that cannot become ConfigMap keys",
			func(declared []string) {
				entries := make([]any, 0, len(declared))
				for _, d := range declared {
					entries = append(entries, d)
				}

				svc.Extensions = types.Extensions{
					config.K8sServiceExtension: map[string]any{
						"configMapFrom": entries,
					},
				}

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrConfigMapFromPath))
			},
			Entry("an absolute path", []string{"/etc/init.sql"}),
			Entry("a path climbing out", []string{"../init.sql"}),
			Entry("a path climbing out after cleaning",
				[]string{"db/../../init.sql"}),
			Entry("the project directory itself", []string{"."}),
			Entry("a file name with a space", []string{"db/init 01.sql"}),
			Entry("two files sharing a name",
				[]string{"db/init.sql", "seed/init.sql"}),
		)

		It("keeps them out of the chart values", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"configMapFrom": []any{"db/init/01-schema.sql"},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("configMapFrom"))
		})
	})

	Describe("chart values", func() {
		It("decodes service ports into the nested chart shape", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())

			svcValues, ok := values["service"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(svcValues["ports"]).To(Equal([]map[string]any{
				{
					"containerPortName": "p8080",
					"containerPort":     uint16(8080),
					"protocol":          "TCP",
				},
			}))
		})

		It("decodes image pull secrets into the nested chart shape", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"imagePullSecrets": []any{
						map[string]any{"name": "regcred"},
					},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["imagePullSecrets"]).To(Equal([]map[string]any{
				{"name": "regcred"},
			}))
		})

		It("carries replicas through under the key the chart reads", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"replicas": 3,
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).To(HaveKeyWithValue("replicas", uint8(3)))
		})

		It("carries the command through under the key the chart reads", func() {
			svc.Command = types.ShellCommand{"/bin/sh", "-c", "echo ready"}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).To(HaveKeyWithValue(
				"command",
				[]string{"/bin/sh", "-c", "echo ready"},
			))
		})

		// Ensures *bools are converted to bool in the map otherwise the pointer
		// could be interpreted as "truthy" by templating engines.
		It("flattens optional bools to plain values", func() {
			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())

			svcValues, ok := values["service"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(svcValues).To(HaveKeyWithValue("create", false))

			saValues, ok := values["serviceAccount"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(saValues).To(HaveKeyWithValue("create", false))
		})

		// Unset should remain "nil", which is indicates to use the default, and
		// distinguishes from an explicit "false".
		It("omits an optional bool the extension never set", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())

			svcValues, ok := values["service"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(svcValues).ToNot(HaveKey("create"))

			saValues, ok := values["serviceAccount"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(saValues).ToNot(HaveKey("automount"))
		})

		// deploymentType lives on XMiniEnvK8sService rather than ChartValues: it
		// selects which templates get assembled, so it must not leak into the
		// values handed to a chart that has no such key.
		It("omits the deployment type from the chart values", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"deploymentType": "job",
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ChartValues()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("deploymentType"))
		})
	})

	Describe("skip", func() {
		It("reads the skip flag from the extension", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{"skip": true},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Skip).To(BeTrue())
		})

		It("does not require an image when skipped", func() {
			svc = compose.Service{
				Name: "test-service",
				Extensions: types.Extensions{
					config.K8sServiceExtension: map[string]any{"skip": true},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Skip).To(BeTrue())
		})

		It("does not expand a +git tag when skipped", func() {
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{
					"skip":  true,
					"image": map[string]any{"tag": "+git"},
				},
			}

			_, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
