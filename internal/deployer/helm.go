package deployer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	composegraph "github.com/compose-spec/compose-go/v2/graph"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/publishing"
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

// Fixed so Destroy can find the release without rebuilding the ngrok config.
const ngrokReleaseName = "ngrok"

const ngrokDeploymentTimeout = time.Minute

// deployFn acts on one service. Injected so the dependency-ordered walk is
// drivable without a cluster.
type deployFn func(ctx context.Context, svc config.ComposeService) error

type serviceTuple struct {
	compose   config.ComposeService
	extension config.XMiniEnvK8sService
}

type NgrokConfig struct {
	Namespace     string `mapstructure:"namespace"`
	EndpointName  string `mapstructure:"endpointName"`
	ServiceName   string `mapstructure:"serviceName"`
	Url           string `mapstructure:"url"`
	Port          uint16 `mapstructure:"port"`
	TrafficPolicy string `mapstructure:"trafficPolicy"`
}

type HelmOptions struct {
	Ext            *config.XMiniEnv
	ImageClient    image.Client
	GitClient      git.Client
	PublishClient  publishing.Client
	NgrokAuthToken string
	HelmDriver     string
	DryRun         bool
}

type Helm struct {
	ext               *config.XMiniEnv
	project           *config.ComposeProject
	services          map[string]serviceTuple
	servicesToPublish []NgrokConfig
	actionConfig      *helmaction.Configuration
	imageClient       image.Client
	gitClient         git.Client
	publishClient     publishing.Client
	ngrokAuthToken    string
	helmDriver        string
	dryRun            bool
}

func publishedNames(published []NgrokConfig) []string {
	names := make([]string, 0, len(published))
	for _, svc := range published {
		names = append(names, svc.EndpointName)
	}

	return names
}

// The agent reads ngrok.yml only at startup, and the pod template is otherwise
// constant, so this is what makes helm replace the pod when the config changes.
func ngrokConfigChecksum(published []NgrokConfig) string {
	sum := sha256.New()

	for _, svc := range published {
		writeChecksumParts(
			sum,
			svc.Namespace,
			svc.EndpointName,
			svc.ServiceName,
			svc.Url,
			strconv.Itoa(int(svc.Port)),
			svc.TrafficPolicy,
		)
	}

	writeChecksumParts(
		sum,
		config.NGROK_CONFIG_MAP_NAME,
		config.NGROK_CONFIG_KEY,
		HELM_NGROK_CONFIG_MAP_TMPL,
	)

	return hex.EncodeToString(sum.Sum(nil))
}

// Rotating the auth token otherwise leaves the agent on the old credential.
func ngrokSecretChecksum(authToken string) string {
	sum := sha256.New()

	writeChecksumParts(
		sum,
		config.NGROK_SECRET_NAME,
		helmNgrokSecretTmpl(authToken),
	)

	return hex.EncodeToString(sum.Sum(nil))
}

// Length-prefixed so different splits of the same bytes cannot hash alike.
func writeChecksumParts(sum hash.Hash, parts ...string) {
	for _, part := range parts {
		_, _ = fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
}

func NewHelm(opts HelmOptions) *Helm {
	return &Helm{
		ext:            opts.Ext,
		actionConfig:   nil,
		imageClient:    opts.ImageClient,
		gitClient:      opts.GitClient,
		publishClient:  opts.PublishClient,
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

	if err := h.deployInDependencyOrder(h.deployService); err != nil {
		return err
	}

	return h.installNgrokChart()
}

func (h *Helm) Destroy() error {
	if !h.Active() {
		return nil
	}

	destroyService := func(
		ctx context.Context,
		svc config.ComposeService,
	) error {
		return h.uninstallChart(ctx, svc.Name)
	}

	if err := h.destroyInReverseDependencyOrder(
		destroyService,
	); err != nil {
		return err
	}

	return h.uninstallNgrokChart()
}

// initProject is the half of Init that needs no cluster: it resolves every
// service extension once, up front, so a bad extension fails before a single
// release is touched, and so a +git tag costs one git call per run rather than
// one per service per pass. Everything downstream reads h.services, which makes
// Init a hard precondition of Deploy and Destroy.
func (h *Helm) initProject(project *config.ComposeProject) error {
	services := map[string]serviceTuple{}
	servicesToPublish := []NgrokConfig{}

	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(h.extensionOptions(svc))
		if err != nil {
			return err
		}
		services[svc.Name] = serviceTuple{compose: svc, extension: *svcExt}

		// resolveNgrok clears Ngrok without an auth token, so a non-zero port
		// means ngrok is configured and usable.
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
				servicesToPublish = append(servicesToPublish, NgrokConfig{
					Namespace:     h.ext.K8s.Namespace,
					EndpointName:  fmt.Sprintf("%s-%s", h.ext.K8s.Namespace, svc.Name),
					ServiceName:   svc.Name,
					Url:           svcExt.Ngrok.Url,
					Port:          svcExt.Ngrok.Port,
					TrafficPolicy: svcExt.Ngrok.TrafficPolicy,
				})
			}
		}
	}

	// project.Services is a map. An unstable order changes the ConfigMap
	// checksum, which reassigns every unreserved URL on an unchanged deploy.
	slices.SortFunc(servicesToPublish, func(a, b NgrokConfig) int {
		return strings.Compare(a.ServiceName, b.ServiceName)
	})

	h.services = services
	h.servicesToPublish = servicesToPublish
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

func (h *Helm) deployService(
	ctx context.Context,
	svc config.ComposeService,
) error {
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

	values, err := internalService.extension.ToChartValuesMap()
	if err != nil {
		return err
	}

	timeout := internalService.extension.DeploymentTimeout
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

	chart, err := h.loadChart(svc.Name, internalService.extension.DeploymentType)
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

func (h *Helm) upgradeOrInstallChart(
	ctx context.Context,
	chart *helmchart.Chart,
	values map[string]any,
	timeout time.Duration,
	restart bool,
) error {
	exists := h.serviceReleaseExists(chart.Name())

	if exists {
		log.Info().Str("release", chart.Name()).Msg("upgrading release")
		return h.upgradeChart(ctx, chart, values, timeout, restart)
	} else {
		log.Info().Str("release", chart.Name()).Msg("installing release")
		return h.installChart(ctx, chart, values, timeout)
	}
}

func (h *Helm) installChart(
	ctx context.Context,
	chart *helmchart.Chart,
	values map[string]any,
	timeout time.Duration,
) error {
	client := helmaction.NewInstall(h.actionConfig)
	client.ReleaseName = chart.Name()
	client.Namespace = h.ext.K8s.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.Wait = true
	client.WaitForJobs = true
	client.DryRun = h.dryRun
	client.Timeout = timeout

	if _, err := client.RunWithContext(ctx, chart, values); err != nil {
		return errs.Errorf(
			KindChartInstall,
			"failed to install service chart %s: %w",
			chart.Name(),
			err,
		)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
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
	client.Namespace = h.ext.K8s.Namespace
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
			KindChartUpgrade,
			"failed to upgrade service chart %s: %w",
			chart.Name(),
			err,
		)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
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
			KindChartUninstall,
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

// Deleting the last ngrok block has to uninstall the release, or it keeps
// serving endpoints the project no longer declares.
func (h *Helm) installNgrokChart() error {
	if len(h.servicesToPublish) == 0 {
		return h.uninstallNgrokChart()
	}

	chart, err := h.ngrokChart()
	if err != nil {
		return err
	}

	// restart=false: the checksum annotations decide when the pod rolls.
	// Forcing it would reassign every URL without a reserved domain.
	return h.upgradeOrInstallChart(
		context.Background(),
		chart,
		h.ngrokValues(),
		ngrokDeploymentTimeout,
		false,
	)
}

func (h *Helm) PublishedServiceUrls() (map[string]url.URL, error) {
	if len(h.servicesToPublish) == 0 {
		return nil, nil
	}

	return h.publishClient.ServiceUrls(publishedNames(h.servicesToPublish))
}

// Unconditional: the current ngrok config may no longer mention what was
// installed, and uninstallChart tolerates a missing release.
func (h *Helm) uninstallNgrokChart() error {
	return h.uninstallChart(context.Background(), ngrokReleaseName)
}

func (h *Helm) ngrokValues() map[string]any {
	if len(h.servicesToPublish) == 0 {
		return nil
	}

	values := map[string]any{
		"image": map[string]any{
			"repository": config.NGROK_IMAGE_REPO,
			"tag":        config.NGROK_IMAGE_TAG,
		},
		"configMapName":      config.NGROK_CONFIG_MAP_NAME,
		"configKey":          config.NGROK_CONFIG_KEY,
		"configVolMountPath": config.NGROK_CONFIG_VOL_MOUNT_PATH,
		"secretName":         config.NGROK_SECRET_NAME,
		"command": []string{
			"ngrok",
			"start",
			"--all",
			"--log=stdout",
		},
		// RollingUpdate would overlap two agents claiming the same endpoints.
		"strategy": map[string]any{"type": "Recreate"},
	}

	endpoints := []map[string]any{}
	for _, svc := range h.servicesToPublish {
		endpoints = append(endpoints, map[string]any{
			"endpointName":  svc.EndpointName,
			"namespace":     svc.Namespace,
			"serviceName":   svc.ServiceName,
			"url":           svc.Url,
			"port":          svc.Port,
			"trafficPolicy": svc.TrafficPolicy,
		})
	}
	values["endpoints"] = endpoints

	values["volumes"] = []map[string]any{
		{
			"name": config.NGROK_CONFIG_MAP_NAME,
			"configMap": map[string]any{
				"name": config.NGROK_CONFIG_MAP_NAME,
				"items": []map[string]any{
					{
						"key":  config.NGROK_CONFIG_KEY,
						"path": config.NGROK_CONFIG_KEY,
					},
				},
			},
		},
	}

	values["volumeMounts"] = []map[string]any{
		{
			"name":      config.NGROK_CONFIG_MAP_NAME,
			"mountPath": config.NGROK_CONFIG_VOL_MOUNT_PATH,
		},
	}

	values["envFrom"] = []map[string]any{
		{
			"secretRef": map[string]any{
				"name": config.NGROK_SECRET_NAME,
			},
		},
	}

	values["podAnnotations"] = map[string]string{
		"checksum/config": ngrokConfigChecksum(h.servicesToPublish),
		"checksum/secret": ngrokSecretChecksum(h.ngrokAuthToken),
	}

	return values
}

func (h *Helm) ngrokChart() (*helmchart.Chart, error) {
	files := []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(helmChartYamlTmpl(ngrokReleaseName)),
		},
		{
			Name: "values.yaml",
			Data: []byte(HELM_NGROK_VALUES_TMPL),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(HELM_HELPERS_TMPL),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(HELM_DEPLOYMENT_TMPL),
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

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			KindChartLoad,
			"failed to load in-memory ngrok chart: %w",
			err,
		)
	}

	return chart, nil
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
	files := []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(helmChartYamlTmpl(svcName)),
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

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			KindChartLoad,
			"failed to load in-memory chart for %s: %w",
			svcName,
			err,
		)
	}

	return chart, nil
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
