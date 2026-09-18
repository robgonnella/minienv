package docker_test

import (
	"context"
	"path/filepath"

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

	Describe("copying declared paths into the rewritten project", func() {
		const copyCompose = `
name: test-project
services:
  api:
    image: repo/api:v1
    volumes:
      - ./conf:/etc/conf
    x-minienv-docker-service:
      copy:
        - hostPath: conf/app.yml
          containerPath: /etc/app/app.yml
        - hostPath: nested/deep/app.yml
          containerPath: /etc/deep/app.yml
  worker:
    image: repo/worker:v1
    x-minienv-docker-service:
      skip: true
      copy:
        - hostPath: conf/worker.yml
          containerPath: /etc/worker.yml
`

		var (
			rendered string
			copies   copiedPaths
			workDir  string
		)

		BeforeEach(func() {
			mockTransport := transportmocks.NewMockClient(GinkgoT())
			mockImage := imagemocks.NewMockClient(GinkgoT())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(nil).
				Maybe()

			project := projectOnDiskWith(copyCompose, map[string]string{
				"conf/app.yml":        "declared: yes",
				"conf/worker.yml":     "declared: yes",
				"nested/deep/app.yml": "declared: yes",
			})
			workDir = project.WorkingDir

			subject := docker.New(docker.Options{
				DockerExt:   config.XMiniEnvDocker{Namespace: testNamespace},
				ServiceDirs: map[string]string{"api": workDir, "worker": workDir},
				Transport:   mockTransport,
				ImageClient: mockImage,
			})

			Expect(subject.Init(context.Background(), project)).To(Succeed())

			files, copied := captureDeployAndCopies(subject, mockTransport)
			rendered = files.get("compose.yml")
			copies = copied
		})

		It("mounts each declared path under the service's copies directory", func() {
			Expect(rendered).To(ContainSubstring(
				"source: " + remoteDir + "/copies/api/conf/app.yml",
			))
			Expect(rendered).To(ContainSubstring("target: /etc/app/app.yml"))
		})

		It("keeps a nested path's directories apart", func() {
			Expect(rendered).To(ContainSubstring(
				"source: " + remoteDir + "/copies/api/nested/deep/app.yml",
			))
		})

		It("still drops the compose bind mount it did not declare", func() {
			Expect(rendered).ToNot(ContainSubstring("/etc/conf"))
		})

		It("copies each declared path from the project directory", func() {
			Expect(copies).To(HaveKeyWithValue(
				filepath.Join(workDir, "conf/app.yml"),
				remoteDir+"/copies/api/conf/app.yml",
			))
			Expect(copies).To(HaveKeyWithValue(
				filepath.Join(workDir, "nested/deep/app.yml"),
				remoteDir+"/copies/api/nested/deep/app.yml",
			))
		})

		It("copies nothing for a skipped service", func() {
			Expect(copies).ToNot(HaveKey(
				filepath.Join(workDir, "conf/worker.yml"),
			))
			Expect(rendered).ToNot(ContainSubstring("/etc/worker.yml"))
		})
	})

	Describe("copy paths of a service mapped to its own directory", func() {
		const copyCompose = `
name: test-project
services:
  api:
    image: repo/api:v1
    x-minienv-docker-service:
      copy:
        - hostPath: conf/app.yml
          containerPath: /etc/app/app.yml
  db:
    image: repo/db:v1
    x-minienv-docker-service:
      copy:
        - hostPath: conf/app.yml
          containerPath: /etc/db/app.yml
`

		var (
			rendered string
			copies   copiedPaths
			apiDir   string
			dbDir    string
		)

		BeforeEach(func() {
			mockTransport := transportmocks.NewMockClient(GinkgoT())
			mockImage := imagemocks.NewMockClient(GinkgoT())

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(nil).
				Maybe()

			apiDir = GinkgoT().TempDir()
			dbDir = GinkgoT().TempDir()

			subject := docker.New(docker.Options{
				DockerExt:   config.XMiniEnvDocker{Namespace: testNamespace},
				ServiceDirs: map[string]string{"api": apiDir, "db": dbDir},
				Transport:   mockTransport,
				ImageClient: mockImage,
			})

			project := projectOnDiskWith(copyCompose, nil)

			Expect(subject.Init(context.Background(), project)).To(Succeed())

			files, copied := captureDeployAndCopies(subject, mockTransport)
			rendered = files.get("compose.yml")
			copies = copied
		})

		It("copies each service's path from its own directory", func() {
			Expect(copies).To(HaveKeyWithValue(
				filepath.Join(apiDir, "conf/app.yml"),
				remoteDir+"/copies/api/conf/app.yml",
			))
			Expect(copies).To(HaveKeyWithValue(
				filepath.Join(dbDir, "conf/app.yml"),
				remoteDir+"/copies/db/conf/app.yml",
			))
		})

		It("keeps two services' identical hostPaths apart on the remote", func() {
			Expect(rendered).To(ContainSubstring(
				"source: " + remoteDir + "/copies/api/conf/app.yml",
			))
			Expect(rendered).To(ContainSubstring(
				"source: " + remoteDir + "/copies/db/conf/app.yml",
			))
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
