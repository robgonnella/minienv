package config

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrK8sNotConfigured       errs.Kind = "config.k8s_not_configured"
	ErrExtensionDecode        errs.Kind = "config.extension_decode"
	ErrValuesDecode           errs.Kind = "config.values_decode"
	ErrImageRepositoryMissing errs.Kind = "config.image_repository_missing"
	ErrImageTagMissing        errs.Kind = "config.image_tag_missing"
	ErrGitShortSha            errs.Kind = "config.git_short_sha"
	ErrInvalidPort            errs.Kind = "config.invalid_port"
	ErrInvalidPublishedPort   errs.Kind = "config.invalid_published_port"
	ErrNgrokPortMismatch      errs.Kind = "config.ngrok_port_mismatch"
	ErrInvalidDeploymentType  errs.Kind = "config.invalid_deployment_type"
)
