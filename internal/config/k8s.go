package config

// ChartImage is the configuration for a service image.
type ChartImage struct {
	ServiceImage `mapstructure:",squash"`

	// PullPolicy for this image
	PullPolicy string `json:"pullPolicy,omitempty" mapstructure:"pullPolicy,omitempty"`
}

// ChartImagePullSecret names a secret that enables pulling private images.
type ChartImagePullSecret struct {
	// The name of the secret for pulling images
	Name string `json:"name" mapstructure:"name"`
}

// ChartServiceAccount is the service account configuration for the deployment.
type ChartServiceAccount struct {
	// Whether or not to create a Kubernetes service account
	Create *bool `json:"create,omitempty" jsonschema:"oneof_type=boolean;string" mapstructure:"create,omitempty"`
	// The name for the service account
	Name string `json:"name,omitempty" mapstructure:"name,omitempty"`
	// Whether or not to automount the service account
	Automount *bool `json:"automount,omitempty" jsonschema:"oneof_type=boolean;string" mapstructure:"automount,omitempty"`
	// Additional annotations for the service account
	Annotations map[string]string `json:"annotations,omitempty" mapstructure:"annotations,omitempty"`
}

// ChartServicePort is the port configuration used by both the service and the
// deployment pod container.
type ChartServicePort struct {
	// Name of the port, used by both the service and the container
	ContainerPortName string `json:"containerPortName" mapstructure:"containerPortName"`
	// Container port to expose, which the service exposes under the same number
	ContainerPort uint16 `json:"containerPort" mapstructure:"containerPort"`
	// Protocol to use for these ports
	Protocol string `json:"protocol" mapstructure:"protocol"`
}

// ChartService is the Kubernetes service configuration.
type ChartService struct {
	// Whether or not to create a Kubernetes service
	Create *bool `json:"create,omitempty" jsonschema:"oneof_type=boolean;string" mapstructure:"create,omitempty"`
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
	// Container environment variables as Kubernetes EnvVar entries, merged over compose environment by name
	Env []map[string]any `json:"env,omitempty" mapstructure:"env,omitempty"`
	// Sources to populate container environment variables from, as Kubernetes EnvFromSource entries
	EnvFrom []map[string]any `json:"envFrom,omitempty" mapstructure:"envFrom,omitempty"`
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
	// Deletes the defined namespace on destroy. Default is false to prevent removing pre-configured namespaces
	RemoveNamespaceOnDestroy bool `json:"removeNamespaceOnDestroy,omitempty" jsonschema:"default=false" mapstructure:"removeNamespaceOnDestroy,omitempty"`
	// Controls the Helm timeout. This is applied to all services but can be
	// overridden using the service-level extension
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
	// Ngrok configuration for exposing services publicly
	Ngrok *NgrokTopLevel `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero"`
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
	Recreate bool `json:"recreate,omitempty" jsonschema:"oneof_type=boolean;string" mapstructure:"recreate"`
	// Controls the type of deployment (service | job). Default is "service"
	DeploymentType K8sDeploymentType `json:"deploymentType,omitempty" jsonschema:"enum=service,enum=job,default=service" mapstructure:"deploymentType,omitempty"`
	// Controls the Helm timeout for deploying the targeted service
	DeploymentTimeout string `json:"deploymentTimeout,omitempty" mapstructure:"deploymentTimeout,omitempty"`
	// Paths to Kubernetes manifest files, relative to the directory of the compose file that declares them, rendered as part of this service's chart
	Manifests []string `json:"manifests,omitempty" mapstructure:"manifests,omitempty"`
	// Paths to files, relative to the directory of the compose file that declares them, whose contents become this service's ConfigMap keyed by file name
	ConfigMapFrom []string `json:"configMapFrom,omitempty" mapstructure:"configMapFrom,omitempty"`
}
