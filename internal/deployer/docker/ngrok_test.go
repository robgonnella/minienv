package docker_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/docker"
	transportmocks "github.com/robgonnella/minienv/internal/transport/mocks"
	"gopkg.in/yaml.v3"
)

const publishingCompose = `
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
`

var _ = Describe("Docker ngrok config", func() {
	deployWith := func(token, compose string) remoteFiles {
		GinkgoHelper()

		mockTransport := transportmocks.NewMockClient(GinkgoT())
		subject := docker.New(docker.Options{
			DockerExt:      config.XMiniEnvDocker{Namespace: testNamespace},
			Transport:      mockTransport,
			NgrokAuthToken: token,
		})

		Expect(subject.Init(context.Background(), projectOnDisk(compose))).
			To(Succeed())

		return captureDeploy(subject, mockTransport)
	}

	Describe("the rendered agent config", func() {
		var rendered string

		BeforeEach(func() {
			rendered = deployWith(testAuthToken, publishingCompose).
				get("ngrok.yml")
		})

		It("renders config the agent can parse", func() {
			var parsed map[string]any

			Expect(yaml.Unmarshal([]byte(rendered), &parsed)).
				To(Succeed(), "not valid yaml: %s", rendered)
			Expect(parsed).To(HaveKeyWithValue("version", 3))
		})

		It("names the endpoint after the namespace and service", func() {
			Expect(rendered).
				To(ContainSubstring("name: " + testNamespace + "-alpha"))
		})

		It("points the upstream at the container port", func() {
			Expect(rendered).To(ContainSubstring("url: alpha:8080"))
		})

		It("omits a url that was never configured", func() {
			Expect(rendered).ToNot(ContainSubstring("\n    url:"))
		})

		It("carries no auth token", func() {
			Expect(rendered).ToNot(ContainSubstring(testAuthToken))
		})
	})

	Describe("a configured endpoint url", func() {
		It("renders the reserved domain", func() {
			rendered := deployWith(testAuthToken, `
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
        url: https://alpha.example.ngrok.app
`).get("ngrok.yml")

			Expect(rendered).
				To(ContainSubstring("url: https://alpha.example.ngrok.app"))
		})

		It("indents a multi-line traffic policy under the endpoint", func() {
			rendered := deployWith(testAuthToken, `
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
        trafficPolicy: |
          on_http_request:
            - actions:
                - type: deny
`).get("ngrok.yml")

			var parsed map[string]any

			Expect(yaml.Unmarshal([]byte(rendered), &parsed)).
				To(Succeed(), "not valid yaml: %s", rendered)
			Expect(rendered).To(ContainSubstring("traffic_policy:"))
			Expect(rendered).To(ContainSubstring("on_http_request:"))
		})
	})

	Describe("the order endpoints resolve in", func() {
		const twoServices = `
name: test-project
services:
  beta:
    image: reg/beta:v1
    ports:
      - 9090:9090
    x-minienv-docker-service:
      ngrok:
        port: 9090
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
`

		It("renders the same config every time", func() {
			first := deployWith(testAuthToken, twoServices).get("ngrok.yml")

			for range 5 {
				Expect(deployWith(testAuthToken, twoServices).get("ngrok.yml")).
					To(Equal(first))
			}
		})

		It("gives two services their own endpoints", func() {
			rendered := deployWith(testAuthToken, twoServices).get("ngrok.yml")

			Expect(rendered).To(ContainSubstring(testNamespace + "-alpha"))
			Expect(rendered).To(ContainSubstring(testNamespace + "-beta"))
		})
	})

	Describe("services that publish nothing", func() {
		It("skips a service flagged skip", func() {
			files := deployWith(testAuthToken, `
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
  beta:
    image: reg/beta:v1
    ports:
      - 9090:9090
    x-minienv-docker-service:
      skip: true
      ngrok:
        port: 9090
`)

			Expect(files.get("ngrok.yml")).ToNot(ContainSubstring("beta"))
		})

		It("writes no agent config without an auth token", func() {
			files := deployWith("", publishingCompose)

			Expect(files).ToNot(HaveKey(remoteDir + "/ngrok.yml"))
		})
	})

	Describe("the checksum on the injected agent", func() {
		checksumOf := func(token, compose string) string {
			GinkgoHelper()

			var parsed struct {
				Services map[string]struct {
					Labels map[string]string `yaml:"labels"`
				} `yaml:"services"`
			}

			rendered := deployWith(token, compose).get("compose.yml")
			Expect(yaml.Unmarshal([]byte(rendered), &parsed)).To(Succeed())

			ngrok, ok := parsed.Services["ngrok"]
			Expect(ok).To(BeTrue(), "no ngrok service in %s", rendered)

			return ngrok.Labels[config.NgrokConfigChecksumLabel]
		}

		const withBeta = `
name: test-project
services:
  alpha:
    image: reg/alpha:v1
    ports:
      - 8080:8080
    x-minienv-docker-service:
      ngrok:
        port: 8080
  beta:
    image: reg/beta:v1
    ports:
      - 9090:9090
    x-minienv-docker-service:
      ngrok:
        port: 9090
`

		It("labels the agent with a checksum", func() {
			Expect(checksumOf(testAuthToken, publishingCompose)).ToNot(BeEmpty())
		})

		It("stays stable for the same endpoints and token", func() {
			Expect(checksumOf(testAuthToken, publishingCompose)).
				To(Equal(checksumOf(testAuthToken, publishingCompose)))
		})

		It("changes when the token rotates", func() {
			Expect(checksumOf(testAuthToken, publishingCompose)).
				ToNot(Equal(checksumOf("a-different-token", publishingCompose)))
		})

		It("changes when an endpoint is added", func() {
			Expect(checksumOf(testAuthToken, publishingCompose)).
				ToNot(Equal(checksumOf(testAuthToken, withBeta)))
		})
	})

	Describe("the compose project the remote receives", func() {
		var rendered string

		BeforeEach(func() {
			rendered = deployWith(testAuthToken, publishingCompose).
				get("compose.yml")
		})

		It("declares the token variable without its value", func() {
			Expect(rendered).ToNot(ContainSubstring(testAuthToken))
			Expect(rendered).To(ContainSubstring("NGROK_AUTHTOKEN: null"))
		})

		// Guards the ordering in modifyComposeContent.
		It("keeps the agent's empty entrypoint", func() {
			Expect(rendered).To(ContainSubstring("entrypoint: []"))
		})

		It("mounts the agent config read-only from the namespaced directory", func() {
			Expect(rendered).
				To(ContainSubstring("source: " + remoteDir + "/ngrok.yml"))
			Expect(rendered).To(ContainSubstring("read_only: true"))
		})

		It("makes the agent wait on every published service", func() {
			Expect(strings.Count(rendered, "condition: service_started")).
				To(Equal(1))
		})
	})
})
