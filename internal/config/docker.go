package config

// XMiniEnvDocker uses docker and a specified transport mechanism to deploy
// modified docker-compose config on a remote server.
type XMiniEnvDocker struct {
	// The namespace for this deployment to ensure there are no conflicts
	// with other user minienvs on the same host
	Namespace string `json:"namespace" mapstructure:"namespace"`
	// The transport mechanism to use for communication with the remote server
	Transport XMiniEnvDockerTransport `json:"transport" mapstructure:"transport"`
	// Ngrok configuration for exposing services publicly
	Ngrok *NgrokTopLevel `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero"`
}

// XMiniEnvSSH holds the fields required for deploying to any SSH-enabled
// remote server.
type XMiniEnvSSH struct {
	// The target host for the SSH connection
	Host string `json:"host" mapstructure:"host"`
	// The user for the SSH connection
	User string `json:"user" mapstructure:"user"`
	// The port for the SSH connection
	Port uint16 `json:"port" mapstructure:"port"`
	// The RSA private key identity file for the SSH connection
	Identity string `json:"identity" mapstructure:"identity"`
}

// XMiniEnvDockerTransport holds the transport mechanism for copying files
// and running commands on a remote server. Only one transport can be defined
// at a time.
type XMiniEnvDockerTransport struct {
	// Copies files and runs commands on the remote server over SSH
	SSH *XMiniEnvSSH `json:"ssh,omitempty" mapstructure:"ssh,omitempty"`
}

// ConfigFields returns the available "json" config fields for this extension.
func (x *XMiniEnvDockerTransport) ConfigFields() []string {
	return []string{"ssh"}
}

// XMiniEnvDockerCopy copies a file or directory to the remote server and bind
// mounts it into the container at the specified containerPath. Normal compose
// bind mounts are intentionally discarded making this config the override
// escape hatch when needed.
type XMiniEnvDockerCopy struct {
	// The path, relative to the directory of the compose file that declares it, of the file or directory to be copied
	HostPath string `json:"hostPath" mapstructure:"hostPath"`
	// The absolute path inside the container to mount the file or directory
	ContainerPath string `json:"containerPath" mapstructure:"containerPath"`
}

// XMiniEnvDockerService is the service level configuration controlling docker
// deployment properties.
type XMiniEnvDockerService struct {
	XMiniEnvCommonService `mapstructure:",squash"`

	// The image for this deployment. Will try to use compose service image if not set
	Image ServiceImage `json:"image,omitzero" mapstructure:"image,omitzero"`

	// Files and directories copied to the remote server and bind mounted into
	// the container. Normal compose bind mounts are intentionally discarded
	// making this config the override escape hatch when needed.
	Copy []XMiniEnvDockerCopy `json:"copy,omitempty" mapstructure:"copy,omitempty"`
}
