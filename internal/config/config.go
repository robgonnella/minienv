// Package config models the x-minienv extension fields and resolves them
// against the surrounding docker-compose configuration.
package config

import "github.com/compose-spec/compose-go/v2/types"

// ComposeService wraps the compose service definition for easier
// differentiation.
type ComposeService = types.ServiceConfig
type ComposeProject = types.Project

// NgrokTopLevel is the ngrok configuration applied to all services unless
// overridden per service.
type NgrokTopLevel struct {
	// Ngrok "on_http_request" configuration
	TrafficPolicy string `json:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty" yaml:"trafficPolicy,omitempty"`
}

// XMiniEnv top-level extension configuration for deploying to various targets.
type XMiniEnv struct {
	// Ngrok configuration for exposing services publicly
	Ngrok NgrokTopLevel `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero" yaml:"ngrok,omitzero"`
	// K8s configuration for deploying to Kubernetes
	K8s XMiniEnvK8s `json:"k8s,omitzero" mapstructure:"k8s,omitzero" yaml:"k8s,omitzero"`
}

// Ngrok is the configuration for exposing a service publicly.
type Ngrok struct {
	// Exposes the specified service port publicly via ngrok. This port must match
	// one of the configured service ports. If ports were auto-discovered from
	// docker-compose config, this value should match the Host side of the
	// mapping, not container.
	Port uint16 `json:"port" mapstructure:"port" yaml:"port"`
	// Ngrok url configuration for the service endpoint
	URL string `json:"url,omitempty" mapstructure:"url,omitempty" yaml:"url,omitempty"`
	// Ngrok "on_http_request" configuration
	TrafficPolicy string `json:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty" yaml:"trafficPolicy,omitempty"`
}

// XMiniEnvCommonService holds the options shared across all service deployment
// types.
type XMiniEnvCommonService struct {
	// Ngrok configuration for exposing services publicly
	Ngrok Ngrok `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero" yaml:"ngrok,omitzero"`
	// Prevents the targeted service from being deployed to the cluster
	Skip bool `json:"skip,omitempty" mapstructure:"skip,omitempty" yaml:"skip,omitempty"`
}
