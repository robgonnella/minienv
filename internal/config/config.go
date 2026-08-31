package config

import "github.com/compose-spec/compose-go/v2/types"

// Wrapper around compose service definition for easier differentiation
type ComposeService = types.ServiceConfig
type ComposeProject = types.Project

// XMiniEnv top-level extension configuration for deploying to various targets
type XMiniEnv struct {
	// Ngrok configuration for exposing services publicly
	Ngrok *Ngrok `json:"ngrok,omitempty" yaml:"ngrok,omitempty" mapstructure:"ngrok,omitempty"`
	// K8s configuration for deploying to Kubernetes
	K8s *XMiniEnvK8s `json:"k8s,omitempty" yaml:"k8s,omitempty" mapstructure:"k8s,omitempty"`
}

// Ngrok configuration for exposing services publicly
type Ngrok struct {
	// Exposes the specified service port publicly via ngrok. This port must match
	// one of the configured service ports. If ports were auto-discovered from
	// docker-compose config, this value should match the Host side of the
	// mapping, not container.
	Port uint16 `json:"port" yaml:"port" mapstructure:"port"`
	// Ngrok url configuration for the service endpoint
	Url *string `json:"url,omitempty" yaml:"url,omitempty" mapstructure:"url,omitempty"`
	// Ngrok "on_http_request" configuration
	TrafficPolicy *string `json:"trafficPolicy,omitempty" yaml:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty"`
}

// Common options shared across all service deployment type
type XMiniEnvCommonService struct {
	// Ngrok configuration for exposing services publicly
	Ngrok *Ngrok `json:"ngrok,omitempty" yaml:"ngrok,omitempty" mapstructure:"ngrok,omitempty"`
	// Prevents the targeted service from being deployed to the cluster
	Skip *bool `json:"skip,omitempty" yaml:"skip,omitempty" mapstructure:"skip,omitempty"`
}
