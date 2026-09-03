package config

import (
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

	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/os"
	"github.com/rs/zerolog/log"
)

var urlRegex = regexp.MustCompile(`(?m)http:\/\/localhost(:?\:\d+)?(:?\/.*)?`)

// Configuration for service image
type ChartImage struct {
	// Image Repository for the service image
	Repository string `json:"repository" yaml:"repository" mapstructure:"repository"`
	// PullPolicy for this image
	PullPolicy string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty" mapstructure:"pullPolicy,omitempty"`
	// Image tag for the service image
	Tag string `json:"tag" yaml:"tag" mapstructure:"tag"`
	// The platforms for which to build and push default [linux/amd64])
	Platforms []string `json:"platforms,omitempty" yaml:"platforms,omitempty" mapstructure:"platforms,omitempty"`
}

// Pull secrets to enable pulling private images
type ChartImagePullSecret struct {
	// The name of the secret for pulling images
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// ServiceAccount configuration for the deployment service
type ChartServiceAccount struct {
	// Whether or not to create a Kubernetes service account
	Create bool `json:"create,omitempty" yaml:"create,omitempty" mapstructure:"create,omitempty"`
	// The name for the service account
	Name string `json:"name,omitempty" yaml:"name,omitempty" mapstructure:"name,omitempty"`
	// Whether or not to automount the service account
	Automount bool `json:"automount,omitempty" yaml:"automount,omitempty" mapstructure:"automount,omitempty"`
	// Additional annotations for the service account
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty" mapstructure:"annotations,omitempty"`
}

// Port configuration use in services and deployment pod container
type ChartServicePort struct {
	// Name of the container port
	ContainerPortName string `json:"containerPortName" yaml:"containerPortName" mapstructure:"containerPortName"`
	// Container port to expose to service
	ContainerPort uint16 `json:"containerPort" yaml:"containerPort" mapstructure:"containerPort"`
	// Name of the service port
	ServicePortName string `json:"servicePortName" yaml:"servicePortName" mapstructure:"servicePortName"`
	// Service port to map to the container port
	ServicePort uint16 `json:"servicePort" yaml:"servicePort" mapstructure:"servicePort"`
	// Protocol to use for these ports
	Protocol string `json:"protocol" yaml:"protocol" mapstructure:"protocol"`
}

// Service configuration
type ChartService struct {
	// Whether or not to create a Kubernetes service
	Create bool `json:"create,omitempty" yaml:"create,omitempty" mapstructure:"create,omitempty"`
	// The type of service to create
	ServiceType string `json:"type,omitempty" yaml:"type,omitempty" mapstructure:"type,omitempty"`
	// The ports to associate with pod container and service mapping
	Ports []ChartServicePort `json:"ports" yaml:"ports" mapstructure:"ports"`
}

type ChartValues struct {
	// The number of replicas for this deployment
	Replicas uint8 `json:"replicas,omitempty" yaml:"replicas,omitempty" mapstructure:"replicas,omitempty,omitempty"`
	// The image for this deployment. Will try to use compose service image if not set
	Image ChartImage `json:"image,omitzero" yaml:"image,omitzero" mapstructure:"image,omitzero"`
	// Any image pull secrets required to pull images on the cluster
	ImagePullSecrets []ChartImagePullSecret `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty" mapstructure:"imagePullSecrets,omitempty"`
	// Service configuration including container and service port specifications
	Service ChartService `json:"service,omitzero" yaml:"service,omitzero" mapstructure:"service,omitzero"`
	// Service account configuration
	ServiceAccount ChartServiceAccount `json:"serviceAccount,omitzero" yaml:"serviceAccount,omitzero" mapstructure:"serviceAccount,omitzero"`
	// Container environment configuration
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty" mapstructure:"env,omitempty"`
	// Annotations to add to the deployment pods
	PodAnnotations map[string]string `json:"podAnnotations,omitempty" yaml:"podAnnotations,omitempty" mapstructure:"podAnnotations,omitempty"`
	// Labels to add to the deployment pods
	PodLabels map[string]string `json:"podLabels,omitempty" yaml:"podLabels,omitempty" mapstructure:"podLabels,omitempty"`
	// Security context for the deployment pods
	PodSecurityContext map[string]any `json:"podSecurityContext,omitempty" yaml:"podSecurityContext,omitempty" mapstructure:"podSecurityContext,omitempty"`
	// Security context for the container in each pod
	SecurityContext map[string]any `json:"securityContext,omitempty" yaml:"securityContext,omitempty" mapstructure:"securityContext,omitempty"`
	// Resources configuration for deployment pods
	Resources map[string]any `json:"resources,omitempty" yaml:"resources,omitempty" mapstructure:"resources,omitempty"`
	// Container startup probe configuration
	StartupProbe map[string]any `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty" mapstructure:"startupProbe,omitempty"`
	// Container liveness probe configuration
	LivenessProbe map[string]any `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty" mapstructure:"livenessProbe,omitempty"`
	// Container readiness probe configuration
	ReadinessProbe map[string]any `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty" mapstructure:"readinessProbe,omitempty"`
	// Volumes configuration for the deployment pods
	Volumes []map[string]any `json:"volumes,omitempty" yaml:"volumes,omitempty" mapstructure:"volumes,omitempty"`
	// VolumeMounts configuration for the container
	VolumeMounts []map[string]any `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty" mapstructure:"volumeMounts,omitempty"`
	// NodeSelector configuration for the deployment pods
	NodeSelector map[string]any `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty" mapstructure:"nodeSelector,omitempty"`
	// Tolerations configuration for the deployment pods
	Tolerations []map[string]any `json:"tolerations,omitempty" yaml:"tolerations,omitempty" mapstructure:"tolerations,omitempty"`
	// Affinity configuration for the deployment pods
	Affinity map[string]any `json:"affinity,omitempty" yaml:"affinity,omitempty" mapstructure:"affinity,omitempty"`
}

// Required fields for deploying to Kubernetes
type XMiniEnvK8s struct {
	// Targets a specific cluster when deploying
	Context string `json:"context" yaml:"context" mapstructure:"context"`
	// Targets a specific namespace when deploying
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`
	// Controls the Helm timeout. This is applied to all services but can be
	// overriden using the service-level extension
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" yaml:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
}

// Service level configuration for controlling Kubernetes deployment properties
type XMiniEnvK8sService struct {
	// Common properties
	XMiniEnvCommonService
	// Chart value overrides for the Helm deployment
	ChartValues
	// Controls the Helm timeout for deploying the targeted service
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" yaml:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
}

func NewXMiniEnvK8sService(
	mainExt *XMiniEnv,
	svc ComposeService,
	commander os.Commander,
) (*XMiniEnvK8sService, error) {
	if mainExt.K8s.Context == "" || mainExt.K8s.Namespace == "" {
		return nil, Errorf("k8s is not configured for this project")
	}

	svcExt, ok := svc.Extensions[K8S_SERVICE_EXTENSION]
	if !ok {
		svcExt = map[string]any{}
	}

	log.Info().Str("service", svc.Name).Msg("processing service")

	svcExtConfig := &XMiniEnvK8sService{}
	if err := mapstructure.Decode(svcExt, svcExtConfig); err != nil {
		return nil, Errorf(
			"failed to parse x-minienv-k8s-service extension: %s",
			err,
		)
	}

	if err := svcExtConfig.resolve(mainExt, svcExt, svc, commander); err != nil {
		return nil, err
	}

	return svcExtConfig, nil
}

func (s *XMiniEnvK8sService) resolve(
	mainExt *XMiniEnv,
	rawSvcExt any,
	svc ComposeService,
	commander os.Commander,
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

	if err := s.resolveServiceImage(svc, commander); err != nil {
		return err
	}

	if err := s.resolveServicePorts(svc); err != nil {
		return err
	}

	s.resolveHealthCheck(svc)
	s.resolveEnvironment(svc)
	s.resolveNgrok(mainExt, svc)

	if s.DeploymentTimeout == "" {
		if mainExt.K8s.DeploymentTimeout != "" {
			s.DeploymentTimeout = mainExt.K8s.DeploymentTimeout
		} else {
			s.DeploymentTimeout = HELM_DEFAULT_DEPLOYMENT_TIMEOUT
		}
	}

	return nil
}

func (s *XMiniEnvK8sService) ToValuesMap() (map[string]any, error) {
	var values map[string]any

	if err := mapstructure.Decode(s.ChartValues, &values); err != nil {
		return nil, Errorf("failed to resolve chart values: %s", err)
	}

	if err := s.decodeNestedStructures(values); err != nil {
		return nil, err
	}

	hasServicePort := func(p uint16) bool {
		for _, svcPrt := range s.Service.Ports {
			if svcPrt.ServicePort == p {
				return true
			}
		}
		return false
	}

	if NGROK_AUTHTOKEN != "" && s.Ngrok.Port != 0 {
		if !hasServicePort(s.Ngrok.Port) {
			return nil, Errorf(
				"exposeServicePort must match a mapped port either in extension or" +
					"from host port mapping in docker compose config",
			)
		}

		ngrok := map[string]any{
			"enabled": true,
			"port":    s.Ngrok.Port,
		}

		if s.Ngrok.Port != 0 && s.Ngrok.TrafficPolicy != "" {
			ngrok["trafficPolicy"] = s.Ngrok.TrafficPolicy
		}

		values["ngrok"] = ngrok
	}

	log.Info().Fields(values).Msg("resolved chart values")

	return values, nil

}

func (s *XMiniEnvK8sService) resolveCommonProperties(
	rawSvcExt any,
) error {
	common := XMiniEnvCommonService{}
	if err := mapstructure.Decode(rawSvcExt, &common); err != nil {
		return Errorf(
			"failed to parse common service properties: %s",
			err,
		)
	}
	s.XMiniEnvCommonService = common
	return nil
}

func (s *XMiniEnvK8sService) resolveChartValues(rawSvcExt any) error {
	chartValues := ChartValues{}
	if err := mapstructure.Decode(rawSvcExt, &chartValues); err != nil {
		return Errorf(
			"failed to parse k8s chart values: %s",
			err,
		)
	}
	s.ChartValues = chartValues

	return nil
}

func (s *XMiniEnvK8sService) resolveNgrok(
	mainExt *XMiniEnv,
	svc ComposeService,
) {
	if s.Ngrok.TrafficPolicy == "" && mainExt.Ngrok.TrafficPolicy != "" {
		s.Ngrok.TrafficPolicy = mainExt.Ngrok.TrafficPolicy
	}

	if NGROK_AUTHTOKEN != "" && s.Ngrok.Port != 0 {
		volumes := []map[string]any{}
		if s.Volumes != nil {
			volumes = slices.Concat(volumes, s.Volumes)
		}
		volumes = append(volumes, map[string]any{
			"name": NGROK_CONFIG_MAP_NAME,
			"configMap": map[string]any{
				"name": NGROK_CONFIG_MAP_NAME,
				"items": []map[string]any{
					{
						"key":  NGROK_CONFIG_KEY,
						"path": NGROK_CONFIG_KEY,
					},
				},
			},
		})
		s.Volumes = volumes
	}
}

func (s *XMiniEnvK8sService) resolveServiceImage(
	svc ComposeService,
	commander os.Commander,
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
			Errorf("image.repository must be specified in service extension"),
		)
	}

	if s.Image.Tag == "" {
		err = errors.Join(
			err,
			Errorf("image.tag must be specified in service extension"),
		)
	}

	if strings.Contains(s.Image.Tag, "+git") {
		sha, err := commander.
			Command("git", "rev-parse", "--short", "HEAD").Output()

		if err != nil {
			return Errorf(
				"failed to get short sha from git for image tag: %s",
				err,
			)
		}
		s.Image.Tag = strings.ReplaceAll(s.Image.Tag, "+git", string(sha))
	}

	s.Image.Repository = strings.TrimSpace(s.Image.Repository)
	s.Image.Tag = strings.TrimSpace(s.Image.Tag)

	if len(s.Image.Platforms) == 0 {
		s.Image.Platforms = []string{"linux/amd64"}
	}

	return err
}

func (s *XMiniEnvK8sService) resolveServicePorts(svc ComposeService) error {
	if len(svc.Ports) == 0 {
		s.Service.Create = false
		s.ServiceAccount.Create = false
		return nil
	}

	extensionHasPort := func(ctrPrt, svcPrt uint16) bool {
		for _, extPrt := range s.Service.Ports {
			if extPrt.ContainerPort == ctrPrt && extPrt.ServicePort == svcPrt {
				return true
			}
		}
		return false
	}

	for _, p := range svc.Ports {
		if p.Target > math.MaxInt16 {
			return Errorf("invalid port configuration: %+v", p.Target)
		}

		published, err := strconv.ParseUint(p.Published, 10, 16)
		if err != nil {
			return err
		}
		published16 := uint16(published)

		protocol := "TCP"
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

	return nil
}

func (s *XMiniEnvK8sService) resolveEnvironment(svc ComposeService) {
	if len(svc.Environment) > 0 {
		mapping := map[string]string{}

		for key, value := range svc.Environment {
			mapping[key] = *value
		}

		if s.Env != nil {
			maps.Copy(mapping, s.Env)
		}

		s.Env = mapping
	}
}

func (s *XMiniEnvK8sService) resolveHealthCheck(svc ComposeService) {
	if svc.HealthCheck == nil || svc.HealthCheck.Disable {
		return
	}

	instruction := svc.HealthCheck.Test[0]
	test := svc.HealthCheck.Test[1:]

	intervalSeconds := 0
	if svc.HealthCheck.Interval != nil {
		intervalSeconds = int(
			time.Duration(*svc.HealthCheck.Interval) / time.Second,
		)
	}

	startIntervalSeconds := 0
	if svc.HealthCheck.StartInterval != nil {
		startIntervalSeconds = int(
			time.Duration(*svc.HealthCheck.StartInterval) / time.Second,
		)
	}

	startPeriodSeconds := 0
	if svc.HealthCheck.StartPeriod != nil {
		startPeriodSeconds = int(
			time.Duration(*svc.HealthCheck.StartPeriod) / time.Second,
		)
	}

	timeoutSeconds := 0
	if svc.HealthCheck.Timeout != nil {
		timeoutSeconds = int(
			time.Duration(*svc.HealthCheck.Timeout) / time.Second,
		)
	}

	retries := 0
	if svc.HealthCheck.Retries != nil {
		retries = int(*svc.HealthCheck.Retries)
	}

	cmd := ""

	switch instruction {
	case "NONE":
		return
	case "CMD":
		cmd = strings.Join(test, " ")
	case "CMD-SHELL":
		cmd = test[0]
	}

	var startupProbe map[string]any = nil
	var livenessProbe map[string]any = nil
	var readinessProbe map[string]any = nil

	getContainerPortName := func(portStr string) string {
		for _, p := range s.Service.Ports {
			if strconv.Itoa(int(p.ContainerPort)) == portStr {
				return p.ContainerPortName
			}
		}
		return ""
	}

	createExecProbe := func() map[string]any {
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

	createHttpGetProbe := func(u *url.URL) map[string]any {
		defaultPort := "80"

		if u.Scheme == "https" {
			defaultPort = "443"
		}

		port := u.Port()

		if port == "" {
			port = defaultPort
		}

		name := getContainerPortName(port)

		return map[string]any{
			"httpGet": map[string]any{
				"path": u.Path,
				"port": name,
			},
		}
	}

	addLivenessReadinessIntervals := func(p map[string]any) {
		if intervalSeconds > 0 {
			p["periodSeconds"] = intervalSeconds
		}
		if timeoutSeconds > 0 {
			p["timeoutSeconds"] = timeoutSeconds
		}
		if retries > 0 {
			p["failureThreshold"] = retries
		}
	}

	addStartUpIntervals := func(p map[string]any) {
		if startIntervalSeconds > 0 {
			p["periodSeconds"] = startIntervalSeconds
		}
		if timeoutSeconds > 0 {
			p["timeoutSeconds"] = timeoutSeconds
		}
		if startIntervalSeconds > 0 && startPeriodSeconds > 0 {
			p["failureThreshold"] = math.Ceil(
				float64(startPeriodSeconds) / float64(startIntervalSeconds),
			)
		}
	}

	if strings.HasPrefix(cmd, "curl") || strings.HasPrefix(cmd, "wget") {
		matches := urlRegex.FindStringSubmatch(cmd)
		if len(matches) > 0 {
			parsedURL, err := url.Parse(matches[0])
			if err != nil {
				startupProbe = createExecProbe()
				livenessProbe = createExecProbe()
				readinessProbe = createExecProbe()
			} else {
				startupProbe = createHttpGetProbe(parsedURL)
				livenessProbe = createHttpGetProbe(parsedURL)
				readinessProbe = createHttpGetProbe(parsedURL)
			}
		}
	} else {
		startupProbe = createExecProbe()
		livenessProbe = createExecProbe()
		readinessProbe = createExecProbe()
	}

	if s.StartupProbe == nil && startupProbe != nil {
		addStartUpIntervals(startupProbe)
		s.StartupProbe = startupProbe
	}

	if s.LivenessProbe == nil && livenessProbe != nil {
		addLivenessReadinessIntervals(livenessProbe)
		s.LivenessProbe = livenessProbe
	}

	if s.ReadinessProbe == nil && readinessProbe != nil {
		addLivenessReadinessIntervals(readinessProbe)
		s.ReadinessProbe = readinessProbe
	}
}

func (s *XMiniEnvK8sService) decodeNestedStructures(values map[string]any) error {
	servicePorts := []map[string]any{}
	for _, port := range s.Service.Ports {
		var mapPort map[string]any
		if err := mapstructure.Decode(port, &mapPort); err != nil {
			return Errorf("failed to resolve service port: %s", err)
		}
		servicePorts = append(servicePorts, mapPort)
	}

	if mapSvc, ok := values["service"].(map[string]any); ok {
		mapSvc["ports"] = servicePorts
		values["service"] = mapSvc
	}

	if len(s.ImagePullSecrets) != 0 {
		pullSecrets := []map[string]any{}
		for _, secret := range s.ImagePullSecrets {
			var mapSecret map[string]any
			if err := mapstructure.Decode(secret, &mapSecret); err != nil {
				return Errorf("failed to resolve imagePullSecret: %s", err)
			}
			pullSecrets = append(pullSecrets, mapSecret)
		}
		values["imagePullSecrets"] = pullSecrets
	}

	return nil
}
