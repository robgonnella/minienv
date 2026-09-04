package deployer

import (
	"context"
	"time"

	composegraph "github.com/compose-spec/compose-go/v2/graph"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/rs/zerolog/log"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
	helmcli "helm.sh/helm/v3/pkg/cli"
	k8sv1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// bounds concurrent helm releases; each one waits on rollout
const maxDeployConcurrency = 5

// deployFn acts on one service. Injected so the dependency-ordered walk is
// drivable without a cluster.
type deployFn func(ctx context.Context, svc config.ComposeService) error

type serviceTuple struct {
	compose   config.ComposeService
	extension config.XMiniEnvK8sService
}

type HelmOptions struct {
	Ext            *config.XMiniEnv
	ImageClient    image.Client
	GitClient      git.Client
	NgrokAuthToken string
	HelmDriver     string
	DryRun         bool
}

type Helm struct {
	ext            *config.XMiniEnv
	project        *config.ComposeProject
	services       map[string]serviceTuple
	actionConfig   *helmaction.Configuration
	imageClient    image.Client
	gitClient      git.Client
	ngrokAuthToken string
	helmDriver     string
	dryRun         bool
}

func NewHelm(opts HelmOptions) *Helm {
	return &Helm{
		ext:            opts.Ext,
		actionConfig:   nil,
		imageClient:    opts.ImageClient,
		gitClient:      opts.GitClient,
		ngrokAuthToken: opts.NgrokAuthToken,
		helmDriver:     opts.HelmDriver,
		dryRun:         opts.DryRun,
	}
}

func (h *Helm) Active() bool {
	return h.ext.K8s.Context != "" && h.ext.K8s.Namespace != ""
}

func (h *Helm) ConfigField() string {
	return "k8s"
}

func (h *Helm) String() string {
	return "Helm"
}

func (h *Helm) Init(project *config.ComposeProject) error {
	if !h.Active() {
		return nil
	}

	if err := h.initProject(project); err != nil {
		return err
	}

	settings := helmcli.New()
	settings.SetNamespace(h.ext.K8s.Namespace)
	settings.KubeContext = h.ext.K8s.Context

	actionConfig := new(helmaction.Configuration)

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		settings.Namespace(),
		h.helmDriver,
		log.Printf,
	); err != nil {
		return err
	}

	h.actionConfig = actionConfig
	return nil
}

func (h *Helm) Deploy() error {
	if !h.Active() {
		return nil
	}

	if err := h.buildAndPushServiceImages(); err != nil {
		return err
	}

	if err := h.createNamespaceIfNotExists(); err != nil {
		return err
	}

	return h.deployInDependencyOrder(h.deployService)
}

func (h *Helm) Destroy() error {
	if !h.Active() {
		return nil
	}

	if err := h.destroyInReverseDependencyOrder(
		h.uninstallChart,
	); err != nil {
		return err
	}

	return nil
}

// initProject is the half of Init that needs no cluster: it resolves every
// service extension once, up front, so a bad extension fails before a single
// release is touched, and so a +git tag costs one git call per run rather than
// one per service per pass. Everything downstream reads h.services, which makes
// Init a hard precondition of Deploy and Destroy.
func (h *Helm) initProject(project *config.ComposeProject) error {
	services := map[string]serviceTuple{}

	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(h.extensionOptions(svc))
		if err != nil {
			return err
		}
		services[svc.Name] = serviceTuple{compose: svc, extension: *svcExt}
	}

	h.services = services
	h.project = project

	return nil
}

func (h *Helm) extensionOptions(
	svc config.ComposeService,
) config.XMiniEnvK8sServiceOptions {
	return config.XMiniEnvK8sServiceOptions{
		MainExt:      h.ext,
		Service:      svc,
		GitClient:    h.gitClient,
		NgrokEnabled: h.ngrokAuthToken != "",
	}
}

func (h *Helm) deployService(ctx context.Context, svc config.ComposeService) error {
	internalService, ok := h.services[svc.Name]
	if !ok {
		return errs.Errorf(
			KindHelmMissingService,
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

	return h.upgradeOrInstallService(ctx, &internalService.extension, svc)
}

func (h *Helm) buildAndPushServiceImages() error {
	dockerServices := []image.ServiceProperties{}
	for name, svc := range h.services {
		if svc.extension.Skip {
			log.Warn().Str("service", name).Msg("detected skip: skipping")
			continue
		}

		platforms := []string{"linux/amd64"}

		if len(svc.extension.Image.Platforms) != 0 {
			platforms = svc.extension.Image.Platforms
		}

		if svc.compose.Build != nil {
			dockerServices = append(dockerServices, image.ServiceProperties{
				Name:       name,
				Registry:   svc.extension.Image.Repository,
				Tag:        svc.extension.Image.Tag,
				Context:    svc.compose.Build.Context,
				Dockerfile: svc.compose.Build.Dockerfile,
				Platforms:  platforms,
				Args:       svc.compose.Build.Args.ToMapping(),
			})
		}
	}

	if len(dockerServices) > 0 {
		return h.imageClient.BuildAndPush(dockerServices)
	}

	return nil
}

// deployInDependencyOrder walks the project's depends_on graph, deploying a
// service only once everything it depends on has been deployed. Services with
// no unmet dependency are deployed concurrently.
func (h *Helm) deployInDependencyOrder(
	deploy deployFn,
) error {
	if err := h.checkDependencyGraph(); err != nil {
		return err
	}

	return composegraph.InDependencyOrder(
		context.Background(),
		h.project,
		func(ctx context.Context, _ string, svc config.ComposeService) error {
			return deploy(ctx, svc)
		},
		composegraph.WithMaxConcurrency(maxDeployConcurrency),
	)
}

// destroyInReverseDependencyOrder mirrors deployInDependencyOrder: a service is
// uninstalled only once everything depending on it is gone.
func (h *Helm) destroyInReverseDependencyOrder(
	uninstall deployFn,
) error {
	if err := h.checkDependencyGraph(); err != nil {
		return err
	}

	return composegraph.InDependencyOrder(
		context.Background(),
		h.project,
		func(ctx context.Context, _ string, svc config.ComposeService) error {
			return uninstall(ctx, svc)
		},
		composegraph.InReverseOrder,
		composegraph.WithMaxConcurrency(maxDeployConcurrency),
	)
}

// Validated up front so an unusable depends_on graph — a cycle, or a required
// dependency naming a service the project does not define — fails before a
// single release is touched. It also keeps the traversal's own error return
// carrying nothing but failures from the injected step.
func (h *Helm) checkDependencyGraph() error {
	if err := composegraph.CheckCycle(h.project); err != nil {
		return errs.Errorf(
			KindComposeDependencyGraph,
			"invalid service dependency graph: %w",
			err,
		)
	}

	return nil
}

func (h *Helm) upgradeOrInstallService(
	ctx context.Context,
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	exists := h.serviceReleaseExists(svc.Name)

	if exists {
		log.Info().Str("release", svc.Name).Msg("upgrading release")
		return h.upgradeChart(ctx, svcExt, svc)
	} else {
		log.Info().Str("release", svc.Name).Msg("installing release")
		return h.installChart(ctx, svcExt, svc)
	}
}

func (h *Helm) installChart(
	ctx context.Context,
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	values, err := svcExt.ToChartValuesMap()
	if err != nil {
		return err
	}

	timeout := svcExt.DeploymentTimeout
	if timeout == "" {
		timeout = config.HELM_DEFAULT_DEPLOYMENT_TIMEOUT
	}

	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return errs.Errorf(
			KindChartDeploymentTimeout,
			"invalid deploymentTimeout configuration: %w",
			err,
		)
	}

	client := helmaction.NewInstall(h.actionConfig)
	client.ReleaseName = svc.Name
	client.Namespace = h.ext.K8s.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.Wait = true
	client.WaitForJobs = true
	client.DryRun = h.dryRun
	client.Timeout = parsedTimeout

	chart, err := h.loadChart(svc.Name, svcExt.DeploymentType)
	if err != nil {
		return errs.Errorf(KindChartLoad, "failed to load in-memory chart: %w", err)
	}

	log.Info().Fields(values).Str("chart", svc.Name).Msg("installing chart")

	if _, err := client.RunWithContext(ctx, chart, values); err != nil {
		return errs.Errorf(
			KindChartInstall,
			"failed to install service chart %s: %w",
			svc.Name,
			err,
		)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully installed service chart")

	return nil
}

func (h *Helm) upgradeChart(
	ctx context.Context,
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	values, err := svcExt.ToChartValuesMap()
	if err != nil {
		return err
	}

	timeout := svcExt.DeploymentTimeout
	if timeout == "" {
		timeout = config.HELM_DEFAULT_DEPLOYMENT_TIMEOUT
	}

	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return errs.Errorf(
			KindChartDeploymentTimeout,
			"invalid deploymentTimeout configuration: %w",
			err,
		)
	}

	client := helmaction.NewUpgrade(h.actionConfig)
	client.Namespace = h.ext.K8s.Namespace
	client.Atomic = true
	client.Wait = true
	client.WaitForJobs = true
	client.CleanupOnFail = true
	client.Force = true
	client.Install = true
	client.Recreate = true
	client.ResetValues = true
	client.DryRun = h.dryRun
	client.Timeout = parsedTimeout

	chart, err := h.loadChart(svc.Name, svcExt.DeploymentType)
	if err != nil {
		return errs.Errorf(
			KindChartLoad,
			"failed to load in-memory chart: %w",
			err,
		)
	}

	log.Info().Fields(values).Str("chart", svc.Name).Msg("upgrading chart")

	if _, err := client.RunWithContext(ctx, svc.Name, chart, values); err != nil {
		return errs.Errorf(
			KindChartUpgrade,
			"failed to upgrade service chart %s: %w",
			svc.Name,
			err,
		)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully upgraded service chart")

	return nil
}

func (h *Helm) uninstallChart(_ context.Context, svc config.ComposeService) error {
	client := helmaction.NewUninstall(h.actionConfig)
	client.Wait = true
	client.IgnoreNotFound = true
	client.DryRun = h.dryRun

	log.Info().Str("chart", svc.Name).Msg("uninstalling chart")

	response, err := client.Run(svc.Name)
	if err != nil {
		return errs.Errorf(
			KindChartUninstall,
			"failed to uninstall service chart %s: %w",
			svc.Name,
			err,
		)
	}

	if response == nil || response.Release == nil {
		log.Info().Str("service", svc.Name).Msg("no release found for service")
		return nil
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("service", response.Release.Name).
		Str("namespace", response.Release.Namespace).
		Msg("successfully uninstalled service")

	return nil
}

func (h *Helm) createNamespaceIfNotExists() error {
	clientset, err := h.actionConfig.KubernetesClientSet()
	if err != nil {
		return err
	}

	create := false

	_, err = clientset.
		CoreV1().
		Namespaces().
		Get(context.TODO(), h.ext.K8s.Namespace, k8smetav1.GetOptions{})

	if err != nil && k8s_errors.IsNotFound(err) {
		create = true
	} else if err != nil {
		return err
	}

	if !create {
		return nil
	}

	_, err = clientset.
		CoreV1().
		Namespaces().
		Create(
			context.TODO(),
			&k8sv1.Namespace{Name: h.ext.K8s.Namespace},
			k8smetav1.CreateOptions{},
		)

	return err
}

func (h *Helm) serviceReleaseExists(name string) bool {
	client := helmaction.NewGet(h.actionConfig)
	release, _ := client.Run(name)
	return release != nil
}

func (h *Helm) loadChart(
	svcName string,
	deploymentType config.K8sDeploymentType,
) (*helmchart.Chart, error) {
	name := svcName
	if deploymentType == config.K8sJobDeploymentType {
		id, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 8)
		if err != nil {
			return nil, errs.Errorf(
				KindChartLoad,
				"failed to generate uid for job: %s",
				svcName,
			)
		}
		name = id
	}

	files := []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(helmChartYamlTmpl(name)),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(HELM_HELPERS_TMPL),
		},
	}

	if deploymentType == config.K8sJobDeploymentType {
		files = append(files, h.getJobFiles()...)
	} else {
		files = append(files, h.getServiceFiles()...)
	}

	return helmloader.LoadFiles(files)
}

func (h *Helm) getServiceFiles() []*helmloader.BufferedFile {
	return []*helmloader.BufferedFile{
		{
			Name: "values.yaml",
			Data: []byte(HELM_DEPLOYMENT_VALUES_TMPL),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(HELM_DEPLOYMENT_TMPL),
		},
		{
			Name: "templates/service.yaml",
			Data: []byte(HELM_SERVICE_TMPL),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(HELM_SERVICE_ACCOUNT_TMPL),
		},
		{
			Name: "templates/configmap.yaml",
			Data: []byte(HELM_NGROK_CONFIG_MAP_TMPL),
		},
		{
			Name: "templates/secret.yaml",
			Data: []byte(helmNgrokSecretTmpl(h.ngrokAuthToken)),
		},
	}
}

func (h *Helm) getJobFiles() []*helmloader.BufferedFile {
	return []*helmloader.BufferedFile{
		{
			Name: "values.yaml",
			Data: []byte(HELM_JOB_VALUES_TMPL),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(HELM_SERVICE_ACCOUNT_TMPL),
		},
		{
			Name: "templates/job.yaml",
			Data: []byte(HELM_JOB_TMPL),
		},
	}
}
