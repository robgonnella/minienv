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

type K8sTopLevelConfig struct {
	Context   string `yaml:"context" mapstructure:"context"`
	Namespace string `yaml:"namespace" mapstructure:"namespace"`
}

type K8sServiceConfig struct {
	Skip              *bool       `yaml:"skip" mapstructure:"skip"`
	DeploymentTimeout *string     `yaml:"deploymentTimeout" mapstructure:"deploymentTimeout"`
	ChartValues       ChartValues `yaml:"chartValues" mapstructure:"chartValues"`
}

type ChartValues struct {
	Replicas           *uint8                  `yaml:"replicas,omitempty" mapstructure:"replicas,omitempty,omitempty"`
	Image              *ChartImage             `yaml:"image,omitempty" mapstructure:"image,omitempty"`
	ImagePullSecrets   *[]ChartImagePullSecret `yaml:"imagePullSecrets,omitempty" mapstructure:"imagePullSecrets,omitempty"`
	Service            *ChartService           `yaml:"service,omitempty" mapstructure:"service,omitempty"`
	ServiceAccount     *ChartServiceAccount    `yaml:"serviceAccount,omitempty" mapstructure:"serviceAccount,omitempty"`
	Env                *map[string]string      `yaml:"env,omitempty" mapstructure:"env,omitempty"`
	PodAnnotations     *map[string]string      `yaml:"podAnnotations,omitempty" mapstructure:"podAnnotations,omitempty"`
	PodLabels          *map[string]string      `yaml:"podLabels,omitempty" mapstructure:"podLabels,omitempty"`
	PodSecurityContext *map[string]any         `yaml:"podSecurityContext,omitempty" mapstructure:"podSecurityContext,omitempty"`
	SecurityContext    *map[string]any         `yaml:"securityContext,omitempty" mapstructure:"securityContext,omitempty"`
	Resources          *map[string]any         `yaml:"resources,omitempty" mapstructure:"resources,omitempty"`
	StartupProbe       *map[string]any         `yaml:"startupProbe,omitempty" mapstructure:"startupProbe,omitempty"`
	LivenessProbe      *map[string]any         `yaml:"livenessProbe,omitempty" mapstructure:"livenessProbe,omitempty"`
	ReadinessProbe     *map[string]any         `yaml:"readinessProbe,omitempty" mapstructure:"readinessProbe,omitempty"`
	Volumes            *[]map[string]any       `yaml:"volumes,omitempty" mapstructure:"volumes,omitempty"`
	VolumeMounts       *[]map[string]any       `yaml:"volumeMounts,omitempty" mapstructure:"volumeMounts,omitempty"`
	NodeSelector       *map[string]any         `yaml:"nodeSelector,omitempty" mapstructure:"nodeSelector,omitempty"`
	Tolerations        *[]map[string]any       `yaml:"tolerations,omitempty" mapstructure:"tolerations,omitempty"`
	Affinity           *map[string]any         `yaml:"affinity,omitempty" mapstructure:"affinity,omitempty"`
}

type ChartImage struct {
	Repository string  `yaml:"repository" mapstructure:"repository"`
	PullPolicy *string `yaml:"pullPolicy,omitempty" mapstructure:"pullPolicy,omitempty"`
	Tag        string  `yaml:"tag" mapstructure:"tag"`
}

type ChartImagePullSecret struct {
	Name string `yaml:"name" mapstructure:"name"`
}

type ChartServiceAccount struct {
	Create      *bool             `yaml:"create" mapstructure:"create"`
	AutoMount   *bool             `yaml:"automount" mapstructure:"automount"`
	Annotations map[string]string `yaml:"annotations" mapstructure:"annotations"`
}

type ChartServicePort struct {
	Port          uint16 `yaml:"port" mapstructure:"port"`
	ContainerPort uint16 `yaml:"containerPort" mapstructure:"containerPort"`
	Name          string `yaml:"name" mapstructure:"name"`
	Protocol      string `yaml:"protocol" mapstructure:"protocol"`
}

type ChartService struct {
	Create      *bool              `yaml:"create,omitempty" mapstructure:"create,omitempty"`
	ServiceType *string            `yaml:"type,omitempty" mapstructure:"type,omitempty"`
	Ports       []ChartServicePort `yaml:"ports" mapstructure:"ports"`
}

func (v *ChartValues) Resolve(
	svc *types.ServiceConfig,
) (map[string]any, error) {
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

func (v *ChartValues) resolveServiceImage(svc *types.ServiceConfig) error {
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

func (v *ChartValues) resolveServicePorts(svc *types.ServiceConfig) error {
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

		name := fmt.Sprintf("p%d", containerPort)

		v.Service.Ports = append(v.Service.Ports, ChartServicePort{
			Name:          name,
			ContainerPort: containerPort,
			Port:          published16,
			Protocol:      protocol,
		})
	}

	return nil
}

func (v *ChartValues) resolveEnvironment(svc *types.ServiceConfig) {
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

func (v *ChartValues) resolveHealthCheck(svc *types.ServiceConfig) {
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

func (v *ChartValues) decodeNestedStructures(values map[string]any) error {
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
