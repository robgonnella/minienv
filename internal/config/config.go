package config

// XMiniEnv top-level extension configuration for deploying to various targets
type XMiniEnv struct {
	// K8s configuration for deploying to Kubernetes
	K8s *XMiniEnvK8s `json:"k8s,omitempty" yaml:"k8s,omitempty" mapstructure:"k8s,omitempty"`
}
