package helm

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	composegraph "github.com/compose-spec/compose-go/v2/graph"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/publishing"
	"github.com/rs/zerolog/log"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmcli "helm.sh/helm/v3/pkg/cli"
	k8sv1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// bounds concurrent helm releases; each one waits on rollout.
const maxDeployConcurrency = 5

const ngrokDeploymentTimeout = time.Minute

// Injected so the dependency-ordered walk is drivable without a cluster.
type deployFn func(ctx context.Context, svc config.ComposeService) error

type serviceTuple struct {
	compose   config.ComposeService
	extension config.XMiniEnvK8sService
}

type Options struct {
	K8sExt         config.XMiniEnvK8s
	ImageClient    image.Client
	GitClient      git.Client
	PublishClient  publishing.Client
	NgrokAuthToken string
	HelmDriver     string
	DryRun         bool
}

type Helm struct {
	k8sExt            config.XMiniEnvK8s
	project           config.ComposeProject
	services          map[string]serviceTuple
	servicesToPublish []config.NgrokEndpointConfig
	chartBuilder      *ChartBuilder
	actionConfig      *helmaction.Configuration
	imageClient       image.Client
	gitClient         git.Client
	publishClient     publishing.Client
	ngrokAuthToken    string
	helmDriver        string
	dryRun            bool
}

func New(opts Options) *Helm {
	return &Helm{
		k8sExt:         opts.K8sExt,
		actionConfig:   nil,
		imageClient:    opts.ImageClient,
		gitClient:      opts.GitClient,
		publishClient:  opts.PublishClient,
		ngrokAuthToken: opts.NgrokAuthToken,
		chartBuilder:   NewChartBuilder(opts.NgrokAuthToken),
		helmDriver:     opts.HelmDriver,
		dryRun:         opts.DryRun,
	}
}

func (h *Helm) String() string {
	return "Helm"
}

func (h *Helm) Init(
	ctx context.Context,
	project config.ComposeProject,
) error {
	if err := h.validateExtension(); err != nil {
		return err
	}

	if err := h.initProject(ctx, project); err != nil {
		return err
	}

	settings := helmcli.New()
	settings.SetNamespace(h.k8sExt.Namespace)
	settings.KubeContext = h.k8sExt.Context

	actionConfig := new(helmaction.Configuration)

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		settings.Namespace(),
		h.helmDriver,
		log.Printf,
	); err != nil {
		return errs.Errorf(
			ErrActionConfig,
			"failed to initialize helm action configuration: %w",
			err,
		)
	}

	h.actionConfig = actionConfig

	return nil
}

func (h *Helm) Deploy(ctx context.Context) error {
	if err := h.validateExtension(); err != nil {
		return err
	}

	if err := h.buildAndPushServiceImages(ctx); err != nil {
		return err
	}

	if err := h.createNamespaceIfNotExists(ctx); err != nil {
		return err
	}

	if err := h.deployInDependencyOrder(ctx, h.deployService); err != nil {
		return err
	}

	return h.installNgrokChart(ctx)
}

func (h *Helm) Destroy(ctx context.Context) error {
	if err := h.validateExtension(); err != nil {
		return err
	}

	destroyService := func(
		ctx context.Context,
		svc config.ComposeService,
	) error {
		return h.uninstallChart(ctx, svc.Name)
	}

	if err := h.destroyInReverseDependencyOrder(
		ctx,
		destroyService,
	); err != nil {
		return err
	}

	return h.uninstallNgrokChart(ctx)
}

// PublishedServiceUrls returns an empty map rather than a nil one when nothing
// is published: the caller cannot otherwise distinguish that from a lookup
// that never ran.
func (h *Helm) PublishedServiceUrls(
	ctx context.Context,
) (map[string]url.URL, error) {
	if len(h.servicesToPublish) == 0 {
		return map[string]url.URL{}, nil
	}

	return h.publishClient.ServiceUrls(
		ctx,
		deployer.PublishedNames(h.servicesToPublish),
	)
}

func (h *Helm) validateExtension() error {
	if h.k8sExt.Context == "" ||
		h.k8sExt.Namespace == "" {
		return errs.Errorf(
			ErrInvalidExtension,
			"missing one or both of required fields in k8s extension config: "+
				"[context, namespace]",
		)
	}

	return nil
}

// initProject is the half of Init that needs no cluster. Every extension is
// resolved before any is stored, so a bad one fails the whole project rather
// than one service midway.
func (h *Helm) initProject(
	ctx context.Context,
	project config.ComposeProject,
) error {
	services := map[string]serviceTuple{}
	servicesToPublish := []config.NgrokEndpointConfig{}

	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(
			ctx,
			h.k8sExtensionOptions(svc),
		)
		if err != nil {
			return err
		}

		services[svc.Name] = serviceTuple{compose: svc, extension: *svcExt}

		// Port is zero unless ngrok is configured and usable.
		if svcExt.Ngrok.Port != 0 {
			// Neither a skipped service nor a job has a k8s Service for the
			// endpoint's upstream to route to.
			switch {
			case svcExt.Skip:
				log.
					Warn().
					Str("service", svc.Name).
					Msg("detected skip: not publishing an ngrok endpoint")
			case svcExt.DeploymentType == config.K8sJobDeploymentType:
				log.
					Warn().
					Str("service", svc.Name).
					Msg("jobs serve no traffic: not publishing an ngrok endpoint")
			default:
				servicesToPublish = append(servicesToPublish, config.NgrokEndpointConfig{
					Namespace:     h.k8sExt.Namespace,
					EndpointName:  fmt.Sprintf("%s-%s", h.k8sExt.Namespace, svc.Name),
					ServiceName:   svc.Name,
					URL:           svcExt.Ngrok.URL,
					Port:          svcExt.Ngrok.Port,
					TrafficPolicy: svcExt.Ngrok.TrafficPolicy,
				})
			}
		}
	}

	// project.Services is a map. An unstable order changes the ConfigMap
	// checksum, which reassigns every unreserved URL on an unchanged deploy.
	slices.SortFunc(servicesToPublish, func(a, b config.NgrokEndpointConfig) int {
		return strings.Compare(a.ServiceName, b.ServiceName)
	})

	h.services = services
	h.servicesToPublish = servicesToPublish
	h.project = project

	return nil
}

func (h *Helm) k8sExtensionOptions(
	svc config.ComposeService,
) config.XMiniEnvK8sServiceOptions {
	return config.XMiniEnvK8sServiceOptions{
		K8sExt:       h.k8sExt,
		Service:      svc,
		GitClient:    h.gitClient,
		NgrokEnabled: h.ngrokAuthToken != "",
	}
}

func (h *Helm) deployService(
	ctx context.Context,
	svc config.ComposeService,
) error {
	internalService, ok := h.services[svc.Name]
	if !ok {
		return errs.Errorf(
			ErrMissingService,
			"service not in map: %s",
			svc.Name,
		)
	}

	if internalService.extension.Skip {
		log.
			Warn().
			Str("service", svc.Name).
			Msg("detected skip: omitting service from deployment")

		return nil
	}

	values, err := internalService.extension.ToChartValuesMap()
	if err != nil {
		return err
	}

	timeout := internalService.extension.DeploymentTimeout
	if timeout == "" {
		timeout = config.HelmDefaultDeploymentTimeout
	}

	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return errs.Errorf(
			ErrChartDeploymentTimeout,
			"invalid deploymentTimeout configuration: %w",
			err,
		)
	}

	chart, err := h.chartBuilder.Build(
		svc.Name,
		internalService.extension.DeploymentType,
		internalService.extension.Manifests,
	)
	if err != nil {
		return err
	}

	return h.upgradeOrInstallChart(
		ctx,
		chart,
		values,
		parsedTimeout,
		internalService.extension.Recreate,
	)
}

func (h *Helm) buildAndPushServiceImages(ctx context.Context) error {
	dockerServices := []image.ServiceProperties{}

	for name, svc := range h.services {
		if svc.extension.Skip {
			log.Warn().Str("service", name).Msg("detected skip: skipping")

			continue
		}

		if svc.compose.Build != nil {
			dockerServices = append(dockerServices, image.NewServiceProperties(
				svc.compose,
				svc.extension.Image.ServiceImage,
			))
		}
	}

	if len(dockerServices) > 0 {
		return h.imageClient.BuildAndPush(ctx, dockerServices)
	}

	return nil
}

func (h *Helm) deployInDependencyOrder(
	ctx context.Context,
	deploy deployFn,
) error {
	if err := h.checkDependencyGraph(); err != nil {
		return err
	}

	if err := composegraph.InDependencyOrder(
		ctx,
		&h.project,
		func(ctx context.Context, _ string, svc config.ComposeService) error {
			return deploy(ctx, svc)
		},
		composegraph.WithMaxConcurrency(maxDeployConcurrency),
	); err != nil {
		return errs.Errorf(
			ErrComposeDependencyGraph,
			"failed to deploy in dependency order: %w",
			err,
		)
	}

	return nil
}

func (h *Helm) destroyInReverseDependencyOrder(
	ctx context.Context,
	uninstall deployFn,
) error {
	if err := h.checkDependencyGraph(); err != nil {
		return err
	}

	if err := composegraph.InDependencyOrder(
		ctx,
		&h.project,
		func(ctx context.Context, _ string, svc config.ComposeService) error {
			return uninstall(ctx, svc)
		},
		composegraph.InReverseOrder,
		composegraph.WithMaxConcurrency(maxDeployConcurrency),
	); err != nil {
		return errs.Errorf(
			ErrComposeDependencyGraph,
			"failed to destroy in reverse dependency order: %w",
			err,
		)
	}

	return nil
}

func (h *Helm) checkDependencyGraph() error {
	if err := composegraph.CheckCycle(&h.project); err != nil {
		return errs.Errorf(
			ErrComposeDependencyGraph,
			"invalid service dependency graph: %w",
			err,
		)
	}

	return nil
}

func (h *Helm) upgradeOrInstallChart(
	ctx context.Context,
	chart *helmchart.Chart,
	values map[string]any,
	timeout time.Duration,
	restart bool,
) error {
	if h.serviceReleaseExists(chart.Name()) {
		log.Info().Str("release", chart.Name()).Msg("upgrading release")

		return h.upgradeChart(ctx, chart, values, timeout, restart)
	}

	log.Info().Str("release", chart.Name()).Msg("installing release")

	return h.installChart(ctx, chart, values, timeout)
}

func (h *Helm) installChart(
	ctx context.Context,
	chart *helmchart.Chart,
	values map[string]any,
	timeout time.Duration,
) error {
	client := helmaction.NewInstall(h.actionConfig)
	client.ReleaseName = chart.Name()
	client.Namespace = h.k8sExt.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.WaitForJobs = true
	client.DryRun = h.dryRun
	client.Timeout = timeout

	if _, err := client.RunWithContext(ctx, chart, values); err != nil {
		return errs.Errorf(
			ErrChartInstall,
			"failed to install service chart %s: %w",
			chart.Name(),
			err,
		)
	}

	log.
		Info().
		Str("context", h.k8sExt.Context).
		Str("namespace", h.k8sExt.Namespace).
		Str("service", chart.Name()).
		Msg("successfully installed service chart")

	return nil
}

func (h *Helm) upgradeChart(
	ctx context.Context,
	chart *helmchart.Chart,
	values map[string]any,
	timeout time.Duration,
	restart bool,
) error {
	client := helmaction.NewUpgrade(h.actionConfig)
	client.Namespace = h.k8sExt.Namespace
	client.Atomic = true
	client.Wait = true
	client.WaitForJobs = true
	client.CleanupOnFail = true
	client.DryRun = h.dryRun
	client.Timeout = timeout

	if restart {
		client.Force = true
		client.Install = true
		client.Recreate = true
	}

	if _, err := client.RunWithContext(
		ctx,
		chart.Name(),
		chart,
		values,
	); err != nil {
		return errs.Errorf(
			ErrChartUpgrade,
			"failed to upgrade service chart %s: %w",
			chart.Name(),
			err,
		)
	}

	log.
		Info().
		Str("context", h.k8sExt.Context).
		Str("namespace", h.k8sExt.Namespace).
		Str("service", chart.Name()).
		Msg("successfully upgraded service chart")

	return nil
}

func (h *Helm) uninstallChart(_ context.Context, svcName string) error {
	client := helmaction.NewUninstall(h.actionConfig)
	client.Wait = true
	client.IgnoreNotFound = true
	client.DryRun = h.dryRun

	log.Info().Str("chart", svcName).Msg("uninstalling chart")

	response, err := client.Run(svcName)
	if err != nil {
		return errs.Errorf(
			ErrChartUninstall,
			"failed to uninstall service chart %s: %w",
			svcName,
			err,
		)
	}

	if response == nil || response.Release == nil {
		log.Info().Str("service", svcName).Msg("no release found for service")

		return nil
	}

	log.
		Info().
		Str("context", h.k8sExt.Context).
		Str("service", response.Release.Name).
		Str("namespace", response.Release.Namespace).
		Msg("successfully uninstalled service")

	return nil
}

func (h *Helm) createNamespaceIfNotExists(ctx context.Context) error {
	if h.dryRun {
		log.Warn().
			Str("namespace", h.k8sExt.Namespace).
			Msg("dry-run mode: skipping namespace creation")

		return nil
	}

	clientset, err := h.actionConfig.KubernetesClientSet()
	if err != nil {
		return errs.Errorf(
			ErrK8sNamespace,
			"failed to build kubernetes client: %w",
			err,
		)
	}

	namespaces := clientset.CoreV1().Namespaces()

	_, err = namespaces.Get(ctx, h.k8sExt.Namespace, k8smetav1.GetOptions{})

	switch {
	case err == nil:
		return nil
	case !k8s_errors.IsNotFound(err):
		return errs.Errorf(
			ErrK8sNamespace,
			"failed to look up namespace %s: %w",
			h.k8sExt.Namespace,
			err,
		)
	}

	if _, err := namespaces.Create(
		ctx,
		&k8sv1.Namespace{Name: h.k8sExt.Namespace},
		k8smetav1.CreateOptions{},
	); err != nil {
		return errs.Errorf(
			ErrK8sNamespace,
			"failed to create namespace %s: %w",
			h.k8sExt.Namespace,
			err,
		)
	}

	return nil
}

// Deleting the last ngrok block has to uninstall the release, or it keeps
// serving endpoints the project no longer declares.
func (h *Helm) installNgrokChart(ctx context.Context) error {
	if len(h.servicesToPublish) == 0 {
		return h.uninstallNgrokChart(ctx)
	}

	chart, err := h.chartBuilder.BuildNgrok()
	if err != nil {
		return err
	}

	// restart=false: the checksum annotations decide when the pod rolls.
	// Forcing it would reassign every URL without a reserved domain.
	return h.upgradeOrInstallChart(
		ctx,
		chart,
		h.chartBuilder.NgrokValues(h.servicesToPublish),
		ngrokDeploymentTimeout,
		false,
	)
}

// Unconditional: the current ngrok config may no longer mention what was
// installed, and uninstallChart tolerates a missing release.
func (h *Helm) uninstallNgrokChart(ctx context.Context) error {
	return h.uninstallChart(ctx, ngrokReleaseName)
}

func (h *Helm) serviceReleaseExists(name string) bool {
	client := helmaction.NewGet(h.actionConfig)
	release, _ := client.Run(name)

	return release != nil
}
