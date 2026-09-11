package docker_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/docker"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	"github.com/robgonnella/minienv/internal/image"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
	publishingmocks "github.com/robgonnella/minienv/internal/publishing/mocks"
	transportmocks "github.com/robgonnella/minienv/internal/transport/mocks"
	"github.com/stretchr/testify/mock"
)

// Asserted by identity, so the specs prove each error passes through untouched.
var (
	errBake      = errors.New("bake blew up")
	errTransport = errors.New("transport blew up")
	errAPI       = errors.New("api blew up")
)

const testNamespace = "my-namespace"

const remoteDir = "~/.minienv/" + testNamespace

const testAuthToken = "2abcTESTTOKENvalue_do_not_log"

func exposedService(
	name string,
	containerPort, publishedPort int,
) config.ComposeService {
	return config.ComposeService{
		Name:  name,
		Image: "reg/" + name + ":v1",
		Ports: []types.ServicePortConfig{
			{
				Target:    uint32(containerPort),
				Published: strconv.Itoa(publishedPort),
			},
		},
		Extensions: types.Extensions{
			config.DockerServiceExtension: map[string]any{
				"ngrok": map[string]any{"port": containerPort},
			},
		},
	}
}

// Deploy reloads from disk, so specs reaching it need real files.
func projectOnDisk(content string) config.ComposeProject {
	GinkgoHelper()

	dir := GinkgoT().TempDir()
	file := filepath.Join(dir, "compose.yml")

	Expect(os.WriteFile(file, []byte(content), 0o600)).To(Succeed())

	opts, err := composecli.NewProjectOptions(
		[]string{file},
		composecli.WithWorkingDirectory(dir),
		composecli.WithOsEnv,
	)
	Expect(err).ToNot(HaveOccurred())

	project, err := opts.LoadProject(context.Background())
	Expect(err).ToNot(HaveOccurred())

	return *project
}

type remoteFiles map[string]string

func (r remoteFiles) get(name string) string {
	GinkgoHelper()

	content, ok := r[remoteDir+"/"+name]
	Expect(ok).To(BeTrue(), "%s was not written, got %v", name, r)

	return content
}

func captureDeploy(
	subject *docker.Docker,
	mockTransport *transportmocks.MockClient,
) remoteFiles {
	GinkgoHelper()

	files := remoteFiles{}

	mockTransport.
		EXPECT().
		CreateFile(mock.Anything, mock.Anything).
		Run(func(path string, content []byte) {
			files[path] = string(content)
		}).
		Return(nil)
	mockTransport.EXPECT().RunCommand(mock.Anything).Return(nil)
	mockTransport.EXPECT().Close().Return(nil)

	Expect(subject.Deploy(context.Background())).To(Succeed())

	return files
}

var _ = Describe("Docker deploy", func() {
	var (
		dockerExt     config.XMiniEnvDocker
		mockTransport *transportmocks.MockClient
		mockImage     *imagemocks.MockClient
		mockGit       *gitmocks.MockClient
		mockPublish   *publishingmocks.MockClient
		subject       *docker.Docker
		ngrokToken    string
		dryRun        bool
	)

	BeforeEach(func() {
		dockerExt = config.XMiniEnvDocker{Namespace: testNamespace}
		mockTransport = transportmocks.NewMockClient(GinkgoT())
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
		mockPublish = publishingmocks.NewMockClient(GinkgoT())
		ngrokToken = ""
		dryRun = false
	})

	JustBeforeEach(func() {
		subject = docker.New(docker.Options{
			DockerExt:      dockerExt,
			Transport:      mockTransport,
			ImageClient:    mockImage,
			GitClient:      mockGit,
			PublishClient:  mockPublish,
			NgrokAuthToken: ngrokToken,
			DryRun:         dryRun,
		})
	})

	Describe("Deployer interface", func() {
		It("identifies itself as the docker deployer", func() {
			Expect(subject.String()).To(Equal("Docker"))
		})
	})

	Describe("before Init has run", func() {
		It("refuses to deploy", func() {
			Expect(subject.Deploy(context.Background())).
				To(MatchError(docker.ErrNotInitialized))
		})

		It("refuses to destroy", func() {
			Expect(subject.Destroy(context.Background())).
				To(MatchError(docker.ErrNotInitialized))
		})
	})

	Describe("which side of a port mapping ngrok.port names", func() {
		var svc config.ComposeService

		BeforeEach(func() {
			ngrokToken = testAuthToken
			svc = exposedService("alpha", 3000, 8080)
		})

		initWithNgrokPort := func(port int) error {
			svc.Extensions[config.DockerServiceExtension] = map[string]any{
				"ngrok": map[string]any{"port": port},
			}

			return subject.Init(context.Background(), config.ComposeProject{
				Name:     "test-project",
				Services: types.Services{"alpha": svc},
			})
		}

		It("accepts the container side", func() {
			Expect(initWithNgrokPort(3000)).To(Succeed())
		})

		It("rejects the published side", func() {
			Expect(initWithNgrokPort(8080)).
				To(MatchError(config.ErrNgrokPortMismatch))
		})
	})

	Describe("PublishedServiceUrls", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			ngrokToken = testAuthToken
			project = config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"alpha": exposedService("alpha", 8080, 8080),
				},
			}
		})

		JustBeforeEach(func() {
			Expect(subject.Init(context.Background(), project)).To(Succeed())
		})

		It("asks only about the endpoints it exposed", func() {
			mockPublish.
				EXPECT().
				ServiceUrls(mock.Anything, []string{testNamespace + "-alpha"}).
				Return(map[string]url.URL{}, nil).
				Once()

			_, err := subject.PublishedServiceUrls(context.Background())

			Expect(err).ToNot(HaveOccurred())
		})

		It("propagates an api failure", func() {
			mockPublish.
				EXPECT().
				ServiceUrls(mock.Anything, mock.Anything).
				Return(nil, errAPI).
				Once()

			_, err := subject.PublishedServiceUrls(context.Background())

			Expect(err).To(MatchError(errAPI))
		})

		Context("with nothing published", func() {
			BeforeEach(func() {
				ngrokToken = ""
			})

			It("resolves nothing without calling the api", func() {
				urls, err := subject.PublishedServiceUrls(context.Background())

				Expect(err).ToNot(HaveOccurred())
				Expect(urls).To(BeEmpty())
			})
		})
	})

	Describe("building images", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			project = projectOnDisk(`
name: test-project
services:
  hello:
    image: reg/hello:v1
    build:
      context: .
      dockerfile: Dockerfile
`)
		})

		JustBeforeEach(func() {
			Expect(subject.Init(context.Background(), project)).To(Succeed())
		})

		It("translates a buildable service into image properties", func() {
			var got []image.ServiceProperties

			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Run(func(_ context.Context, svcs []image.ServiceProperties) {
					got = svcs
				}).
				Return(nil).
				Once()

			captureDeploy(subject, mockTransport)

			Expect(got).To(HaveLen(1))
			Expect(got[0].Name).To(Equal("hello"))
			Expect(got[0].Registry).To(Equal("reg/hello"))
			Expect(got[0].Tag).To(Equal("v1"))
			Expect(got[0].Dockerfile).To(Equal("Dockerfile"))
			Expect(got[0].Platforms).To(Equal([]string{"linux/amd64"}))
		})

		Context("with a service flagged skip", func() {
			BeforeEach(func() {
				project = projectOnDisk(`
name: test-project
services:
  hello:
    image: reg/hello:v1
    build: .
  skipped:
    image: reg/skipped:v1
    build: .
    x-minienv-docker-service:
      skip: true
`)
			})

			It("excludes it from the build", func() {
				mockImage.
					EXPECT().
					BuildAndPush(mock.Anything, mock.MatchedBy(
						func(svcs []image.ServiceProperties) bool {
							return len(svcs) == 1 && svcs[0].Name == "hello"
						},
					)).
					Return(nil).
					Once()

				captureDeploy(subject, mockTransport)
			})

			It("drops it from the rewritten project", func() {
				mockImage.
					EXPECT().
					BuildAndPush(mock.Anything, mock.Anything).
					Return(nil).
					Once()

				files := captureDeploy(subject, mockTransport)

				Expect(files.get("compose.yml")).ToNot(ContainSubstring("skipped"))
			})
		})

		Context("with nothing carrying a build block", func() {
			BeforeEach(func() {
				project = projectOnDisk(`
name: test-project
services:
  hello:
    image: reg/hello:v1
`)
			})

			It("never calls the image client", func() {
				captureDeploy(subject, mockTransport)
			})
		})

		It("propagates an image client failure", func() {
			mockImage.
				EXPECT().
				BuildAndPush(mock.Anything, mock.Anything).
				Return(errBake).
				Once()
			mockTransport.EXPECT().Close().Return(nil).Once()

			Expect(subject.Deploy(context.Background())).To(MatchError(errBake))
		})
	})

	Describe("Deploy", func() {
		var project config.ComposeProject

		BeforeEach(func() {
			project = projectOnDisk(`
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
`)
		})

		JustBeforeEach(func() {
			Expect(subject.Init(context.Background(), project)).To(Succeed())
		})

		Context("with nothing to publish", func() {
			It("writes only the compose file", func() {
				files := captureDeploy(subject, mockTransport)

				Expect(files).To(HaveLen(1))
				Expect(files).To(HaveKey(remoteDir + "/compose.yml"))
			})

			It("removes the files a previous publishing deploy left", func() {
				var issued []string

				mockTransport.
					EXPECT().
					CreateFile(mock.Anything, mock.Anything).
					Return(nil)
				mockTransport.
					EXPECT().
					RunCommand(mock.Anything).
					Run(func(cmd string) {
						issued = append(issued, cmd)
					}).
					Return(nil)
				mockTransport.EXPECT().Close().Return(nil)

				Expect(subject.Deploy(context.Background())).To(Succeed())

				Expect(issued).To(ContainElement(
					"rm -f " + remoteDir + "/.env " + remoteDir + "/ngrok.yml",
				))
			})
		})

		Context("with a service to publish", func() {
			BeforeEach(func() {
				ngrokToken = testAuthToken
			})

			It("writes the compose file, the env file and the agent config", func() {
				files := captureDeploy(subject, mockTransport)

				Expect(files).To(HaveKey(remoteDir + "/compose.yml"))
				Expect(files).To(HaveKey(remoteDir + "/.env"))
				Expect(files).To(HaveKey(remoteDir + "/ngrok.yml"))
			})

			It("carries the token in the env file only", func() {
				files := captureDeploy(subject, mockTransport)

				Expect(files.get(".env")).
					To(Equal("NGROK_AUTHTOKEN=" + testAuthToken))
			})

			It("runs compose up in the namespaced directory", func() {
				var issued []string

				mockTransport.
					EXPECT().
					CreateFile(mock.Anything, mock.Anything).
					Return(nil)
				mockTransport.
					EXPECT().
					RunCommand(mock.Anything).
					Run(func(cmd string) {
						issued = append(issued, cmd)
					}).
					Return(nil)
				mockTransport.EXPECT().Close().Return(nil)

				Expect(subject.Deploy(context.Background())).To(Succeed())

				Expect(issued).To(ContainElement(
					"cd " + remoteDir + " && docker compose up -d --remove-orphans",
				))
			})
		})

		It("does not run the up command when a write fails", func() {
			mockTransport.
				EXPECT().
				CreateFile(mock.Anything, mock.Anything).
				Return(errTransport).
				Once()
			mockTransport.EXPECT().Close().Return(nil).Once()

			Expect(subject.Deploy(context.Background())).
				To(MatchError(errTransport))
		})

		Context("in dry-run mode", func() {
			BeforeEach(func() {
				dryRun = true
				ngrokToken = testAuthToken
			})

			It("writes nothing and opens no session", func() {
				mockTransport.EXPECT().Close().Return(nil).Once()

				Expect(subject.Deploy(context.Background())).To(Succeed())
			})
		})
	})

	Describe("Destroy", func() {
		var issued string

		JustBeforeEach(func() {
			Expect(subject.Init(context.Background(), config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"alpha": exposedService("alpha", 8080, 8080),
				},
			})).To(Succeed())
		})

		runDestroy := func() {
			GinkgoHelper()

			mockTransport.
				EXPECT().
				RunCommand(mock.Anything).
				Run(func(cmd string) { issued = cmd }).
				Return(nil).
				Once()
			mockTransport.EXPECT().Close().Return(nil).Once()

			Expect(subject.Destroy(context.Background())).To(Succeed())
		}

		It("tears down in the namespaced directory", func() {
			runDestroy()

			Expect(issued).To(ContainSubstring("cd " + remoteDir))
			Expect(issued).To(ContainSubstring("docker compose down"))
		})

		It("removes the directory even when compose down fails", func() {
			runDestroy()

			Expect(issued).ToNot(ContainSubstring("down && rm -rf"))
			Expect(issued).To(ContainSubstring("rm -rf " + remoteDir))
			Expect(issued).To(ContainSubstring("exit $status"))
		})

		Context("in dry-run mode", func() {
			BeforeEach(func() {
				dryRun = true
			})

			It("issues no command at all", func() {
				Expect(subject.Destroy(context.Background())).To(Succeed())
			})
		})
	})
})
