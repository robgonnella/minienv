package config

type MainExtensionConfig struct {
	K8s *K8sTopLevelConfig `yaml:"k8s" mapstructure:"k8s"`
}
