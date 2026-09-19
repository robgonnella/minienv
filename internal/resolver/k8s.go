package resolver

import (
	"context"
	"fmt"
	"maps"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/util/validation"
)

var urlRegex = regexp.MustCompile(`(?m)http:\/\/localhost(:?\:\d+)?(:?\/.*)?`)

const (
	defaultPortProtocol = "TCP"
	httpDefaultPort     = "80"
	httpsDefaultPort    = "443"
)

type K8sServiceOptions struct {
	K8sExt       config.XMiniEnvK8s
	Service      compose.Service
	Raw          compose.RawService
	Dir          string
	GitClient    git.Client
	NgrokEnabled bool
}

type K8sService struct {
	config.XMiniEnvK8sService

	Compose   compose.Service
	Dir       string
	SecretEnv map[string]string
}

func NewK8sService(
	ctx context.Context,
	opts K8sServiceOptions,
) (*K8sService, error) {
	if opts.K8sExt.Context == "" || opts.K8sExt.Namespace == "" {
		return nil, errs.Errorf(
			ErrK8sNotConfigured,
			"k8s is not configured for this project",
		)
	}

	svc := opts.Service

	log.Info().Str("service", svc.Name).Msg("loading service extension")

	svcExt, ok := svc.Extensions[config.K8sServiceExtension]
	if !ok {
		svcExt = map[string]any{}
	}

	svcExtConfig := &K8sService{Compose: svc, Dir: opts.Dir}
	if err := mapstructure.Decode(
		svcExt,
		&svcExtConfig.XMiniEnvK8sService,
	); err != nil {
		return nil, errs.Errorf(
			ErrExtensionDecode,
			"failed to parse x-minienv-k8s-service extension: %w",
			err,
		)
	}

	if err := svcExtConfig.resolve(ctx, svcExt, opts); err != nil {
		return nil, err
	}

	return svcExtConfig, nil
}

func (s *K8sService) ChartValues() (map[string]any, error) {
	var values map[string]any

	if err := mapstructure.Decode(
		s.XMiniEnvK8sService.ChartValues,
		&values,
	); err != nil {
		return nil, errs.Errorf(
			ErrValuesDecode,
			"failed to resolve chart values: %w",
			err,
		)
	}

	service, ok := values["service"].(map[string]any)
	if !ok {
		service = map[string]any{}
	}

	servicePorts := []map[string]any{}
	for _, p := range s.Service.Ports {
		servicePorts = append(servicePorts, map[string]any{
			"containerPort":     p.ContainerPort,
			"containerPortName": p.ContainerPortName,
			"protocol":          p.Protocol,
		})
	}

	if len(servicePorts) > 0 {
		service["ports"] = servicePorts
	}

	flattenOptionalBool(service, "create", s.Service.Create)
	values["service"] = service

	serviceAccount, ok := values["serviceAccount"].(map[string]any)
	if !ok {
		serviceAccount = map[string]any{}
	}

	flattenOptionalBool(serviceAccount, "create", s.ServiceAccount.Create)
	flattenOptionalBool(serviceAccount, "automount", s.ServiceAccount.Automount)
	values["serviceAccount"] = serviceAccount

	pullSecrets := []map[string]any{}
	for _, s := range s.ImagePullSecrets {
		pullSecrets = append(pullSecrets, map[string]any{
			"name": s.Name,
		})
	}

	if len(pullSecrets) > 0 {
		values["imagePullSecrets"] = pullSecrets
	}

	if len(s.SecretEnv) > 0 {
		values["secretEnv"] = s.SecretEnv
	}

	return values, nil
}

func (s *K8sService) ManifestPaths() []string {
	return joinAll(s.Dir, s.Manifests)
}

func (s *K8sService) ConfigMapFromPaths() []string {
	return joinAll(s.Dir, s.ConfigMapFrom)
}

func joinAll(dir string, paths []string) []string {
	joined := make([]string, 0, len(paths))
	for _, p := range paths {
		joined = append(joined, filepath.Join(dir, p))
	}

	return joined
}

func (s *K8sService) resolve(
	ctx context.Context,
	rawSvcExt any,
	opts K8sServiceOptions,
) error {
	if err := s.resolveCommonProperties(rawSvcExt); err != nil {
		return err
	}

	if err := s.resolveChartValues(rawSvcExt); err != nil {
		return err
	}

	if err := s.resolveServiceImage(
		ctx,
		opts.Service,
		opts.GitClient,
	); err != nil {
		return err
	}

	if err := s.resolveServicePorts(opts.Service); err != nil {
		return err
	}

	if err := s.resolveDeploymentType(); err != nil {
		return err
	}

	if err := s.resolveManifests(); err != nil {
		return err
	}

	if err := s.resolveConfigMapFrom(); err != nil {
		return err
	}

	s.resolveContainerCommand(opts.Service)
	s.resolveHealthCheck(opts.Service)
	s.resolveEnvironment(opts.Service, opts.Raw)

	// After resolveServicePorts, which it checks the ngrok port against.
	if err := s.resolveNgrok(opts.K8sExt, opts.NgrokEnabled); err != nil {
		return err
	}

	s.resolveDeploymentTimeout(opts.K8sExt)

	return nil
}

// flattenOptionalBool replaces an optional bool with the plain value the chart
// templates expects.
func flattenOptionalBool(values map[string]any, key string, value *bool) {
	if value == nil {
		delete(values, key)
		return
	}

	values[key] = *value
}

type healthCheckProps struct {
	intervalSeconds      int
	startIntervalSeconds int
	startPeriodSeconds   int
	timeoutSeconds       int
	retries              int
}

func healthCheckProperties(hchk *types.HealthCheckConfig) healthCheckProps {
	intervalSeconds := 0
	if hchk.Interval != nil {
		intervalSeconds = int(
			time.Duration(*hchk.Interval) / time.Second,
		)
	}

	startIntervalSeconds := 0
	if hchk.StartInterval != nil {
		startIntervalSeconds = int(
			time.Duration(*hchk.StartInterval) / time.Second,
		)
	}

	startPeriodSeconds := 0
	if hchk.StartPeriod != nil {
		startPeriodSeconds = int(
			time.Duration(*hchk.StartPeriod) / time.Second,
		)
	}

	timeoutSeconds := 0
	if hchk.Timeout != nil {
		timeoutSeconds = int(
			time.Duration(*hchk.Timeout) / time.Second,
		)
	}

	retries := 0
	// Clamped rather than converted straight: compose types Retries as uint64,
	// which does not fit int on a 32-bit build, and failureThreshold is an
	// int32 on the k8s side regardless.
	if hchk.Retries != nil {
		retries = int(min(*hchk.Retries, math.MaxInt32))
	}

	return healthCheckProps{
		intervalSeconds,
		startIntervalSeconds,
		startPeriodSeconds,
		timeoutSeconds,
		retries,
	}
}

func getContainerPortName(
	ports []config.ChartServicePort,
	portStr string,
) string {
	for _, p := range ports {
		if strconv.Itoa(int(p.ContainerPort)) == portStr {
			return p.ContainerPortName
		}
	}

	return ""
}

func createExecProbe(cmd string) map[string]any {
	return map[string]any{
		"exec": map[string]any{
			"command": []string{
				"/bin/sh",
				"-c",
				cmd,
			},
		},
	}
}

func createHTTPGetProbe(
	ports []config.ChartServicePort,
	u *url.URL,
) map[string]any {
	defaultPort := httpDefaultPort

	if u.Scheme == "https" {
		defaultPort = httpsDefaultPort
	}

	port := u.Port()

	if port == "" {
		port = defaultPort
	}

	name := getContainerPortName(ports, port)

	return map[string]any{
		"httpGet": map[string]any{
			"path": u.Path,
			"port": name,
		},
	}
}

type k8sProbes struct {
	startup   map[string]any
	liveness  map[string]any
	readiness map[string]any
}

func addLivenessReadinessIntervals(p map[string]any, props healthCheckProps) {
	if props.intervalSeconds > 0 {
		p["periodSeconds"] = props.intervalSeconds
	}

	if props.timeoutSeconds > 0 {
		p["timeoutSeconds"] = props.timeoutSeconds
	}

	if props.retries > 0 {
		p["failureThreshold"] = props.retries
	}
}

func addStartUpIntervals(p map[string]any, props healthCheckProps) {
	if props.startIntervalSeconds > 0 {
		p["periodSeconds"] = props.startIntervalSeconds
	}

	if props.timeoutSeconds > 0 {
		p["timeoutSeconds"] = props.timeoutSeconds
	}

	if props.startIntervalSeconds > 0 && props.startPeriodSeconds > 0 {
		p["failureThreshold"] = math.Ceil(
			float64(props.startPeriodSeconds) / float64(props.startIntervalSeconds),
		)
	}
}

func getInitialProbes(cmd string, ports []config.ChartServicePort) k8sProbes {
	probes := k8sProbes{
		startup:   nil,
		liveness:  nil,
		readiness: nil,
	}

	if cmd == "" {
		return probes
	}

	probe := probeForCmd(cmd, ports)
	if probe == nil {
		return probes
	}

	probes.startup = probe
	probes.liveness = probeForCmd(cmd, ports)
	probes.readiness = probeForCmd(cmd, ports)

	return probes
}

// A curl or wget healthcheck against localhost is translated to a native
// httpGet probe, which does not need the binary present in the image. Anything
// else — including a url we cannot parse — falls back to exec.
func probeForCmd(cmd string, ports []config.ChartServicePort) map[string]any {
	if !strings.HasPrefix(cmd, "curl") && !strings.HasPrefix(cmd, "wget") {
		return createExecProbe(cmd)
	}

	matches := urlRegex.FindStringSubmatch(cmd)
	if len(matches) == 0 {
		return nil
	}

	parsedURL, err := url.Parse(matches[0])
	if err != nil {
		return createExecProbe(cmd)
	}

	return createHTTPGetProbe(ports, parsedURL)
}

func (s *K8sService) resolveCommonProperties(rawSvcExt any) error {
	common := config.XMiniEnvCommonService{}
	if err := mapstructure.Decode(rawSvcExt, &common); err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to parse common service properties: %w",
			err,
		)
	}

	s.XMiniEnvCommonService = common

	return nil
}

func (s *K8sService) resolveChartValues(rawSvcExt any) error {
	chartValues := config.ChartValues{}
	if err := mapstructure.Decode(rawSvcExt, &chartValues); err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to parse k8s chart values: %w",
			err,
		)
	}

	s.XMiniEnvK8sService.ChartValues = chartValues

	return nil
}

func (s *K8sService) resolveNgrok(
	k8sExt config.XMiniEnvK8s,
	ngrokEnabled bool,
) error {
	containerPorts := make([]uint16, 0, len(s.Service.Ports))
	for _, p := range s.Service.Ports {
		containerPorts = append(containerPorts, p.ContainerPort)
	}

	return resolveNgrok(k8sExt.Ngrok, &s.Ngrok, containerPorts, ngrokEnabled)
}

func (s *K8sService) resolveServiceImage(
	ctx context.Context,
	svc compose.Service,
	gitClient git.Client,
) error {
	if s.Skip {
		return nil
	}

	image := &config.ServiceImage{
		Repository: s.Image.Repository,
		Tag:        s.Image.Tag,
		Platforms:  s.Image.Platforms,
	}

	if err := resolveServiceImage(ctx, image, svc, gitClient); err != nil {
		return err
	}

	s.Image.Repository = image.Repository
	s.Image.Tag = image.Tag
	s.Image.Platforms = image.Platforms

	return nil
}

func (s *K8sService) resolveServicePorts(svc compose.Service) error {
	extensionHasPort := func(ctrPrt uint16) bool {
		return slices.ContainsFunc(
			s.Service.Ports,
			func(p config.ChartServicePort) bool {
				return p.ContainerPort == ctrPrt
			},
		)
	}

	for _, p := range svc.Ports {
		if p.Target > math.MaxUint16 {
			return errs.Errorf(
				ErrInvalidPort,
				"invalid port configuration: %+v",
				p.Target,
			)
		}

		protocol := defaultPortProtocol
		if p.Protocol != "" {
			protocol = strings.ToUpper(p.Protocol)
		}

		var containerPort = uint16(p.Target)

		containerPortName := fmt.Sprintf("p%d", containerPort)

		if !extensionHasPort(containerPort) {
			s.Service.Ports = append(s.Service.Ports, config.ChartServicePort{
				ContainerPortName: containerPortName,
				ContainerPort:     containerPort,
				Protocol:          protocol,
			})
		}
	}

	if len(s.Service.Ports) == 0 {
		s.Service.Create = new(false)
		s.ServiceAccount.Create = new(false)
	}

	return nil
}

func (s *K8sService) resolveEnvironment(
	svc compose.Service,
	raw compose.RawService,
) {
	if len(svc.Environment) == 0 && len(s.Env) == 0 {
		return
	}

	k8sHasValue := func(key string) bool {
		return slices.ContainsFunc(s.Env, func(entry map[string]any) bool {
			return entry["name"] == key
		})
	}

	hostSourced := hostSourcedKeys(svc, raw)
	k8sMappings, secretEnv := s.splitExtensionEnv(hostSourced)

	// Sorted: an unchanged project must render an unchanged pod template.
	for _, key := range slices.Sorted(maps.Keys(svc.Environment)) {
		value := svc.Environment[key]

		// A nil value is compose's "inherit this variable from the host"
		// form (`environment: [FOO]`) that it could not resolve. Leave the
		// variable unset rather than setting it to the empty string.
		if value == nil || k8sHasValue(key) {
			continue
		}

		if hostSourced[key] {
			secretEnv[key] = *value

			continue
		}

		k8sMappings = append(k8sMappings, map[string]any{
			"name":  key,
			"value": *value,
		})
	}

	s.Env = k8sMappings

	if len(secretEnv) > 0 {
		s.SecretEnv = secretEnv
	}
}

func (s *K8sService) splitExtensionEnv(
	hostSourced map[string]bool,
) ([]map[string]any, map[string]string) {
	k8sMappings := []map[string]any{}
	secretEnv := map[string]string{}

	for _, entry := range s.Env {
		name, _ := entry["name"].(string)
		value, isString := entry["value"].(string)

		if isString && hostSourced[name] {
			secretEnv[name] = value

			continue
		}

		k8sMappings = append(k8sMappings, entry)
	}

	return k8sMappings, secretEnv
}

func hostSourcedKeys(
	svc compose.Service,
	raw compose.RawService,
) map[string]bool {
	literal := compose.LiteralEnvKeys(raw.Environment)
	hostSourced := map[string]bool{}

	for key, value := range svc.Environment {
		if value != nil && !literal[key] {
			hostSourced[key] = true
		}
	}

	for _, entry := range rawExtensionEnv(raw) {
		name, ok := entry["name"].(string)
		if !ok {
			continue
		}

		value, ok := entry["value"].(string)
		if !ok {
			continue
		}

		if compose.Templated(value) {
			hostSourced[name] = true
		} else {
			delete(hostSourced, name)
		}
	}

	return hostSourced
}

func rawExtensionEnv(raw compose.RawService) []map[string]any {
	ext, ok := raw.Extensions[config.K8sServiceExtension].(map[string]any)
	if !ok {
		return nil
	}

	entries, ok := ext["env"].([]any)
	if !ok {
		return nil
	}

	env := make([]map[string]any, 0, len(entries))

	for _, entry := range entries {
		if m, ok := entry.(map[string]any); ok {
			env = append(env, m)
		}
	}

	return env
}

// K8sDeploymentType is a string alias, so the compiler cannot reject a typo and
// neither can mapstructure. Validating here is what stops "service" from being
// waved through to the deployer, which would otherwise have to guess at it.
func (s *K8sService) resolveDeploymentType() error {
	if s.DeploymentType == "" {
		s.DeploymentType = config.K8sServiceDeploymentType
		return nil
	}

	switch s.DeploymentType {
	case config.K8sServiceDeploymentType, config.K8sJobDeploymentType:
		return nil
	default:
		return errs.Errorf(
			ErrInvalidDeploymentType,
			"invalid deploymentType %q: must be one of %q, %q",
			s.DeploymentType,
			config.K8sServiceDeploymentType,
			config.K8sJobDeploymentType,
		)
	}
}

// A declared path names the chart file it becomes, so one climbing out of the
// project has no name to take.
func (s *K8sService) resolveManifests() error {
	names := map[string]bool{}

	for i, declared := range s.Manifests {
		cleaned := filepath.Clean(declared)

		if !filepath.IsLocal(cleaned) {
			return errs.Errorf(
				ErrManifestPath,
				"manifest path must be a relative path inside the project: %s",
				declared,
			)
		}

		name := filepath.Base(cleaned)

		if names[name] {
			return errs.Errorf(
				ErrManifestPath,
				"manifest file name is already used by this service: %s",
				name,
			)
		}

		names[name] = true
		s.Manifests[i] = cleaned
	}

	return nil
}

func (s *K8sService) resolveConfigMapFrom() error {
	keys := map[string]bool{}

	for i, declared := range s.ConfigMapFrom {
		cleaned := filepath.Clean(declared)

		if cleaned == "." || !filepath.IsLocal(cleaned) {
			return errs.Errorf(
				ErrConfigMapFromPath,
				"configMapFrom path must name a file inside the project: %s",
				declared,
			)
		}

		key := filepath.Base(cleaned)

		if msgs := validation.IsConfigMapKey(key); len(msgs) > 0 {
			return errs.Errorf(
				ErrConfigMapFromPath,
				"configMapFrom file name %q is not a valid ConfigMap key: %s",
				key,
				strings.Join(msgs, "; "),
			)
		}

		if keys[key] {
			return errs.Errorf(
				ErrConfigMapFromPath,
				"configMapFrom file name is already used by this service: %s",
				key,
			)
		}

		keys[key] = true
		s.ConfigMapFrom[i] = cleaned
	}

	return nil
}

func (s *K8sService) resolveContainerCommand(svc compose.Service) {
	if s.Command != nil {
		return
	}

	if len(svc.Command) > 0 {
		s.Command = slices.Clone(svc.Command)
	}
}

// healthCheckCommand collapses a compose test vector to the shell command it
// runs. The bool reports whether there is anything to probe at all: "NONE" and
// a bare "CMD-SHELL" both mean there is not.
func healthCheckCommand(test []string) (string, bool) {
	switch test[0] {
	case "CMD":
		return strings.Join(test[1:], " "), true
	case "CMD-SHELL":
		rest := test[1:]
		if len(rest) == 0 {
			return "", false
		}

		return rest[0], true
	default:
		return "", false
	}
}

func (s *K8sService) resolveHealthCheck(svc compose.Service) {
	if svc.HealthCheck == nil || svc.HealthCheck.Disable {
		return
	}

	if len(svc.HealthCheck.Test) == 0 {
		return
	}

	cmd, ok := healthCheckCommand(svc.HealthCheck.Test)
	if !ok {
		return
	}

	hchkProps := healthCheckProperties(svc.HealthCheck)

	s.applyProbes(getInitialProbes(cmd, s.Service.Ports), hchkProps)
}

// An explicitly configured probe always wins; only the gaps are filled from
// the compose healthcheck.
func (s *K8sService) applyProbes(
	probes k8sProbes,
	props healthCheckProps,
) {
	if s.StartupProbe == nil && probes.startup != nil {
		addStartUpIntervals(probes.startup, props)
		s.StartupProbe = probes.startup
	}

	if s.LivenessProbe == nil && probes.liveness != nil {
		addLivenessReadinessIntervals(probes.liveness, props)
		s.LivenessProbe = probes.liveness
	}

	if s.ReadinessProbe == nil && probes.readiness != nil {
		addLivenessReadinessIntervals(probes.readiness, props)
		s.ReadinessProbe = probes.readiness
	}
}

func (s *K8sService) resolveDeploymentTimeout(k8sExt config.XMiniEnvK8s) {
	if s.DeploymentTimeout == "" {
		if k8sExt.DeploymentTimeout != "" {
			s.DeploymentTimeout = k8sExt.DeploymentTimeout
		} else {
			s.DeploymentTimeout = config.HelmDefaultDeploymentTimeout
		}
	}
}
