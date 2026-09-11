// Package config models the x-minienv extension fields and resolves them
// against the surrounding docker-compose configuration.
package config

import "github.com/compose-spec/compose-go/v2/types"

// ComposeService wraps the compose service definition for easier
// differentiation.
type ComposeService = types.ServiceConfig
type ComposeProject = types.Project

// XMiniEnv top-level extension configuration for deploying to various targets.
type XMiniEnv struct {
	// K8s configuration for deploying to Kubernetes
	K8s *XMiniEnvK8s `json:"k8s,omitzero" mapstructure:"k8s,omitzero"`
	// Docker uses docker and a specified transport mechanism to deploy modified
	// docker-compose config on a remote server
	Docker *XMiniEnvDocker `json:"docker,omitzero" mapstructure:"docker,omitzero"`
}

// ConfigFields returns the available "json" config fields for this extension.
func (x *XMiniEnv) ConfigFields() []string {
	return []string{"k8s", "docker"}
}

// NgrokTopLevel is the ngrok configuration applied to all services unless
// overridden per service.
type NgrokTopLevel struct {
	// Ngrok "on_http_request" configuration
	TrafficPolicy string `json:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty"`
}

// NgrokServiceLevel is the configuration for exposing a service publicly.
type NgrokServiceLevel struct {
	// Exposes the specified service port publicly via ngrok. This port must match
	// one of the configured service ports. Which side of a docker-compose port
	// mapping that is depends on the target: k8s routes through a Service and
	// expects the host side, docker reaches the container directly over the
	// compose network and expects the container side.
	Port uint16 `json:"port" mapstructure:"port"`
	// Ngrok url configuration for the service endpoint
	URL string `json:"url,omitempty" mapstructure:"url,omitempty"`
	// Ngrok "on_http_request" configuration
	TrafficPolicy string `json:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty"`
}

// NgrokEndpointConfig is used to generate the necessary config for running
// supporting ngrok service.
type NgrokEndpointConfig struct {
	Namespace    string `mapstructure:"namespace"`
	EndpointName string `mapstructure:"endpointName"`
	ServiceName  string `mapstructure:"serviceName"`
	URL          string `mapstructure:"url"`
	// The port the ngrok upstream dials, which is not the same thing per
	// target: the k8s Service port under helm, the container port under
	// docker compose.
	Port          uint16 `mapstructure:"port"`
	TrafficPolicy string `mapstructure:"trafficPolicy"`
}

// ServiceImage represents the image that will be pulled and used for a service
// in the remote environment. It is expected that the remote environment
// already has credentials to pull from this registry.
type ServiceImage struct {
	// Image Repository for the service image
	Repository string `json:"repository" mapstructure:"repository"`
	// Image tag for the service image
	Tag string `json:"tag" mapstructure:"tag"`
	// The platforms for which to build and push default [linux/amd64])
	Platforms []string `json:"platforms,omitempty" mapstructure:"platforms,omitempty"`
}

// XMiniEnvCommonService holds the options shared across all service deployment
// types.
type XMiniEnvCommonService struct {
	// Ngrok configuration for exposing services publicly
	Ngrok NgrokServiceLevel `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero"`
	// Prevents the targeted service from being deployed to the cluster
	Skip bool `json:"skip,omitempty" mapstructure:"skip,omitempty"`
}
