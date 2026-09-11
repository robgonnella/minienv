package docker_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/docker"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	transportmocks "github.com/robgonnella/minienv/internal/transport/mocks"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Docker", func() {
	Describe("validating the extension", func() {
		newDeployer := func(namespace string) *docker.Docker {
			return docker.New(docker.Options{
				DockerExt: config.XMiniEnvDocker{Namespace: namespace},
			})
		}

		It("rejects an empty namespace on Init", func() {
			err := newDeployer("").Init(
				context.Background(),
				config.ComposeProject{},
			)

			Expect(err).To(MatchError(docker.ErrInvalidExtension))
		})

		It("rejects an empty namespace on Deploy", func() {
			err := newDeployer("").Deploy(context.Background())

			Expect(err).To(MatchError(docker.ErrInvalidExtension))
		})

		It("rejects an empty namespace on Destroy", func() {
			err := newDeployer("").Destroy(context.Background())

			Expect(err).To(MatchError(docker.ErrInvalidExtension))
		})

		It("accepts a populated namespace", func() {
			err := newDeployer("my-namespace").Init(
				context.Background(),
				config.ComposeProject{},
			)

			Expect(err).ToNot(HaveOccurred())
		})

		// Would otherwise aim Destroy's removal at ~/.minienv itself.
		It("rejects a namespace that normalizes away to nothing", func() {
			err := newDeployer("!!!").Init(
				context.Background(),
				config.ComposeProject{},
			)

			Expect(err).To(MatchError(docker.ErrInvalidExtension))
		})
	})

	Describe("rewriting the compose project for the remote host", func() {
		const composeFile = `
name: original
services:
  api:
    image: repo/api:v1
    build: .
    ports:
      - 8080:8080
    volumes:
      - ./conf:/etc/conf
      - data:/var/lib/data
    environment:
      GREETING: ${GREETING_VAR}
      PASSWORD: "AESp$$ssw0rd"
    depends_on:
      worker:
        condition: service_started
  worker:
    image: repo/worker:v1
    x-minienv-docker-service:
      skip: true
volumes:
  data:
`

		var rendered string

		BeforeEach(func() {
			GinkgoT().Setenv("GREETING_VAR", "hello")

			mockTransport := transportmocks.NewMockClient(GinkgoT())
			mockImage := imagemocks.NewMockClient(GinkgoT())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(nil).
				Once()

			subject := docker.New(docker.Options{
				DockerExt:   config.XMiniEnvDocker{Namespace: "my-namespace"},
				Transport:   mockTransport,
				ImageClient: mockImage,
			})

			Expect(subject.Init(
				context.Background(),
				projectOnDisk(composeFile),
			)).To(Succeed())

			rendered = captureDeploy(subject, mockTransport).get("compose.yml")
		})

		It("renames the project to the namespace", func() {
			Expect(rendered).To(ContainSubstring("name: my-namespace"))
		})

		It("resolves interpolation rather than shipping it empty", func() {
			Expect(rendered).To(ContainSubstring("GREETING: hello"))
		})

		It("escapes a literal dollar against the remote's own pass", func() {
			Expect(rendered).To(ContainSubstring("AESp$$ssw0rd"))
		})

		It("drops the skipped service and the edge pointing at it", func() {
			Expect(rendered).ToNot(ContainSubstring("worker"))
			Expect(rendered).ToNot(ContainSubstring("depends_on"))
		})

		It("strips what the remote must not act on", func() {
			Expect(rendered).ToNot(ContainSubstring("build:"))
			Expect(rendered).ToNot(ContainSubstring("ports:"))
		})

		It("keeps named volumes and drops bind mounts", func() {
			Expect(rendered).ToNot(ContainSubstring("/etc/conf"))
			Expect(rendered).To(ContainSubstring("/var/lib/data"))
		})

		It("does not inject ngrok when nothing publishes", func() {
			Expect(rendered).ToNot(ContainSubstring("ngrok"))
		})
	})

	Describe("resolving the image for the remote", func() {
		renderWith := func(compose string) string {
			GinkgoHelper()

			mockTransport := transportmocks.NewMockClient(GinkgoT())
			mockImage := imagemocks.NewMockClient(GinkgoT())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(nil).
				Maybe()

			subject := docker.New(docker.Options{
				DockerExt:   config.XMiniEnvDocker{Namespace: "my-namespace"},
				Transport:   mockTransport,
				ImageClient: mockImage,
			})

			Expect(subject.Init(context.Background(), projectOnDisk(compose))).
				To(Succeed())

			return captureDeploy(subject, mockTransport).get("compose.yml")
		}

		It("uses the extension image when the service declares none", func() {
			Expect(renderWith(`
name: test-project
services:
  api:
    build: .
    x-minienv-docker-service:
      image:
        repository: repo/api
        tag: v9
`)).To(ContainSubstring("image: repo/api:v9"))
		})

		It("leaves an existing compose image alone", func() {
			Expect(renderWith(`
name: test-project
services:
  api:
    image: repo/api:v1
`)).To(ContainSubstring("image: repo/api:v1"))
		})
	})

	Describe("the remote directory", func() {
		It("normalizes a namespace that is not a usable project name", func() {
			mockTransport := transportmocks.NewMockClient(GinkgoT())
			subject := docker.New(docker.Options{
				DockerExt: config.XMiniEnvDocker{Namespace: "My Branch"},
				Transport: mockTransport,
			})

			Expect(subject.Init(context.Background(), projectOnDisk(`
name: test-project
services:
  api:
    image: repo/api:v1
`))).To(Succeed())

			Expect(captureDeploy(subject, mockTransport)).
				To(HaveKey("~/.minienv/mybranch/compose.yml"))
		})
	})
})
