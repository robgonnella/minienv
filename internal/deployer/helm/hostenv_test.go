package helm_test

import (
	"context"

	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
)

var _ = Describe("host environment", func() {
	It("routes the keys attributed to the host into the secret values", func() {
		subject := helm.New(helm.Options{
			K8sExt: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
			HostEnv:   deployer.HostEnv{"hello": []string{"DB_PASSWORD"}},
			GitClient: gitmocks.NewMockClient(GinkgoT()),
		})

		project := config.ComposeProject{
			Name: "test-project",
			Services: types.Services{
				"hello": {
					Name:  "hello",
					Image: "reg/hello:v1",
					Environment: types.MappingWithEquals{
						"DB_PASSWORD": new("hunter2"),
						"LOG_LEVEL":   new("debug"),
					},
				},
			},
		}

		Expect(subject.InitProject(context.Background(), project)).To(Succeed())

		ext := subject.ServiceExtension("hello")

		values, err := ext.ToChartValuesMap()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(values["secretEnv"]).To(Equal(map[string]string{
			"DB_PASSWORD": "hunter2",
		}))
		Expect(values["env"]).To(Equal([]map[string]any{
			{"name": "LOG_LEVEL", "value": "debug"},
		}))
	})
})
