// Package resolver resolves the x-minienv service extensions against the
// compose service they annotate.
package resolver

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrK8sNotConfigured       errs.Kind = "resolver.k8s_not_configured"
	ErrExtensionDecode        errs.Kind = "resolver.extension_decode"
	ErrValuesDecode           errs.Kind = "resolver.values_decode"
	ErrImageRepositoryMissing errs.Kind = "resolver.image_repository_missing"
	ErrImageTagMissing        errs.Kind = "resolver.image_tag_missing"
	ErrImageDigestUnsupported errs.Kind = "resolver.image_digest_unsupported"
	ErrGitShortSha            errs.Kind = "resolver.git_short_sha"
	ErrInvalidPort            errs.Kind = "resolver.invalid_port"
	ErrNgrokPortMismatch      errs.Kind = "resolver.ngrok_port_mismatch"
	ErrInvalidDeploymentType  errs.Kind = "resolver.invalid_deployment_type"
	ErrManifestPath           errs.Kind = "resolver.manifest_path"
	ErrConfigMapFromPath      errs.Kind = "resolver.config_map_from_path"
	ErrCopyHostPath           errs.Kind = "resolver.copy_host_path"
	ErrCopyContainerPath      errs.Kind = "resolver.copy_container_path"
)
