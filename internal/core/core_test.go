package core_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/core"
	deployermocks "github.com/robgonnella/minienv/internal/deployer/mocks"
)

// The deployer failure the fakes return. Specs assert identity against this
// value rather than its text, which also proves Core preserved the cause.
var errBoom = errors.New("boom")

var _ = Describe("Core", func() {
	var (
		mockDeployer *deployermocks.MockDeployer
		project      *config.ComposeProject
		ext          *config.XMiniEnv
		subject      *core.Core
	)

	BeforeEach(func() {
		mockDeployer = deployermocks.NewMockDeployer(GinkgoT())
		project = &config.ComposeProject{Name: "test-project"}
		ext = &config.XMiniEnv{
			K8s: config.XMiniEnvK8s{
				Context:   "context",
				Namespace: "namespace",
			},
		}
		subject = core.New(ext, project, mockDeployer, false)

		// Every Deploy/Destroy path logs the deployer name at least once.
		mockDeployer.EXPECT().String().Return("MockDeployer").Maybe()
	})

	Describe("Deploy", func() {
		It("initializes the deployer then deploys the loaded project", func() {
			initCall := mockDeployer.EXPECT().Init(project).Return(nil).Once()
			mockDeployer.
				EXPECT().
				Deploy().
				Return(nil).
				Once().
				NotBefore(initCall)

			Expect(subject.Deploy()).To(Succeed())
		})

		It("returns a core error and never deploys when Init fails", func() {
			mockDeployer.
				EXPECT().
				Init(project).
				Return(errBoom).
				Once()

			err := subject.Deploy()
			Expect(err).To(MatchError(core.KindDeployerInit))
			Expect(err).To(MatchError(errBoom))

			mockDeployer.AssertNotCalled(GinkgoT(), "Deploy")
		})

		It("returns a core error when the deploy itself fails", func() {
			mockDeployer.EXPECT().Init(project).Return(nil).Once()
			mockDeployer.
				EXPECT().
				Deploy().
				Return(errBoom).
				Once()

			err := subject.Deploy()
			Expect(err).To(MatchError(core.KindDeploy))
			Expect(err).To(MatchError(errBoom))
		})
	})

	Describe("Destroy", func() {
		It("initializes the deployer then destroys the loaded project", func() {
			initCall := mockDeployer.EXPECT().Init(project).Return(nil).Once()
			mockDeployer.
				EXPECT().
				Destroy().
				Return(nil).
				Once().
				NotBefore(initCall)

			Expect(subject.Destroy()).To(Succeed())
		})

		It("returns a core error and never destroys when Init fails", func() {
			mockDeployer.
				EXPECT().
				Init(project).
				Return(errBoom).
				Once()

			err := subject.Destroy()
			Expect(err).To(MatchError(core.KindDeployerInit))
			Expect(err).To(MatchError(errBoom))

			mockDeployer.AssertNotCalled(GinkgoT(), "Destroy")
		})

		It("returns a core error when the destroy itself fails", func() {
			mockDeployer.EXPECT().Init(project).Return(nil).Once()
			mockDeployer.
				EXPECT().
				Destroy().
				Return(errBoom).
				Once()

			err := subject.Destroy()
			Expect(err).To(MatchError(core.KindDestroy))
			Expect(err).To(MatchError(errBoom))
		})
	})
})
