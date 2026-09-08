package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/rs/zerolog/log"
)

var urlRegex = regexp.MustCompile(`(?m)http:\/\/localhost(:?\:\d+)?(:?\/.*)?`)

const (
	defaultImagePlatform = "linux/amd64"
	defaultPortProtocol  = "TCP"
	httpDefaultPort      = "80"
	httpsDefaultPort     = "443"
)

// ChartImage is the configuration for a service image.
type ChartImage struct {
	// Image Repository for the service image
	Repository string `json:"repository" mapstructure:"repository"`
	// PullPolicy for this image
	PullPolicy string `json:"pullPolicy,omitempty" mapstructure:"pullPolicy,omitempty"`
	// Image tag for the service image
	Tag string `json:"tag" mapstructure:"tag"`
	// The platforms for which to build and push default [linux/amd64])
	Platforms []string `json:"platforms,omitempty" mapstructure:"platforms,omitempty"`
}

// ChartImagePullSecret names a secret that enables pulling private images.
type ChartImagePullSecret struct {
	// The name of the secret for pulling images
	Name string `json:"name" mapstructure:"name"`
}

// ChartServiceAccount is the service account configuration for the deployment.
type ChartServiceAccount struct {
	// Whether or not to create a Kubernetes service account
	Create *bool `json:"create,omitempty" mapstructure:"create,omitempty"`
	// The name for the service account
	Name string `json:"name,omitempty" mapstructure:"name,omitempty"`
	// Whether or not to automount the service account
	Automount *bool `json:"automount,omitempty" mapstructure:"automount,omitempty"`
	// Additional annotations for the service account
	Annotations map[string]string `json:"annotations,omitempty" mapstructure:"annotations,omitempty"`
}

// ChartServicePort is the port configuration used by both the service and the
// deployment pod container.
type ChartServicePort struct {
	// Name of the container port
	ContainerPortName string `json:"containerPortName" mapstructure:"containerPortName"`
	// Container port to expose to service
	ContainerPort uint16 `json:"containerPort" mapstructure:"containerPort"`
	// Name of the service port
	ServicePortName string `json:"servicePortName" mapstructure:"servicePortName"`
	// Service port to map to the container port
	ServicePort uint16 `json:"servicePort" mapstructure:"servicePort"`
	// Protocol to use for these ports
	Protocol string `json:"protocol" mapstructure:"protocol"`
}

// ChartService is the Kubernetes service configuration.
type ChartService struct {
	// Whether or not to create a Kubernetes service
	Create *bool `json:"create,omitempty" mapstructure:"create,omitempty"`
	// The type of service to create
	ServiceType string `json:"type,omitempty" mapstructure:"type,omitempty"`
	// The ports to associate with pod container and service mapping
	Ports []ChartServicePort `json:"ports" mapstructure:"ports"`
}

type ChartValues struct {
	// The number of replicas for this deployment
	Replicas uint8 `json:"replicas,omitempty" mapstructure:"replicas,omitempty"`
	// The image for this deployment. Will try to use compose service image if not set
	Image ChartImage `json:"image,omitzero" mapstructure:"image"`
	// Any image pull secrets required to pull images on the cluster
	ImagePullSecrets []ChartImagePullSecret `json:"imagePullSecrets,omitempty" mapstructure:"imagePullSecrets,omitempty"`
	// Command to run for the main deployment container
	Command []string `json:"command,omitempty" mapstructure:"command,omitempty"`
	// Service configuration including container and service port specifications
	Service ChartService `json:"service,omitzero" mapstructure:"service"`
	// Service account configuration
	ServiceAccount ChartServiceAccount `json:"serviceAccount,omitzero" mapstructure:"serviceAccount,omitzero"`
	// Container environment configuration
	Env map[string]string `json:"env,omitempty" mapstructure:"env,omitempty"`
	// Annotations to add to the deployment pods
	PodAnnotations map[string]string `json:"podAnnotations,omitempty" mapstructure:"podAnnotations,omitempty"`
	// Labels to add to the deployment pods
	PodLabels map[string]string `json:"podLabels,omitempty" mapstructure:"podLabels,omitempty"`
	// Security context for the deployment pods
	PodSecurityContext map[string]any `json:"podSecurityContext,omitempty" mapstructure:"podSecurityContext,omitempty"`
	// Security context for the container in each pod
	SecurityContext map[string]any `json:"securityContext,omitempty" mapstructure:"securityContext,omitempty"`
	// Resources configuration for deployment pods
	Resources map[string]any `json:"resources,omitempty" mapstructure:"resources,omitempty"`
	// Container startup probe configuration
	StartupProbe map[string]any `json:"startupProbe,omitempty" mapstructure:"startupProbe,omitempty"`
	// Container liveness probe configuration
	LivenessProbe map[string]any `json:"livenessProbe,omitempty" mapstructure:"livenessProbe,omitempty"`
	// Container readiness probe configuration
	ReadinessProbe map[string]any `json:"readinessProbe,omitempty" mapstructure:"readinessProbe,omitempty"`
	// Volumes configuration for the deployment pods
	Volumes []map[string]any `json:"volumes,omitempty" mapstructure:"volumes,omitempty"`
	// VolumeMounts configuration for the container
	VolumeMounts []map[string]any `json:"volumeMounts,omitempty" mapstructure:"volumeMounts,omitempty"`
	// NodeSelector configuration for the deployment pods
	NodeSelector map[string]any `json:"nodeSelector,omitempty" mapstructure:"nodeSelector,omitempty"`
	// Tolerations configuration for the deployment pods
	Tolerations []map[string]any `json:"tolerations,omitempty" mapstructure:"tolerations,omitempty"`
	// Affinity configuration for the deployment pods
	Affinity map[string]any `json:"affinity,omitempty" mapstructure:"affinity,omitempty"`
}

// XMiniEnvK8s holds the fields required for deploying to Kubernetes.
type XMiniEnvK8s struct {
	// Targets a specific cluster when deploying
	Context string `json:"context" mapstructure:"context"`
	// Targets a specific namespace when deploying
	Namespace string `json:"namespace" mapstructure:"namespace"`
	// Controls the Helm timeout. This is applied to all services but can be
	// overridden using the service-level extension
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
}

type K8sDeploymentType = string

const (
	K8sServiceDeploymentType K8sDeploymentType = "service"
	K8sJobDeploymentType     K8sDeploymentType = "job"
)

// XMiniEnvK8sService is the service level configuration controlling Kubernetes
// deployment properties.
type XMiniEnvK8sService struct {
	// Common properties
	XMiniEnvCommonService `mapstructure:",squash"`
	// Chart value overrides for the Helm deployment
	ChartValues `mapstructure:",squash"`

	// Recreates the service on each deploy even if values have not changed
	Recreate bool `json:"recreate,omitempty" mapstructure:"recreate"`
	// Controls the type of deployment (service | job). Default is "service"
	DeploymentType K8sDeploymentType `json:"deploymentType,omitempty" jsonschema:"enum=service,enum=job,default=service" mapstructure:"deploymentType,omitempty"`
	// Controls the Helm timeout for deploying the targeted service
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
}

// XMiniEnvK8sServiceOptions carries the runtime inputs needed to properly
// create and resolve a new XMiniEnvK8sService instance.
type XMiniEnvK8sServiceOptions struct {
	// The main top-level x-minienv extension config
	MainExt *XMiniEnv
	// The specific docker-compose service we are generating config for
	Service ComposeService
	// GitClient for resolving short-shas in +git tags
	GitClient git.Client
	// NgrokEnabled reports whether an ngrok auth token is available
	NgrokEnabled bool
}

func NewXMiniEnvK8sService(
	ctx context.Context,
	opts XMiniEnvK8sServiceOptions,
) (*XMiniEnvK8sService, error) {
	if opts.MainExt.K8s.Context == "" || opts.MainExt.K8s.Namespace == "" {
		return nil, errs.Errorf(
			ErrK8sNotConfigured,
			"k8s is not configured for this project",
		)
	}

	svc := opts.Service

	log.Info().Str("service", svc.Name).Msg("loading service extension")

	svcExt, ok := svc.Extensions[K8sServiceExtension]
	if !ok {
		svcExt = map[string]any{}
	}

	svcExtConfig := &XMiniEnvK8sService{}
	if err := mapstructure.Decode(svcExt, svcExtConfig); err != nil {
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

func (s *XMiniEnvK8sService) ToChartValuesMap() (map[string]any, error) {
	var values map[string]any

	if err := mapstructure.Decode(s.ChartValues, &values); err != nil {
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
			"servicePort":       p.ServicePort,
			"servicePortName":   p.ServicePortName,
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

	return values, nil
}

func (s *XMiniEnvK8sService) resolve(
	ctx context.Context,
	rawSvcExt any,
	opts XMiniEnvK8sServiceOptions,
) error {
	if s == nil {
		s = &XMiniEnvK8sService{}
	}

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

	s.resolveContainerCommand(opts.Service)
	s.resolveHealthCheck(opts.Service)
	s.resolveEnvironment(opts.Service)

	// After resolveServicePorts, which it checks the ngrok port against.
	if err := s.resolveNgrok(opts.MainExt, opts.NgrokEnabled); err != nil {
		return err
	}

	s.resolveDeploymentTimeout(opts.MainExt)

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

func getContainerPortName(ports []ChartServicePort, portStr string) string {
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

func createHTTPGetProbe(ports []ChartServicePort, u *url.URL) map[string]any {
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

func getProbes(cmd string, ports []ChartServicePort) k8sProbes {
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

	// All three start from the same check; only the intervals the caller adds
	// afterwards differ.
	probes.startup = probe
	probes.liveness = probeForCmd(cmd, ports)
	probes.readiness = probeForCmd(cmd, ports)

	return probes
}

// A curl or wget healthcheck against localhost is translated to a native
// httpGet probe, which does not need the binary present in the image. Anything
// else — including a url we cannot parse — falls back to exec.
func probeForCmd(cmd string, ports []ChartServicePort) map[string]any {
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

func (s *XMiniEnvK8sService) resolveCommonProperties(
	rawSvcExt any,
) error {
	common := XMiniEnvCommonService{}
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

func (s *XMiniEnvK8sService) resolveChartValues(rawSvcExt any) error {
	chartValues := ChartValues{}
	if err := mapstructure.Decode(rawSvcExt, &chartValues); err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to parse k8s chart values: %w",
			err,
		)
	}

	s.ChartValues = chartValues

	return nil
}

func (s *XMiniEnvK8sService) resolveNgrok(
	mainExt *XMiniEnv,
	ngrokEnabled bool,
) error {
	// With no auth token there is nothing to expose, so drop any ngrok config
	// entirely. Everything downstream can then treat a zero Ngrok.Port as
	// "ngrok is off" without needing to know about the token.
	if !ngrokEnabled {
		s.Ngrok = Ngrok{}
		return nil
	}

	if s.Ngrok.TrafficPolicy == "" && mainExt.Ngrok.TrafficPolicy != "" {
		s.Ngrok.TrafficPolicy = mainExt.Ngrok.TrafficPolicy
	}

	if s.Ngrok.Port == 0 {
		return nil
	}

	hasServicePort := slices.ContainsFunc(
		s.Service.Ports,
		func(svcPort ChartServicePort) bool {
			return svcPort.ServicePort == s.Ngrok.Port
		},
	)

	if !hasServicePort {
		return errs.Errorf(
			ErrNgrokPortMismatch,
			"exposeServicePort must match a mapped port either in extension or"+
				"from host port mapping in docker compose config",
		)
	}

	return nil
}

func (s *XMiniEnvK8sService) resolveServiceImage(
	ctx context.Context,
	svc ComposeService,
	gitClient git.Client,
) error {
	split := strings.SplitN(svc.Image, ":", 2)
	svcImageRepo := ""
	svcImageTag := ""

	if len(split) > 1 {
		svcImageRepo = split[0]
		svcImageTag = split[1]
	}

	if s.Image.Repository == "" {
		s.Image.Repository = svcImageRepo
	}

	if s.Image.Tag == "" {
		s.Image.Tag = svcImageTag
	}

	var err error

	if s.Image.Repository == "" {
		err = errors.Join(
			err,
			errs.Errorf(
				ErrImageRepositoryMissing,
				"image.repository must be specified in service extension",
			),
		)
	}

	if s.Image.Tag == "" {
		err = errors.Join(
			err,
			errs.Errorf(
				ErrImageTagMissing,
				"image.tag must be specified in service extension",
			),
		)
	}

	if strings.Contains(s.Image.Tag, "+git") {
		if err := s.expandGitTag(ctx, gitClient); err != nil {
			return err
		}
	}

	s.Image.Repository = strings.TrimSpace(s.Image.Repository)
	s.Image.Tag = strings.TrimSpace(s.Image.Tag)

	if len(s.Image.Platforms) == 0 {
		s.Image.Platforms = []string{defaultImagePlatform}
	}

	return err
}

// A "+git" tag is resolved here rather than at deploy time so every chart in
// one run embeds the same sha.
func (s *XMiniEnvK8sService) expandGitTag(
	ctx context.Context,
	gitClient git.Client,
) error {
	sha, err := gitClient.ShortSha(ctx)
	if err != nil {
		return errs.Errorf(
			ErrGitShortSha,
			"failed to get short sha from git for image tag: %w",
			err,
		)
	}

	s.Image.Tag = strings.ReplaceAll(s.Image.Tag, "+git", sha)

	return nil
}

func (s *XMiniEnvK8sService) resolveServicePorts(svc ComposeService) error {
	extensionHasPort := func(ctrPrt, svcPrt uint16) bool {
		return slices.ContainsFunc(s.Service.Ports, func(p ChartServicePort) bool {
			return p.ContainerPort == ctrPrt && p.ServicePort == svcPrt
		})
	}

	for _, p := range svc.Ports {
		if p.Target > math.MaxUint16 {
			return errs.Errorf(
				ErrInvalidPort,
				"invalid port configuration: %+v",
				p.Target,
			)
		}

		published, err := strconv.ParseUint(p.Published, 10, 16)
		if err != nil {
			return errs.Errorf(
				ErrInvalidPublishedPort,
				"invalid published port %q for container port %d: %w",
				p.Published,
				p.Target,
				err,
			)
		}

		published16 := uint16(published)

		protocol := defaultPortProtocol
		if p.Protocol != "" {
			protocol = strings.ToUpper(p.Protocol)
		}

		var containerPort = uint16(p.Target)

		containerPortName := fmt.Sprintf("p%d", containerPort)
		servicePortName := fmt.Sprintf("p%d", published16)

		if !extensionHasPort(containerPort, published16) {
			s.Service.Ports = append(s.Service.Ports, ChartServicePort{
				ContainerPortName: containerPortName,
				ContainerPort:     containerPort,
				ServicePort:       published16,
				ServicePortName:   servicePortName,
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

func (s *XMiniEnvK8sService) resolveEnvironment(svc ComposeService) {
	if len(svc.Environment) > 0 {
		mapping := map[string]string{}

		for key, value := range svc.Environment {
			// A nil value is compose's "inherit this variable from the host"
			// form (`environment: [FOO]`) that it could not resolve. Leave the
			// variable unset rather than setting it to the empty string.
			if value != nil {
				mapping[key] = *value
			}
		}

		if s.Env != nil {
			maps.Copy(mapping, s.Env)
		}

		s.Env = mapping
	}
}

// K8sDeploymentType is a string alias, so the compiler cannot reject a typo and
// neither can mapstructure. Validating here is what stops "service" from being
// waved through to the deployer, which would otherwise have to guess at it.
func (s *XMiniEnvK8sService) resolveDeploymentType() error {
	if s.DeploymentType == "" {
		s.DeploymentType = K8sServiceDeploymentType
		return nil
	}

	switch s.DeploymentType {
	case K8sServiceDeploymentType, K8sJobDeploymentType:
		return nil
	default:
		return errs.Errorf(
			ErrInvalidDeploymentType,
			"invalid deploymentType %q: must be one of %q, %q",
			s.DeploymentType,
			K8sServiceDeploymentType,
			K8sJobDeploymentType,
		)
	}
}

func (s *XMiniEnvK8sService) resolveContainerCommand(svc ComposeService) {
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

func (s *XMiniEnvK8sService) resolveHealthCheck(svc ComposeService) {
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

	s.applyProbes(getProbes(cmd, s.Service.Ports), hchkProps)
}

// An explicitly configured probe always wins; only the gaps are filled from
// the compose healthcheck.
func (s *XMiniEnvK8sService) applyProbes(
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

func (s *XMiniEnvK8sService) resolveDeploymentTimeout(
	mainExt *XMiniEnv,
) {
	if s.DeploymentTimeout == "" {
		if mainExt.K8s.DeploymentTimeout != "" {
			s.DeploymentTimeout = mainExt.K8s.DeploymentTimeout
		} else {
			s.DeploymentTimeout = HelmDefaultDeploymentTimeout
		}
	}
}
