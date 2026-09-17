package config

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrK8sNotConfigured       errs.Kind = "config.k8s_not_configured"
	ErrExtensionDecode        errs.Kind = "config.extension_decode"
	ErrValuesDecode           errs.Kind = "config.values_decode"
	ErrImageRepositoryMissing errs.Kind = "config.image_repository_missing"
	ErrImageTagMissing        errs.Kind = "config.image_tag_missing"
	ErrImageDigestUnsupported errs.Kind = "config.image_digest_unsupported"
	ErrGitShortSha            errs.Kind = "config.git_short_sha"
	ErrInvalidPort            errs.Kind = "config.invalid_port"
	ErrNgrokPortMismatch      errs.Kind = "config.ngrok_port_mismatch"
	ErrInvalidDeploymentType  errs.Kind = "config.invalid_deployment_type"
	ErrManifestPath           errs.Kind = "config.manifest_path"
	ErrConfigMapFromPath      errs.Kind = "config.config_map_from_path"
	ErrCopyHostPath           errs.Kind = "config.copy_host_path"
	ErrCopyContainerPath      errs.Kind = "config.copy_container_path"
)
