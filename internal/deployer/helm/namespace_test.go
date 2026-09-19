package helm_test

import (
	"context"
	"io"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmkubefake "helm.sh/helm/v3/pkg/kube/fake"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	k8sv1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testNamespace = "namespace"

func namespaceObject(name string) *k8sv1.Namespace {
	return &k8sv1.Namespace{Name: name}
}

func failVerb(clientset *k8sfake.Clientset, verb string) {
	clientset.PrependReactor(
		verb,
		"namespaces",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errBoom
		},
	)
}

func namespaceExists(clientset *k8sfake.Clientset, name string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(
		context.Background(),
		name,
		k8smetav1.GetOptions{},
	)
	if err == nil {
		return true
	}

	Expect(k8s_errors.IsNotFound(err)).To(BeTrue())

	return false
}

var _ = Describe("namespace lifecycle", func() {
	var (
		subject         *helm.Helm
		actionConfig    *helmaction.Configuration
		clientset       *k8sfake.Clientset
		dryRun          bool
		removeNamespace bool
	)

	BeforeEach(func() {
		dryRun = false
		removeNamespace = false
		clientset = k8sfake.NewClientset()
		actionConfig = &helmaction.Configuration{
			Releases:   helmstorage.Init(helmdriver.NewMemory()),
			KubeClient: &helmkubefake.PrintingKubeClient{Out: io.Discard},
			Log:        func(string, ...any) {},
		}
	})

	JustBeforeEach(func() {
		subject = helm.New(helm.Options{
			K8sExt: config.XMiniEnvK8s{
				Context:                  "context",
				Namespace:                testNamespace,
				RemoveNamespaceOnDestroy: removeNamespace,
			},
			DryRun: dryRun,
		})
		subject.SetActionConfig(actionConfig)
		subject.SetKubernetesClientSet(clientset)
	})

	Describe("destroyNamespace", func() {
		Context("when the namespace exists", func() {
			BeforeEach(func() {
				clientset = k8sfake.NewClientset(namespaceObject(testNamespace))
			})

			It("deletes it", func() {
				Expect(subject.DestroyNamespace(context.Background())).To(Succeed())

				Expect(namespaceExists(clientset, testNamespace)).To(BeFalse())
			})

			Context("in dry-run mode", func() {
				BeforeEach(func() {
					dryRun = true
				})

				It("leaves it in place without calling the api", func() {
					Expect(subject.DestroyNamespace(context.Background())).To(Succeed())

					Expect(clientset.Actions()).To(BeEmpty())
					Expect(namespaceExists(clientset, testNamespace)).To(BeTrue())
				})
			})
		})

		It("succeeds when the namespace does not exist", func() {
			Expect(subject.DestroyNamespace(context.Background())).To(Succeed())
		})

		It("wraps a delete failure", func() {
			failVerb(clientset, "delete")

			err := subject.DestroyNamespace(context.Background())

			Expect(err).To(MatchError(helm.ErrK8sNamespace))
			Expect(err).To(MatchError(errBoom))
		})
	})

	Describe("createNamespaceIfNotExists", func() {
		It("creates the namespace when it is missing", func() {
			Expect(subject.CreateNamespaceIfNotExists(context.Background())).
				To(Succeed())

			Expect(namespaceExists(clientset, testNamespace)).To(BeTrue())
		})

		Context("when the namespace already exists", func() {
			BeforeEach(func() {
				clientset = k8sfake.NewClientset(namespaceObject(testNamespace))
			})

			It("issues no create", func() {
				Expect(subject.CreateNamespaceIfNotExists(context.Background())).
					To(Succeed())

				for _, action := range clientset.Actions() {
					Expect(action.GetVerb()).NotTo(Equal("create"))
				}
			})
		})

		Context("in dry-run mode", func() {
			BeforeEach(func() {
				dryRun = true
			})

			It("creates nothing without calling the api", func() {
				Expect(subject.CreateNamespaceIfNotExists(context.Background())).
					To(Succeed())

				Expect(clientset.Actions()).To(BeEmpty())
				Expect(namespaceExists(clientset, testNamespace)).To(BeFalse())
			})
		})

		It("wraps a lookup failure", func() {
			failVerb(clientset, "get")

			err := subject.CreateNamespaceIfNotExists(context.Background())

			Expect(err).To(MatchError(helm.ErrK8sNamespace))
			Expect(err).To(MatchError(errBoom))
		})

		It("wraps a create failure", func() {
			failVerb(clientset, "create")

			err := subject.CreateNamespaceIfNotExists(context.Background())

			Expect(err).To(MatchError(helm.ErrK8sNamespace))
			Expect(err).To(MatchError(errBoom))
		})
	})

	Describe("Destroy", func() {
		BeforeEach(func() {
			clientset = k8sfake.NewClientset(namespaceObject(testNamespace))
		})

		JustBeforeEach(func() {
			subject.SetProject(compose.Project{Name: "test-project"})
		})

		It("keeps the namespace by default", func() {
			Expect(subject.Destroy(context.Background())).To(Succeed())

			Expect(namespaceExists(clientset, testNamespace)).To(BeTrue())

			for _, action := range clientset.Actions() {
				Expect(action.GetVerb()).NotTo(Equal("delete"))
			}
		})

		Context("with removeNamespaceOnDestroy set", func() {
			BeforeEach(func() {
				removeNamespace = true
			})

			It("deletes the namespace", func() {
				Expect(subject.Destroy(context.Background())).To(Succeed())

				Expect(namespaceExists(clientset, testNamespace)).To(BeFalse())
			})

			It("propagates a delete failure", func() {
				failVerb(clientset, "delete")

				err := subject.Destroy(context.Background())

				Expect(err).To(MatchError(helm.ErrK8sNamespace))
			})

			Context("in dry-run mode", func() {
				BeforeEach(func() {
					dryRun = true
				})

				It("keeps the namespace", func() {
					Expect(subject.Destroy(context.Background())).To(Succeed())

					Expect(namespaceExists(clientset, testNamespace)).To(BeTrue())
				})
			})
		})
	})
})

var _ = Describe("dry-run without a cluster", func() {
	var subject *helm.Helm

	BeforeEach(func() {
		subject = helm.New(helm.Options{
			K8sExt: config.XMiniEnvK8s{
				Context:                  "context",
				Namespace:                testNamespace,
				RemoveNamespaceOnDestroy: true,
			},
			DryRun: true,
		})
	})

	It("deploys a service without touching helm", func() {
		svc := compose.Service{Name: "hello", Image: "reg/hello:v1"}
		project := compose.Project{
			Name:     "test-project",
			Services: types.Services{"hello": svc},
		}

		Expect(subject.InitProject(context.Background(), project, nil, nil)).
			To(Succeed())

		Expect(subject.DeployService(context.Background(), svc)).To(Succeed())
	})

	It("destroys a project without touching helm or the api", func() {
		subject.SetProject(compose.Project{Name: "test-project"})

		Expect(subject.Destroy(context.Background())).To(Succeed())
	})
})
