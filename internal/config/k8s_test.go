package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/mocks"
)

var _ = Describe("XMiniEnvK8sService", func() {
	It("Returns error if k8s is not configured", func() {
		mainExt := config.XMiniEnv{}

		project := config.ComposeService{
			Name: "test-service",
		}

		mockCommander := mocks.NewMockCommander(GinkgoT())

		_, err := config.NewXMiniEnvK8sService(&mainExt, project, mockCommander)
		Expect(err).Should(HaveOccurred())

		_, ok := err.(*config.Error)
		Expect(ok).To(BeTrue())
	})

	It("Errors if missing image and port info", func() {
		mainExt := config.XMiniEnv{
			K8s: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
		}

		project := config.ComposeService{
			Name: "test-service",
		}

		mockCommander := mocks.NewMockCommander(GinkgoT())

		_, err := config.NewXMiniEnvK8sService(&mainExt, project, mockCommander)
		Expect(err).Should(HaveOccurred())
	})
})
