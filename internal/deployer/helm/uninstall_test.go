package helm_test

import (
	"context"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmkubefake "helm.sh/helm/v3/pkg/kube/fake"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
)

var _ = Describe("uninstallChart", func() {
	var (
		subject      *helm.Helm
		actionConfig *helmaction.Configuration
		dryRun       bool
	)

	BeforeEach(func() {
		dryRun = false
		actionConfig = &helmaction.Configuration{
			Releases:   helmstorage.Init(helmdriver.NewMemory()),
			KubeClient: &helmkubefake.PrintingKubeClient{Out: io.Discard},
			Log:        func(string, ...any) {},
		}
	})

	JustBeforeEach(func() {
		subject = helm.New(helm.Options{
			K8sExt: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
			DryRun: dryRun,
		})
		subject.SetActionConfig(actionConfig)
	})

	Describe("when the release was never installed", func() {
		It("succeeds", func() {
			err := subject.UninstallChart(context.Background(), "ngrok")

			Expect(err).ShouldNot(HaveOccurred())
		})

		Describe("in dry-run mode", func() {
			BeforeEach(func() {
				dryRun = true
			})

			It("succeeds", func() {
				err := subject.UninstallChart(context.Background(), "ngrok")

				Expect(err).ShouldNot(HaveOccurred())
			})
		})
	})

	Describe("when the release exists in dry-run mode", func() {
		BeforeEach(func() {
			dryRun = true

			Expect(actionConfig.Releases.Create(helmrelease.Mock(
				&helmrelease.MockReleaseOptions{
					Name:      "ngrok",
					Namespace: "namespace",
					Status:    helmrelease.StatusDeployed,
				},
			))).To(Succeed())
		})

		It("succeeds and leaves the release in place", func() {
			err := subject.UninstallChart(context.Background(), "ngrok")

			Expect(err).ShouldNot(HaveOccurred())

			rel, err := actionConfig.Releases.Last("ngrok")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(rel.Info.Status).To(Equal(helmrelease.StatusDeployed))
		})
	})
})
