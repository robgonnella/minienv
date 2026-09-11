package helm_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	gitmocks "github.com/robgonnella/minienv/internal/git/mocks"
	imagemocks "github.com/robgonnella/minienv/internal/image/mocks"
)

// The per-service failure the injected step returns, asserted by identity for
// the same reason.
var errStep = errors.New("step blew up")

// dependent builds a bare service that depends_on each named service. Only the
// name and depends_on reach the dependency walk. Required matters: compose-go
// silently drops a dependency that is both unknown and not required, so an
// optional one would not exercise the unknown-dependency branch.
func dependent(name string, deps ...string) config.ComposeService {
	dependsOn := types.DependsOnConfig{}

	for _, dep := range deps {
		dependsOn[dep] = types.ServiceDependency{
			Condition: types.ServiceConditionStarted,
			Required:  true,
		}
	}

	return config.ComposeService{Name: name, DependsOn: dependsOn}
}

// recorder logs the order the walk visited services in. The walk runs services
// concurrently and the suite runs with -race, so the log needs a lock: an
// unguarded append here would be a data race, not an occasional flake.
type recorder struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	ok     []string
	fail   map[string]error
}

func newRecorder(cancel context.CancelFunc) *recorder {
	return &recorder{
		mu:     sync.Mutex{},
		cancel: cancel,
		ok:     []string{},
		fail:   map[string]error{},
	}
}

func (r *recorder) visit(_ context.Context, svc config.ComposeService) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err, ok := r.fail[svc.Name]; ok {
		r.cancel()
		return err
	}

	r.ok = append(r.ok, svc.Name)

	return nil
}

func (r *recorder) succeeded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.ok)
}

var _ = Describe("Helm", func() {
	var (
		k8sExt    config.XMiniEnvK8s
		mockImage *imagemocks.MockClient
		mockGit   *gitmocks.MockClient
		subject   *helm.Helm
	)

	BeforeEach(func() {
		k8sExt = config.XMiniEnvK8s{
			Context:   "context",
			Namespace: "namespace",
		}
		mockImage = imagemocks.NewMockClient(GinkgoT())
		mockGit = gitmocks.NewMockClient(GinkgoT())
	})

	JustBeforeEach(func() {
		subject = helm.New(helm.Options{
			K8sExt:         k8sExt,
			ImageClient:    mockImage,
			GitClient:      mockGit,
			NgrokAuthToken: "",
			DryRun:         false,
		})
	})

	Describe("deployInDependencyOrder", func() {
		var (
			project config.ComposeProject
			rec     *recorder
		)

		BeforeEach(func() {
			_, cancel := context.WithCancel(context.Background())
			project = config.ComposeProject{Name: "test-project"}
			rec = newRecorder(cancel)
		})

		It("deploys a chain dependencies first", func() {
			project.Services = types.Services{
				"api":   dependent("api", "cache"),
				"cache": dependent("cache", "db"),
				"db":    dependent("db"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(context.Background(), rec.visit)).
				To(Succeed())

			// The only fully determined order in the suite, so assert it exactly.
			Expect(rec.succeeded()).To(Equal([]string{"db", "cache", "api"}))
		})

		It("deploys a diamond by position, not by sequence", func() {
			project.Services = types.Services{
				"db":     dependent("db"),
				"api":    dependent("api", "db"),
				"worker": dependent("worker", "db"),
				"ui":     dependent("ui", "api", "worker"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(context.Background(), rec.visit)).
				To(Succeed())

			// api and worker are free to run concurrently, so only their position
			// relative to db and ui is guaranteed.
			order := rec.succeeded()
			Expect(order).To(HaveLen(4))
			Expect(order[0]).To(Equal("db"))
			Expect(order[1:3]).To(ConsistOf("api", "worker"))
			Expect(order[3]).To(Equal("ui"))
		})

		It("deploys every unrelated service exactly once", func() {
			project.Services = types.Services{
				"one":   dependent("one"),
				"two":   dependent("two"),
				"three": dependent("three"),
			}
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(context.Background(), rec.visit)).
				To(Succeed())

			Expect(rec.succeeded()).To(ConsistOf("one", "two", "three"))
		})

		// Both fakes have to be in flight at once for either to return, so a
		// serialized walk cannot satisfy this spec. The ctx guard is what makes
		// that show up as a timeout failure rather than a hung suite.
		It("deploys unrelated services concurrently", func(specCtx SpecContext) {
			project.Services = types.Services{
				"one": dependent("one"),
				"two": dependent("two"),
			}
			subject.SetProject(project)

			var (
				mu       sync.Mutex
				inFlight int
				bothIn   = make(chan struct{})
			)

			deploy := func(ctx context.Context, _ config.ComposeService) error {
				mu.Lock()

				inFlight++
				if inFlight == 2 {
					close(bothIn)
				}
				mu.Unlock()

				select {
				case <-bothIn:
					return nil
				case <-specCtx.Done():
					return ctx.Err()
				}
			}

			Expect(subject.DeployInDependencyOrder(context.Background(), deploy)).To(Succeed())
		}, SpecTimeout(10*time.Second))

		It("propagates a failure from the last service in a chain", func() {
			project.Services = types.Services{
				"api":   dependent("api", "cache"),
				"cache": dependent("cache", "db"),
				"db":    dependent("db"),
			}
			subject.SetProject(project)

			rec.fail["api"] = errStep

			err := subject.DeployInDependencyOrder(context.Background(), rec.visit)

			Expect(err).To(MatchError(errStep))
			Expect(rec.succeeded()).To(Equal([]string{"db", "cache"}))
		})

		It("propagates a failure from a dependency", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db"),
			}
			subject.SetProject(project)

			rec.fail["db"] = errStep

			err := subject.DeployInDependencyOrder(context.Background(), rec.visit)

			// Only the propagation is asserted. Whether api is dispatched before
			// the group's context cancellation lands is a race inside compose-go's
			// traversal, so asserting api was skipped would be asserting a
			// guarantee the walk does not make.
			Expect(err).To(MatchError(errStep))
			Expect(rec.succeeded()).NotTo(ContainElement("db"))
		})

		It("rejects a dependency cycle before deploying anything", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db", "api"),
			}
			subject.SetProject(project)

			err := subject.DeployInDependencyOrder(context.Background(), rec.visit)

			Expect(err).To(MatchError(helm.ErrComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})

		It("rejects a dependency naming an undefined service", func() {
			project.Services = types.Services{
				"api": dependent("api", "missing"),
			}
			subject.SetProject(project)

			err := subject.DeployInDependencyOrder(context.Background(), rec.visit)

			Expect(err).To(MatchError(helm.ErrComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})

		It("deploys nothing for an empty project", func() {
			subject.SetProject(project)

			Expect(subject.DeployInDependencyOrder(context.Background(), rec.visit)).
				To(Succeed())

			Expect(rec.succeeded()).To(BeEmpty())
		})
	})

	Describe("destroyInReverseDependencyOrder", func() {
		var (
			project config.ComposeProject
			rec     *recorder
		)

		BeforeEach(func() {
			_, cancel := context.WithCancel(context.Background())
			rec = newRecorder(cancel)
			project = config.ComposeProject{
				Name: "test-project",
				Services: types.Services{
					"api":   dependent("api", "cache"),
					"cache": dependent("cache", "db"),
					"db":    dependent("db"),
				},
			}
		})

		It("destroys a chain dependents first", func() {
			subject.SetProject(project)
			Expect(subject.DestroyInReverseDependencyOrder(context.Background(), rec.visit)).
				To(Succeed())

			// The exact inverse of the deploy walk: nothing is uninstalled while
			// something still depends on it.
			Expect(rec.succeeded()).To(Equal([]string{"api", "cache", "db"}))
		})

		It("propagates a failure from service that failed to be destroyed", func() {
			subject.SetProject(project)

			rec.fail["api"] = errStep
			err := subject.DestroyInReverseDependencyOrder(context.Background(), rec.visit)
			Expect(err).To(MatchError(errStep))
		})

		It("rejects a dependency cycle before destroying anything", func() {
			project.Services = types.Services{
				"api": dependent("api", "db"),
				"db":  dependent("db", "api"),
			}
			subject.SetProject(project)

			err := subject.DestroyInReverseDependencyOrder(context.Background(), rec.visit)

			Expect(err).To(MatchError(helm.ErrComposeDependencyGraph))
			Expect(rec.succeeded()).To(BeEmpty())
		})
	})

	// The step the walk dispatches to. Only the branches that return before a
	// helm client is constructed are reachable without a cluster.
	Describe("deployService", func() {
		It("omits a service flagged skip", func() {
			ctx := context.Background()
			svc := config.ComposeService{Name: "hello", Image: "reg/hello:v1"}
			svc.Extensions = types.Extensions{
				config.K8sServiceExtension: map[string]any{"skip": true},
			}

			Expect(subject.InitProject(context.Background(), config.ComposeProject{
				Name:     "test-project",
				Services: types.Services{"hello": svc},
			})).To(Succeed())

			// mockImage and mockGit carry no EXPECT(), so reaching any client
			// fails this spec on cleanup — as would reaching helm, since Init was
			// never called and the action config is still nil.
			Expect(subject.DeployService(ctx, svc)).To(Succeed())
		})

		// The step reads the map initProject builds rather than resolving the
		// extension itself, so a service the map does not know about is the one
		// failure it still owns — and it means Init was skipped, or the project
		// changed underneath it.
		It("errors for a service that was never resolved by Init", func() {
			ctx := context.Background()

			Expect(subject.InitProject(context.Background(), config.ComposeProject{
				Name: "test-project",
			})).To(Succeed())

			err := subject.DeployService(ctx, config.ComposeService{Name: "hello"})

			Expect(err).To(MatchError(helm.ErrMissingService))
		})
	})
})
