package config_test

import (
	"errors"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
)

func duration(d time.Duration) *types.Duration {
	converted := types.Duration(d)
	return &converted
}

func strPtr(s string) *string {
	return &s
}

func k8sExt() *config.XMiniEnv {
	return &config.XMiniEnv{
		K8s: config.XMiniEnvK8s{
			Context:   "context",
			Namespace: "namespace",
		},
	}
}

// The git failure the fake returns; asserted by identity so the spec proves
// resolveServiceImage preserved the cause.
var errNotARepo = errors.New("not a repo")

var _ = Describe("XMiniEnvK8sService", func() {
	var (
		mainExt      *config.XMiniEnv
		svc          config.ComposeService
		mockGit      *gitmocks.MockClient
		ngrokEnabled bool
	)

	// Options are assembled lazily rather than in BeforeEach because specs
	// mutate mainExt, svc and ngrokEnabled in their own bodies before
	// resolving.
	newSvcExt := func() (*config.XMiniEnvK8sService, error) {
		return config.NewXMiniEnvK8sService(config.XMiniEnvK8sServiceOptions{
			MainExt:      mainExt,
			Service:      svc,
			GitClient:    mockGit,
			NgrokEnabled: ngrokEnabled,
		})
	}

	BeforeEach(func() {
		mainExt = k8sExt()
		mockGit = gitmocks.NewMockClient(GinkgoT())
		ngrokEnabled = false
		svc = config.ComposeService{
			Name:  "test-service",
			Image: "reg/test-service:v1",
		}
	})

	Describe("required configuration", func() {
		It("returns a config error if k8s is not configured", func() {
			mainExt = &config.XMiniEnv{}
			svc = config.ComposeService{Name: "test-service"}

			_, err := newSvcExt()

			Expect(err).To(MatchError(config.KindK8sNotConfigured))
		})

		It("errors if the service has neither an image nor an extension", func() {
			svc = config.ComposeService{Name: "test-service"}

			_, err := newSvcExt()

			// resolveServiceImage accumulates both failures with errors.Join;
			// errors.Is walks the join, so this asserts both were collected.
			Expect(err).To(MatchError(config.KindImageRepositoryMissing))
			Expect(err).To(MatchError(config.KindImageTagMissing))
		})
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
				config.K8S_SERVICE_EXTENSION: map[string]any{
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
				mockGit.EXPECT().ShortSha().Return("abc1234", nil).Once()

				result, err := newSvcExt()

				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Image.Tag).To(Equal("abc1234"))
			})

			It("returns a config error when git fails", func() {
				mockGit.
					EXPECT().
					ShortSha().
					Return("", errNotARepo).
					Once()

				_, err := newSvcExt()

				Expect(err).To(MatchError(config.KindGitShortSha))
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
					ServicePortName:   "p3000",
					ServicePort:       3000,
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
			Expect(result.Service.Ports[0].ServicePort).To(BeEquivalentTo(65535))
		})

		It("rejects a container port beyond the 16-bit range", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 70000, Published: "3000"},
			}

			_, err := newSvcExt()

			Expect(err).To(MatchError(config.KindInvalidPort))
		})

		It("returns a config error for a mapping with no published port", func() {
			svc.Ports = []types.ServicePortConfig{{Target: 8080}}

			_, err := newSvcExt()

			Expect(err).To(MatchError(config.KindInvalidPublishedPort))
		})

		It("does not duplicate a port already declared in the extension", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"service": map[string]any{
						"ports": []any{
							map[string]any{
								"containerPortName": "http",
								"containerPort":     8080,
								"servicePortName":   "http",
								"servicePort":       3000,
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
			Expect(result.Service.Create).To(BeFalse())
			Expect(result.ServiceAccount.Create).To(BeFalse())
		})
	})

	Describe("environment resolution", func() {
		It("merges compose environment into the chart values", func() {
			svc.Environment = types.MappingWithEquals{
				"FOO": strPtr("bar"),
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal(map[string]string{"FOO": "bar"}))
		})

		It("lets the extension env win over the compose env", func() {
			svc.Environment = types.MappingWithEquals{
				"FOO": strPtr("from-compose"),
				"BAZ": strPtr("qux"),
			}
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"env": map[string]any{"FOO": "from-extension"},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal(map[string]string{
				"FOO": "from-extension",
				"BAZ": "qux",
			}))
		})

		It("omits unresolved pass-through variables instead of panicking", func() {
			// `environment: [FOO]` that compose could not resolve from the host
			// arrives as a nil *string.
			svc.Environment = types.MappingWithEquals{
				"FOO":      nil,
				"RESOLVED": strPtr("value"),
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Env).To(Equal(map[string]string{"RESOLVED": "value"}))
			Expect(result.Env).ToNot(HaveKey("FOO"))
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
				config.K8S_SERVICE_EXTENSION: map[string]any{
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
				To(Equal(config.HELM_DEFAULT_DEPLOYMENT_TIMEOUT))
		})

		It("prefers the top-level timeout over the default", func() {
			mainExt.K8s.DeploymentTimeout = "5m"

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.DeploymentTimeout).To(Equal("5m"))
		})

		It("prefers the service timeout over the top-level timeout", func() {
			mainExt.K8s.DeploymentTimeout = "5m"
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
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
			mainExt.Ngrok.TrafficPolicy = "top-level-policy"
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"ngrok": map[string]any{"port": 3000},
				},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Ngrok.TrafficPolicy).To(Equal("top-level-policy"))
		})

		It("lets the service traffic policy win", func() {
			ngrokEnabled = true
			mainExt.Ngrok.TrafficPolicy = "top-level-policy"
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"ngrok": map[string]any{
						"port":          3000,
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
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"ngrok": map[string]any{"port": 3000},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Volumes).To(BeEmpty())

			values, err := result.ToValuesMap()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).ToNot(HaveKey("ngrok"))
		})

		It("clears all ngrok config when no auth token is available", func() {
			mainExt.Ngrok.TrafficPolicy = "top-level-policy"
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"ngrok": map[string]any{
						"port":          3000,
						"url":           "https://example.ngrok.app",
						"trafficPolicy": "service-policy",
					},
				},
			}

			result, err := newSvcExt()

			// Downstream code treats a zero Ngrok.Port as "ngrok is off", so
			// nothing may survive resolution when there is no token to use.
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Ngrok).To(Equal(config.Ngrok{}))
		})

		Context("with an auth token set", func() {
			BeforeEach(func() {
				ngrokEnabled = true
			})

			It("appends the config volume, preserving user volumes", func() {
				svc.Extensions = types.Extensions{
					config.K8S_SERVICE_EXTENSION: map[string]any{
						"ngrok":   map[string]any{"port": 3000},
						"volumes": []any{map[string]any{"name": "user-vol"}},
					},
				}

				result, err := newSvcExt()

				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Volumes).To(HaveLen(2))
				Expect(result.Volumes[0]).To(HaveKeyWithValue("name", "user-vol"))
				Expect(result.Volumes[1]).To(
					HaveKeyWithValue("name", config.NGROK_CONFIG_MAP_NAME),
				)
			})

			It("populates every ngrok key the chart templates read", func() {
				svc.Extensions = types.Extensions{
					config.K8S_SERVICE_EXTENSION: map[string]any{
						"ngrok": map[string]any{
							"port":          3000,
							"url":           "https://example.ngrok.app",
							"trafficPolicy": "policy",
						},
					},
				}

				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())

				values, err := result.ToValuesMap()
				Expect(err).ShouldNot(HaveOccurred())

				Expect(values["ngrok"]).To(Equal(map[string]any{
					"enabled":            true,
					"port":               uint16(3000),
					"image":              config.NGROK_IMAGE,
					"configMapName":      config.NGROK_CONFIG_MAP_NAME,
					"configKey":          config.NGROK_CONFIG_KEY,
					"configVolMountPath": config.NGROK_CONFIG_VOL_MOUNT_PATH,
					"secretName":         config.NGROK_SECRET_NAME,
					"url":                "https://example.ngrok.app",
					"trafficPolicy":      "policy",
				}))
			})

			It("errors when the ngrok port matches no service port", func() {
				svc.Extensions = types.Extensions{
					config.K8S_SERVICE_EXTENSION: map[string]any{
						"ngrok": map[string]any{"port": 9999},
					},
				}

				result, err := newSvcExt()
				Expect(err).ShouldNot(HaveOccurred())

				_, err = result.ToValuesMap()
				Expect(err).To(MatchError(config.KindNgrokPortMismatch))
			})
		})
	})

	Describe("ToValuesMap", func() {
		It("decodes service ports into the nested chart shape", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: 8080, Published: "3000"},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ToValuesMap()
			Expect(err).ShouldNot(HaveOccurred())

			svcValues, ok := values["service"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(svcValues["ports"]).To(Equal([]map[string]any{
				{
					"containerPortName": "p8080",
					"containerPort":     uint16(8080),
					"servicePortName":   "p3000",
					"servicePort":       uint16(3000),
					"protocol":          "TCP",
				},
			}))
		})

		It("decodes image pull secrets into the nested chart shape", func() {
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"imagePullSecrets": []any{
						map[string]any{"name": "regcred"},
					},
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ToValuesMap()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values["imagePullSecrets"]).To(Equal([]map[string]any{
				{"name": "regcred"},
			}))
		})

		It("carries replicas through under the key the chart reads", func() {
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{
					"replicas": 3,
				},
			}

			result, err := newSvcExt()
			Expect(err).ShouldNot(HaveOccurred())

			values, err := result.ToValuesMap()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(values).To(HaveKeyWithValue("replicas", uint8(3)))
		})
	})

	Describe("skip", func() {
		It("reads the skip flag from the extension", func() {
			svc.Extensions = types.Extensions{
				config.K8S_SERVICE_EXTENSION: map[string]any{"skip": true},
			}

			result, err := newSvcExt()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.Skip).To(BeTrue())
		})
	})
})
