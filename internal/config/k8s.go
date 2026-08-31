package config

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/rs/zerolog/log"
)

var urlRegex = regexp.MustCompile(`(?m)http:\/\/localhost(:?\:\d+)?(:?\/.*)?`)

// Required fields for deploying to Kubernetes
type XMiniEnvK8s struct {
	// Targets a specific cluster when deploying
	Context string `json:"context" yaml:"context" mapstructure:"context"`
	// Targets a specific namespace when deploying
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`
}

// Service level configuration for controlling Kubernetes deployment properties
type XMiniEnvK8sService struct {
	// Prevents the targeted service from being deployed to the cluster
	Skip *bool `json:"skip,omitempty" yaml:"skip,omitempty" mapstructure:"skip,omitempty"`
	// Controls the Helm timeout for deploying the targeted service
	DeploymentTimeout *string `json:"deploymentTimeout,omitempty" yaml:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
	// Chart value overrides for the Helm deployment
	Values *Values `json:"values,omitempty" yaml:"values,omitempty" mapstructure:"values,omitempty"`
}

type Values struct {
	// The number of replicas for this deployment
	Replicas *uint8 `json:"replicas,omitempty" yaml:"replicas,omitempty" mapstructure:"replicas,omitempty,omitempty"`
	// The image for this deployment. Will try to use compose service image if not set
	Image *ChartImage `json:"image,omitempty" yaml:"image,omitempty" mapstructure:"image,omitempty"`
	// Any image pull secrets required to pull images on the cluster
	ImagePullSecrets *[]ChartImagePullSecret `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty" mapstructure:"imagePullSecrets,omitempty"`
	// Service configuration including container and service port specifications
	Service *ChartService `json:"service,omitempty" yaml:"service,omitempty" mapstructure:"service,omitempty"`
	// Service account configuration
	ServiceAccount *ChartServiceAccount `json:"serviceAccount,omitempty" yaml:"serviceAccount,omitempty" mapstructure:"serviceAccount,omitempty"`
	// Container environment configuration
	Env *map[string]string `json:"env,omitempty" yaml:"env,omitempty" mapstructure:"env,omitempty"`
	// Annotations to add to the deployment pods
	PodAnnotations *map[string]string `json:"podAnnotations,omitempty" yaml:"podAnnotations,omitempty" mapstructure:"podAnnotations,omitempty"`
	// Labels to add to the deployment pods
	PodLabels *map[string]string `json:"podLabels,omitempty" yaml:"podLabels,omitempty" mapstructure:"podLabels,omitempty"`
	// Security context for the deployment pods
	PodSecurityContext *map[string]any `json:"podSecurityContext,omitempty" yaml:"podSecurityContext,omitempty" mapstructure:"podSecurityContext,omitempty"`
	// Security context for the container in each pod
	SecurityContext *map[string]any `json:"securityContext,omitempty" yaml:"securityContext,omitempty" mapstructure:"securityContext,omitempty"`
	// Resources configuration for deployment pods
	Resources *map[string]any `json:"resources,omitempty" yaml:"resources,omitempty" mapstructure:"resources,omitempty"`
	// Container startup probe configuration
	StartupProbe *map[string]any `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty" mapstructure:"startupProbe,omitempty"`
	// Container liveness probe configuration
	LivenessProbe *map[string]any `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty" mapstructure:"livenessProbe,omitempty"`
	// Container readiness probe configuration
	ReadinessProbe *map[string]any `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty" mapstructure:"readinessProbe,omitempty"`
	// Volumes configuration for the deployment pods
	Volumes *[]map[string]any `json:"volumes,omitempty" yaml:"volumes,omitempty" mapstructure:"volumes,omitempty"`
	// VolumeMounts configuration for the container
	VolumeMounts *[]map[string]any `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty" mapstructure:"volumeMounts,omitempty"`
	// NodeSelector configuration for the deployment pods
	NodeSelector *map[string]any `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty" mapstructure:"nodeSelector,omitempty"`
	// Tolerations configuration for the deployment pods
	Tolerations *[]map[string]any `json:"tolerations,omitempty" yaml:"tolerations,omitempty" mapstructure:"tolerations,omitempty"`
	// Affinity configuration for the deployment pods
	Affinity *map[string]any `json:"affinity,omitempty" yaml:"affinity,omitempty" mapstructure:"affinity,omitempty"`
}

// Configuration for service image
type ChartImage struct {
	// Image Repository for the service image
	Repository string `json:"repository" yaml:"repository" mapstructure:"repository"`
	// PullPolicy for this image
	PullPolicy *string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty" mapstructure:"pullPolicy,omitempty"`
	// Image tag for the service image
	Tag string `json:"tag" yaml:"tag" mapstructure:"tag"`
}

// Pull secrets to enable pulling private images
type ChartImagePullSecret struct {
	// The name of the secret for pulling images
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// ServiceAccount configuration for the deployment service
type ChartServiceAccount struct {
	// Whether or not to create a Kubernetes service account
	Create *bool `json:"create,omitempty" yaml:"create,omitempty" mapstructure:"create,omitempty"`
	// Whether or not to automount the service account
	Automount *bool `json:"automount,omitempty" yaml:"automount,omitempty" mapstructure:"automount,omitempty"`
	// Additional annotations for the service account
	Annotations *map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty" mapstructure:"annotations,omitempty"`
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
	Create *bool `json:"create,omitempty" yaml:"create,omitempty" mapstructure:"create,omitempty"`
	// The type of service to create
	ServiceType *string `json:"type,omitempty" yaml:"type,omitempty" mapstructure:"type,omitempty"`
	// The ports to associate with pod container and service mapping
	Ports []ChartServicePort `json:"ports" yaml:"ports" mapstructure:"ports"`
}

func (v *Values) Resolve(
	svc *types.ServiceConfig,
) (map[string]any, error) {
	if v == nil {
		v = &Values{}
	}

	if err := v.resolveServiceImage(svc); err != nil {
		return nil, err
	}

	if err := v.resolveServicePorts(svc); err != nil {
		return nil, err
	}

	v.resolveHealthCheck(svc)
	v.resolveEnvironment(svc)

	var values map[string]any

	if err := mapstructure.Decode(v, &values); err != nil {
		return nil, fmt.Errorf("failed to resolve chart values: %s", err)
	}

	if err := v.decodeNestedStructures(values); err != nil {
		return nil, err
	}

	log.Info().Fields(values).Msg("resolved chart values")

	return values, nil
}

func (v *Values) resolveServiceImage(svc *types.ServiceConfig) error {
	split := strings.SplitN(svc.Image, ":", 2)
	svcImageRepo := ""
	svcImageTag := ""

	if len(split) > 1 {
		svcImageRepo = split[0]
		svcImageTag = split[1]
	}

	if v.Image == nil {
		v.Image = &ChartImage{
			Repository: svcImageRepo,
			Tag:        svcImageTag,
		}
	}

	if v.Image.Repository == "" {
		v.Image.Repository = svcImageRepo
	}

	if v.Image.Tag == "" {
		v.Image.Tag = svcImageTag
	}

	var err error

	if v.Image.Repository == "" {
		err = errors.Join(
			err,
			fmt.Errorf("image.repository must be specified in service extension"),
		)
	}

	if v.Image.Tag == "" {
		err = errors.Join(
			err,
			fmt.Errorf("image.tag must be specified in service extension"),
		)
	}

	return err
}

func (v *Values) resolveServicePorts(svc *types.ServiceConfig) error {
	shouldCreate := true

	if v.Service == nil {
		v.Service = &ChartService{
			Create: &shouldCreate,
			Ports:  []ChartServicePort{},
		}
	}

	if len(svc.Ports) == 0 {
		shouldCreate = false
		v.Service.Create = &shouldCreate

		if v.ServiceAccount == nil {
			v.ServiceAccount = &ChartServiceAccount{Create: &shouldCreate}
		} else {
			v.ServiceAccount.Create = &shouldCreate
		}

		return nil
	}

	extensionHasPort := func(ctrPrt, svcPrt uint16) bool {
		for _, extPrt := range v.Service.Ports {
			if extPrt.ContainerPort == ctrPrt && extPrt.ServicePort == svcPrt {
				return true
			}
		}
		return false
	}

	for _, p := range svc.Ports {
		if p.Target > math.MaxInt16 {
			return fmt.Errorf("invalid port configuration: %+v", p.Target)
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

		var containerPort uint16 = uint16(p.Target)

		containerPortName := fmt.Sprintf("p%d", containerPort)
		servicePortName := fmt.Sprintf("p%d", published16)

		if !extensionHasPort(containerPort, published16) {
			v.Service.Ports = append(v.Service.Ports, ChartServicePort{
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

func (v *Values) resolveEnvironment(svc *types.ServiceConfig) {
	if len(svc.Environment) > 0 {
		mapping := map[string]string{}

		for key, value := range svc.Environment {
			mapping[key] = *value
		}

		if v.Env != nil {
			maps.Copy(mapping, *v.Env)
		}

		v.Env = &mapping
	}
}

func (v *Values) resolveHealthCheck(svc *types.ServiceConfig) {
	if svc.HealthCheck.Disable {
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
		port := u.Port()
		if port == "" {
			port = "p80"
		} else {
			port = fmt.Sprintf("p%s", port)
		}
		return map[string]any{
			"httpGet": map[string]any{
				"path": u.Path,
				"port": port,
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

	if v.StartupProbe == nil && startupProbe != nil {
		addStartUpIntervals(startupProbe)
		v.StartupProbe = &startupProbe
	}

	if v.LivenessProbe == nil && livenessProbe != nil {
		addLivenessReadinessIntervals(livenessProbe)
		v.LivenessProbe = &livenessProbe
	}

	if v.ReadinessProbe == nil && readinessProbe != nil {
		addLivenessReadinessIntervals(readinessProbe)
		v.ReadinessProbe = &readinessProbe
	}
}

func (v *Values) decodeNestedStructures(values map[string]any) error {
	servicePorts := []map[string]any{}
	for _, port := range v.Service.Ports {
		var mapPort map[string]any
		if err := mapstructure.Decode(port, &mapPort); err != nil {
			return fmt.Errorf("failed to resolve service port: %s", err)
		}
		servicePorts = append(servicePorts, mapPort)
	}

	if mapSvc, ok := values["service"].(map[string]any); ok {
		mapSvc["ports"] = servicePorts
		values["service"] = mapSvc
	}

	if v.ImagePullSecrets != nil {
		pullSecrets := []map[string]any{}
		for _, secret := range *v.ImagePullSecrets {
			var mapSecret map[string]any
			if err := mapstructure.Decode(secret, &mapSecret); err != nil {
				return fmt.Errorf("failed to resolve imagePullSecret: %s", err)
			}
			pullSecrets = append(pullSecrets, mapSecret)
		}
		values["imagePullSecrets"] = pullSecrets
	}

	return nil
}
