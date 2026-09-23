package resolver_test

import (
	"context"
	"math"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/resolver"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("DockerService", func() {
	var (
		dockerExt config.XMiniEnvDocker
		svc       compose.Service
		dir       string
		mockGit   *gitmocks.MockClient
	)

	newSvcExtWithNgrok := func(
		ngrokEnabled bool,
	) (*resolver.DockerService, error) {
		return resolver.NewDockerService(
			context.Background(),
			resolver.DockerServiceOptions{
				DockerExt:    dockerExt,
				Service:      svc,
				Dir:          dir,
				GitClient:    mockGit,
				NgrokEnabled: ngrokEnabled,
			},
		)
	}

	newSvcExt := func() (*resolver.DockerService, error) {
		return newSvcExtWithNgrok(false)
	}

	BeforeEach(func() {
		dockerExt = config.XMiniEnvDocker{Namespace: "namespace"}
		mockGit = gitmocks.NewMockClient(GinkgoT())
		svc = compose.Service{Name: "test-service"}
		dir = ""
	})

	It("keeps the compose service it was resolved from", func() {
		svc.Image = "reg/app:v1"

		svcExt, err := newSvcExt()

		Expect(err).ToNot(HaveOccurred())
		Expect(svcExt.Compose).To(Equal(svc))
	})

	Describe("resolving the image from the compose service", func() {
		DescribeTable(
			"splits the reference into repository and tag",
			func(image, wantRepo, wantTag string) {
				svc.Image = image

				svcExt, err := newSvcExt()

				Expect(err).ToNot(HaveOccurred())
				Expect(svcExt.Image.Repository).To(Equal(wantRepo))
				Expect(svcExt.Image.Tag).To(Equal(wantTag))
			},
			Entry("a plain repository and tag", "alpine:3.24", "alpine", "3.24"),
			Entry("an untagged reference", "alpine", "alpine", "latest"),
			Entry(
				"a namespaced untagged reference",
				"library/alpine",
				"library/alpine",
				"latest",
			),
			Entry(
				"a registry port and a tag",
				"registry.local:5000/app:v1",
				"registry.local:5000/app",
				"v1",
			),
			Entry(
				"a registry port and no tag",
				"registry.local:5000/app",
				"registry.local:5000/app",
				"latest",
			),
		)

		It("rejects a digest reference rather than mis-splitting it", func() {
			svc.Image = "app@sha256:abc123"

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrImageDigestUnsupported))
		})

		It("lets the extension override the compose image", func() {
			svc.Image = "alpine:3.24"
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{
					"image": map[string]any{
						"repository": "repo/override",
						"tag":        "v2",
					},
				},
			}

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Image.Repository).To(Equal("repo/override"))
			Expect(svcExt.Image.Tag).To(Equal("v2"))
		})

		It("errors when there is no image anywhere", func() {
			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrImageRepositoryMissing))
		})
	})

	Describe("resolving ngrok", func() {
		// ngrok resolves to nothing without a token, so every spec here needs
		// one; the token-absent case is asserted on its own below.
		newEnabledSvcExt := func() (*resolver.DockerService, error) {
			return newSvcExtWithNgrok(true)
		}

		withNgrok := func(ngrok map[string]any) {
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{"ngrok": ngrok},
			}
		}

		BeforeEach(func() {
			svc.Image = "reg/app:v1"
			svc.Ports = []types.ServicePortConfig{
				{Target: 3000, Published: "8080"},
			}
		})

		It("accepts the container side of a mapping", func() {
			withNgrok(map[string]any{"port": 3000})

			svcExt, err := newEnabledSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Ngrok.Port).To(BeEquivalentTo(3000))
		})

		It("rejects the published side of a mapping", func() {
			withNgrok(map[string]any{"port": 8080})

			_, err := newEnabledSvcExt()

			Expect(err).To(MatchError(resolver.ErrNgrokPortMismatch))
		})

		It("rejects a port when the service declares none", func() {
			svc.Ports = nil

			withNgrok(map[string]any{"port": 3000})

			_, err := newEnabledSvcExt()

			Expect(err).To(MatchError(resolver.ErrNgrokPortMismatch))
		})

		It("rejects a container port that does not fit a uint16", func() {
			svc.Ports = []types.ServicePortConfig{
				{Target: math.MaxUint16 + 1, Published: "8080"},
			}

			withNgrok(map[string]any{"port": 3000})

			_, err := newEnabledSvcExt()

			Expect(err).To(MatchError(resolver.ErrInvalidPort))
		})

		It("inherits the top-level traffic policy", func() {
			dockerExt.Ngrok = &config.NgrokTopLevel{TrafficPolicy: "top"}

			withNgrok(map[string]any{"port": 3000})

			svcExt, err := newEnabledSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Ngrok.TrafficPolicy).To(Equal("top"))
		})

		It("lets the service override the top-level traffic policy", func() {
			dockerExt.Ngrok = &config.NgrokTopLevel{TrafficPolicy: "top"}

			withNgrok(map[string]any{
				"port":          3000,
				"trafficPolicy": "mine",
			})

			svcExt, err := newEnabledSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Ngrok.TrafficPolicy).To(Equal("mine"))
		})

		It("zeroes the whole block with no auth token", func() {
			dockerExt.Ngrok = &config.NgrokTopLevel{TrafficPolicy: "top"}

			withNgrok(map[string]any{
				"port":          3000,
				"url":           "https://example.ngrok.app",
				"trafficPolicy": "mine",
			})

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Ngrok).To(Equal(config.NgrokServiceLevel{}))
		})
	})

	Describe("common service properties", func() {
		It("decodes skip through the squashed common struct", func() {
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{"skip": true},
			}

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Skip).To(BeTrue())
		})

		DescribeTable(
			"decodes an interpolated skip string",
			func(value string, want bool) {
				svc.Image = "reg/app:v1"
				svc.Extensions = map[string]any{
					config.DockerServiceExtension: map[string]any{"skip": value},
				}

				svcExt, err := newSvcExt()

				Expect(err).ToNot(HaveOccurred())
				Expect(svcExt.Skip).To(Equal(want))
			},
			Entry("true", "true", true),
			Entry("false", "false", false),
			Entry("unset variable", "", false),
		)

		It("rejects a skip string that is not a bool", func() {
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{"skip": "garbage"},
			}

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrExtensionDecode))
		})

		It("expands a +git tag from the service directory", func() {
			svc.Image = "reg/app:+git"
			dir = "/repo/web"

			mockGit.
				EXPECT().
				ShortSha(mock.Anything, "/repo/web").
				Return("abc1234", nil).
				Once()

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Image.Tag).To(Equal("abc1234"))
		})

		It("does not expand a +git tag when skipped", func() {
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{
					"skip":  true,
					"image": map[string]any{"tag": "+git"},
				},
			}

			_, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
		})

		It("defaults skip to false when the extension is absent", func() {
			svc.Image = "reg/app:v1"

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Skip).To(BeFalse())
		})
	})

	Describe("copy", func() {
		declare := func(entries ...map[string]any) {
			svc.Image = "reg/app:v1"
			svc.Extensions = map[string]any{
				config.DockerServiceExtension: map[string]any{
					"copy": entries,
				},
			}
		}

		It("carries the declared entries through", func() {
			declare(
				map[string]any{
					"hostPath":      "conf/app.yml",
					"containerPath": "/etc/app/app.yml",
				},
				map[string]any{
					"hostPath":      "seed",
					"containerPath": "/var/seed",
				},
			)

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Copy).To(Equal([]config.XMiniEnvDockerCopy{
				{HostPath: "conf/app.yml", ContainerPath: "/etc/app/app.yml"},
				{HostPath: "seed", ContainerPath: "/var/seed"},
			}))
		})

		It("leaves them empty when none are declared", func() {
			svc.Image = "reg/app:v1"

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Copy).To(BeEmpty())
		})

		It("cleans each declared host path", func() {
			declare(map[string]any{
				"hostPath":      "./conf/../conf/app.yml",
				"containerPath": "/etc/app/app.yml",
			})

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.Copy[0].HostPath).To(Equal("conf/app.yml"))
		})

		It("resolves the local path against the declaring directory", func() {
			dir = filepath.Join("proj", "api")

			declare(map[string]any{
				"hostPath":      "conf/app.yml",
				"containerPath": "/etc/app/app.yml",
			})

			svcExt, err := newSvcExt()

			Expect(err).ToNot(HaveOccurred())
			Expect(svcExt.CopyLocalPath(svcExt.Copy[0])).
				To(Equal(filepath.Join("proj", "api", "conf", "app.yml")))
		})

		DescribeTable(
			"rejects a host path that leaves the project",
			func(declared string) {
				declare(map[string]any{
					"hostPath":      declared,
					"containerPath": "/etc/app",
				})

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrCopyHostPath))
			},
			Entry("an absolute path", "/etc/app.yml"),
			Entry("a path climbing out", "../app.yml"),
			Entry("a path climbing out after cleaning", "conf/../../app.yml"),
			Entry("an empty path", ""),
			Entry("the project directory itself", "."),
		)

		DescribeTable(
			"rejects an unusable container path",
			func(declared string) {
				declare(map[string]any{
					"hostPath":      "conf/app.yml",
					"containerPath": declared,
				})

				_, err := newSvcExt()

				Expect(err).To(MatchError(resolver.ErrCopyContainerPath))
			},
			Entry("a relative path", "etc/app.yml"),
			Entry("a dot-relative path", "./etc/app.yml"),
			Entry("an empty path", ""),
		)

		It("rejects two entries mounting the same container path", func() {
			declare(
				map[string]any{
					"hostPath":      "conf/app.yml",
					"containerPath": "/etc/app.yml",
				},
				map[string]any{
					"hostPath":      "other/app.yml",
					"containerPath": "/etc/app.yml",
				},
			)

			_, err := newSvcExt()

			Expect(err).To(MatchError(resolver.ErrCopyContainerPath))
		})
	})

	Describe("XMiniEnvDockerTransport", func() {
		It("names ssh as its only config field", func() {
			transport := config.XMiniEnvDockerTransport{}

			Expect(transport.ConfigFields()).To(Equal([]string{"ssh"}))
		})
	})
})
