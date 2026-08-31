package config

// XMiniEnv top-level extension configuration for deploying to various targets
type XMiniEnv struct {
	// Ngrok configuration for exposing services publicly
	Ngrok *Ngrok `json:"ngrok,omitempty" yaml:"ngrok,omitempty" mapstructure:"ngrok,omitempty"`
	// K8s configuration for deploying to Kubernetes
	K8s *XMiniEnvK8s `json:"k8s,omitempty" yaml:"k8s,omitempty" mapstructure:"k8s,omitempty"`
}

// Ngrok configuration for exposing services publicly
type Ngrok struct {
	// Ngrok "on_http_request" configuration
	TrafficPolicy *string `json:"trafficPolicy,omitempty" yaml:"trafficPolicy,omitempty" mapstructure:"trafficPolicy,omitempty"`
}

// Common options shared across all service deployment type
type XMiniEnvCommonService struct {
	// Ngrok configuration for exposing services publicly
	Ngrok *Ngrok `json:"ngrok,omitempty" yaml:"ngrok,omitempty" mapstructure:"ngrok,omitempty"`
	// Prevents the targeted service from being deployed to the cluster
	Skip *bool `json:"skip,omitempty" yaml:"skip,omitempty" mapstructure:"skip,omitempty"`
	// Exposes the specified service port publicly via ngrok. This port must match
	// one of the configured service ports. If ports were auto-discovered from
	// docker-compose config, this value should match one of the mappings for
	// the host port, not container port
	ExposeServicePort *uint16 `json:"exposeServicePort,omitempty" yaml:"exposeServicePort,omitempty" mapstructure:"exposeServicePort,omitempty"`
}
